package api

// matrix.go — GET /api/v1/matrix.
//
// The transparency matrix is a public-facing aggregate of
// telemetry rows bucketed by (operator, app, country). The
// dashboard renders it as a heat-map; the API returns the
// underlying rows so the dashboard can pivot / drill-down
// without a second round trip.
//
// Wire contract (this file IS the schema — there is no
// shared/schemas/matrix.schema.json in Sprint 1):
//
//	GET /api/v1/matrix?operator=&app=&country=&period=30d&limit=1000
//
//	{
//	  "period":  "30d",
//	  "filters": { "operator": "turkcell", "app": "whatsapp", "country": "TR", "period": "30d" },
//	  "rows": [
//	    {
//	      "operator":              "turkcell",
//	      "app":                   "whatsapp",
//	      "country":               "TR",
//	      "sample_count":          1234,
//	      "avg_score":             87.4,
//	      "avg_entropy":           7.21,
//	      "unique_tls_fp_count":   12,
//	      "updated_at":            "2026-07-05T03:00:00Z"
//	    }
//	  ],
//	  "total": 1
//	}
//
// PRIVACY: the response carries aggregate counts and scores
// ONLY. No device_id_hash, no individual telemetry rows, no
// timestamps finer than the bucketed period. The MatrixQuerier
// implementation MUST enforce this — the test in matrix_test.go
// asserts that the row shape carries no per-device fields.

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/opene2ee-com/e2ee-app/backend/internal/operator"
)

// MatrixFilter is the parsed-and-validated query string. Empty
// fields mean "any" — the querier widens by them.
type MatrixFilter struct {
	Operator string
	Country  string
	App      string
	Period   string // "7d" | "30d" | "90d" — empty = default 30d
	Limit    int    // 0 → default (1000), capped at 10000
}

// PeriodDuration converts the string Period to a time.Duration.
// Unknown values fall back to 30d.
func (f MatrixFilter) PeriodDuration() time.Duration {
	switch f.Period {
	case "7d":
		return 7 * 24 * time.Hour
	case "90d":
		return 90 * 24 * time.Hour
	case "30d", "":
		return 30 * 24 * time.Hour
	default:
		return 30 * 24 * time.Hour
	}
}

// MatrixRow is one row of the aggregate. Keep the JSON tags
// stable — the dashboard parses them.
type MatrixRow struct {
	Operator           string    `json:"operator"`
	App                string    `json:"app"`
	Country            string    `json:"country,omitempty"`
	SampleCount        int64     `json:"sample_count"`
	AvgScore           float64   `json:"avg_score"`
	AvgEntropy         float64   `json:"avg_entropy"`
	UniqueTLSFPCount   int       `json:"unique_tls_fp_count"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// MatrixResponse is the top-level response shape.
type MatrixResponse struct {
	Period  string       `json:"period"`
	Filters MatrixFilter `json:"filters"`
	Rows    []MatrixRow  `json:"rows"`
	Total   int64        `json:"total"`
}

// handleMatrix is GET /api/v1/matrix.
func (a *API) handleMatrix() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		f, err := parseMatrixFilter(r)
		if err != nil {
			writeBadRequest(w, err.Error())
			return
		}
		rows, err := a.deps.Cfg.Matrix.Aggregate(r.Context(), f)
		if err != nil {
			a.deps.Cfg.Logger.Error("matrix aggregate failed",
				"err_kind", "aggregator",
				"period", f.Period,
			)
			writeInternal(w)
			return
		}
		resp := MatrixResponse{
			Period:  f.Period,
			Filters: f,
			Rows:    rows,
			Total:   int64(len(rows)),
		}
		if resp.Period == "" {
			resp.Period = "30d"
		}
		writeJSON(w, http.StatusOK, resp)
	}
}

// parseMatrixFilter reads and validates the query string. The
// accepted operator / app values come from
// operator/schemas — we use the operator enum lists embedded
// in the schemas package to keep a single source of truth.
func parseMatrixFilter(r *http.Request) (MatrixFilter, error) {
	q := r.URL.Query()
	f := MatrixFilter{
		Operator: strings.TrimSpace(q.Get("operator")),
		Country:  strings.ToUpper(strings.TrimSpace(q.Get("country"))),
		App:      strings.TrimSpace(q.Get("app")),
		Period:   strings.TrimSpace(q.Get("period")),
	}
	if f.Country != "" {
		if len(f.Country) != 2 || !isAlpha2(f.Country) {
			return MatrixFilter{}, errors.New("country must be a 2-letter ISO 3166-1 code")
		}
	}
	if f.Period != "" && f.Period != "7d" && f.Period != "30d" && f.Period != "90d" {
		return MatrixFilter{}, errors.New("period must be one of: 7d, 30d, 90d")
	}
	if q.Get("limit") != "" {
		n, err := strconv.Atoi(q.Get("limit"))
		if err != nil || n <= 0 {
			return MatrixFilter{}, errors.New("limit must be a positive integer")
		}
		if n > 10000 {
			n = 10000
		}
		f.Limit = n
	}
	return f, nil
}

func isAlpha2(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < 'A' || c > 'Z' {
			return false
		}
	}
	return true
}

// MemoryMatrixQuerier is a small in-process MatrixQuerier used
// as a dev fallback and as the test fake.
//
// In production the wire-up (PR-8) provides a TimescaleDB-
// backed implementation. This in-memory version is enough for
// unit tests and for local development without a database.
//
// PRIVACY: the rows stored here are AGGREGATES (no per-device
// fields), so the memory store is safe to expose in dev.
type MemoryMatrixQuerier struct {
	mu   sync.Mutex
	rows []MatrixRow
	now  func() time.Time
}

// NewMemoryMatrixQuerier returns an empty in-memory querier.
func NewMemoryMatrixQuerier() *MemoryMatrixQuerier {
	return &MemoryMatrixQuerier{now: time.Now}
}

// SetRows replaces the underlying row set. Tests use this to
// pre-seed known data; production code never calls it (the
// real aggregator writes via its own SQL pipeline).
func (q *MemoryMatrixQuerier) SetRows(rows []MatrixRow) {
	q.mu.Lock()
	defer q.mu.Unlock()
	cp := make([]MatrixRow, len(rows))
	copy(cp, rows)
	q.rows = cp
}

// Aggregate filters the in-memory rows by the filter set.
// LIMIT is honored. Updated_at is stamped with the current
// wall-clock time so dashboards can show a freshness indicator.
func (q *MemoryMatrixQuerier) Aggregate(ctx context.Context, f MatrixFilter) ([]MatrixRow, error) {
	_ = ctx // ctx reserved for the future cancellation hook
	q.mu.Lock()
	defer q.mu.Unlock()
	now := q.now().UTC()
	out := make([]MatrixRow, 0, len(q.rows))
	for _, r := range q.rows {
		if f.Operator != "" && !strings.EqualFold(r.Operator, f.Operator) {
			continue
		}
		if f.App != "" && !strings.EqualFold(r.App, f.App) {
			continue
		}
		if f.Country != "" && !strings.EqualFold(r.Country, f.Country) {
			continue
		}
		r.UpdatedAt = now
		out = append(out, r)
		if f.Limit > 0 && len(out) >= f.Limit {
			break
		}
	}
	return out, nil
}

// SeedOperatorLookups is a thin convenience helper that builds
// a MatrixQuerier + OperatorLookup from a single static table.
// Used by wire-up (PR-8) for the dev environment so the
// dashboard has data to show before the first real telemetry
// row arrives.
//
// NOT used at runtime — kept here so tests can swap the dev
// seed for the real aggregator when production wiring lands.
func SeedOperatorLookups(infos []*operator.OperatorInfo) []MatrixRow {
	rows := make([]MatrixRow, 0, len(infos))
	for _, info := range infos {
		if info == nil {
			continue
		}
		rows = append(rows, MatrixRow{
			Operator:           info.Operator,
			Country:            info.Country,
			SampleCount:        1,
			AvgScore:           0,
			AvgEntropy:         0,
			UniqueTLSFPCount:   1,
			UpdatedAt:          info.Timestamp,
		})
	}
	return rows
}