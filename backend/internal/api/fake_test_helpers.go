package api

// fake_test_helpers.go — common test doubles shared by every
// *_test.go file in this package.
//
// Goals:
//   - One fake per interface (SessionWriter, TelemetryWriter,
//     UserPurger, DeviceRegistrar, OperatorLookup, Logger).
//   - Fakes are goroutine-safe (sync.Mutex) so concurrent
//     handler tests don't race.
//   - Fakes expose per-method call counters so tests can
//     assert "InsertTelemetry was called exactly once with
//     these args".
//   - Fakes return canned errors via SetError so a single test
//     can exercise the error path without rewriting the fake.

// (intentionally no package-level build tags; this file is
// compiled only when _test.go files reference its symbols.)

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/opene2ee-com/e2ee-app/backend/internal/operator"
	"github.com/opene2ee-com/e2ee-app/backend/internal/storage"
)

// -----------------------------------------------------------------------------
// Fake store (covers SessionWriter, TelemetryWriter, UserPurger, DeviceRegistrar)
// -----------------------------------------------------------------------------

// fakeStore is an in-memory implementation of every storage
// interface the api package depends on. The Sessions / Telemetry
// maps are keyed by id so GetSession / InsertTelemetry round-trip
// cleanly. DeleteUser removes from every map (mimicking the
// real transactional delete).
type fakeStore struct {
	mu sync.Mutex

	Sessions      map[uuid.UUID]storage.Session
	TelemetryRows []storage.Telemetry
	TelemetryIDs  []int64
	Devices       map[string]fakeDevice // device_id_hash → row
	DeletedHashes []string

	nextTelemetryID int64

	// Errors overrides. Setting InsertSessionErr to a non-nil
	// value makes InsertSession return that error; same for
	// the others. Cleared via ClearErrors.
	InsertSessionErr      error
	InsertTelemetryErr    error
	GetSessionErr         error
	ListSessionsErr       error
	UpsertDeviceErr       error
	DeleteUserErr         error
	UpdateSessionStatusErr error
}

type fakeDevice struct {
	Hash      string
	PublicKey []byte
	FP        string
}

// newFakeStore returns a fresh fakeStore ready for use. Each
// test gets its own instance via t.Cleanup that clears any
// background state.
func newFakeStore() *fakeStore {
	return &fakeStore{
		Sessions:        make(map[uuid.UUID]storage.Session),
		Devices:         make(map[string]fakeDevice),
		nextTelemetryID: 0,
	}
}

func (f *fakeStore) ClearErrors() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.InsertSessionErr = nil
	f.InsertTelemetryErr = nil
	f.GetSessionErr = nil
	f.ListSessionsErr = nil
	f.UpsertDeviceErr = nil
	f.DeleteUserErr = nil
	f.UpdateSessionStatusErr = nil
}

// SessionWriter interface
func (f *fakeStore) InsertSession(ctx context.Context, s storage.Session) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.InsertSessionErr != nil {
		return f.InsertSessionErr
	}
	if _, exists := f.Sessions[s.ID]; exists {
		return errors.New("fake: session already exists")
	}
	f.Sessions[s.ID] = s
	return nil
}

func (f *fakeStore) UpdateSessionStatus(ctx context.Context, id uuid.UUID, status string, endedAt *time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.UpdateSessionStatusErr != nil {
		return f.UpdateSessionStatusErr
	}
	s, ok := f.Sessions[id]
	if !ok {
		return storage.ErrNotFound
	}
	s.Status = status
	if endedAt != nil {
		s.EndedAt = endedAt
	}
	f.Sessions[id] = s
	return nil
}

func (f *fakeStore) GetSession(ctx context.Context, id uuid.UUID) (*storage.Session, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.GetSessionErr != nil {
		return nil, f.GetSessionErr
	}
	s, ok := f.Sessions[id]
	if !ok {
		return nil, storage.ErrNotFound
	}
	cp := s
	return &cp, nil
}

func (f *fakeStore) ListSessions(ctx context.Context, limit int) ([]storage.Session, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.ListSessionsErr != nil {
		return nil, f.ListSessionsErr
	}
	if limit <= 0 {
		limit = 50
	}
	out := make([]storage.Session, 0, len(f.Sessions))
	for _, s := range f.Sessions {
		out = append(out, s)
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

// TelemetryWriter interface
func (f *fakeStore) InsertTelemetry(ctx context.Context, t storage.Telemetry) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.InsertTelemetryErr != nil {
		return 0, f.InsertTelemetryErr
	}
	f.nextTelemetryID++
	id := f.nextTelemetryID
	f.TelemetryRows = append(f.TelemetryRows, t)
	f.TelemetryIDs = append(f.TelemetryIDs, id)
	return id, nil
}

// UserPurger interface
func (f *fakeStore) DeleteUser(ctx context.Context, deviceIDHash string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.DeleteUserErr != nil {
		return f.DeleteUserErr
	}
	f.DeletedHashes = append(f.DeletedHashes, deviceIDHash)
	delete(f.Devices, deviceIDHash)
	// Cascade — drop telemetry and null-out sessions, mirroring
	// the real storage.DeleteUser.
	kept := f.TelemetryRows[:0]
	for _, row := range f.TelemetryRows {
		if row.DeviceIDHash != deviceIDHash {
			kept = append(kept, row)
		}
	}
	f.TelemetryRows = kept
	for id, s := range f.Sessions {
		if s.SenderHash != nil && *s.SenderHash == deviceIDHash {
			s.SenderHash = nil
		}
		if s.ReceiverHash != nil && *s.ReceiverHash == deviceIDHash {
			s.ReceiverHash = nil
		}
		f.Sessions[id] = s
	}
	return nil
}

// DeviceRegistrar interface
func (f *fakeStore) UpsertDevice(ctx context.Context, hash string, publicKey []byte, fp string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.UpsertDeviceErr != nil {
		return f.UpsertDeviceErr
	}
	f.Devices[hash] = fakeDevice{Hash: hash, PublicKey: append([]byte(nil), publicKey...), FP: fp}
	return nil
}

// -----------------------------------------------------------------------------
// Fake operator lookup
// -----------------------------------------------------------------------------

// fakeOperator is a stub OperatorLookup. The canned lookup
// tables let tests cover phone and IP paths without a real
// service.
type fakeOperator struct {
	mu sync.Mutex

	PhoneByE164 map[string]*operator.OperatorInfo
	IPByAddress map[string]*operator.OperatorInfo

	Err error
}

func newFakeOperator() *fakeOperator {
	return &fakeOperator{
		PhoneByE164: make(map[string]*operator.OperatorInfo),
		IPByAddress: make(map[string]*operator.OperatorInfo),
	}
}

func (f *fakeOperator) LookupByPhone(ctx context.Context, e164 string) (*operator.OperatorInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return nil, f.Err
	}
	if info, ok := f.PhoneByE164[e164]; ok {
		return info, nil
	}
	// Default unknown — same shape as Service.makeUnknown.
	return &operator.OperatorInfo{
		QueryType:  operator.QueryPhoneE164,
		QueryValue: e164,
		Operator:   "unknown",
		Source:     operator.SourceFallbackUnknown,
		Confidence: 0.0,
		Timestamp:  time.Now().UTC(),
	}, nil
}

func (f *fakeOperator) LookupByIP(ctx context.Context, ip string) (*operator.OperatorInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return nil, f.Err
	}
	if info, ok := f.IPByAddress[ip]; ok {
		return info, nil
	}
	return &operator.OperatorInfo{
		QueryType:  operator.QueryIPv4,
		QueryValue: ip,
		Operator:   "unknown",
		Source:     operator.SourceFallbackUnknown,
		Confidence: 0.0,
		Timestamp:  time.Now().UTC(),
	}, nil
}

// -----------------------------------------------------------------------------
// Fake logger — captures every call so tests can assert log content
// -----------------------------------------------------------------------------

// fakeLogger records every call. Tests inspect Entries to
// verify the privacy contract (no forbidden fields, etc.).
type fakeLogger struct {
	mu      sync.Mutex
	Entries []fakeLogEntry
}

type fakeLogEntry struct {
	Level string
	Msg   string
	Args  map[string]any
}

func newFakeLogger() *fakeLogger { return &fakeLogger{} }

func (l *fakeLogger) Info(msg string, args ...any)  { l.record("info", msg, args) }
func (l *fakeLogger) Error(msg string, args ...any) { l.record("error", msg, args) }
func (l *fakeLogger) Warn(msg string, args ...any)  { l.record("warn", msg, args) }
func (l *fakeLogger) Debug(msg string, args ...any) { l.record("debug", msg, args) }

func (l *fakeLogger) record(level, msg string, args []any) {
	m := make(map[string]any, len(args)/2)
	for i := 0; i+1 < len(args); i += 2 {
		k, ok := args[i].(string)
		if !ok {
			continue
		}
		m[k] = args[i+1]
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.Entries = append(l.Entries, fakeLogEntry{Level: level, Msg: msg, Args: m})
}

// LastEntry returns the most recent log entry (or zero-value if none).
func (l *fakeLogger) LastEntry() fakeLogEntry {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.Entries) == 0 {
		return fakeLogEntry{}
	}
	return l.Entries[len(l.Entries)-1]
}

// EntriesByLevel returns the subset of entries whose Level matches.
func (l *fakeLogger) EntriesByLevel(level string) []fakeLogEntry {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]fakeLogEntry, 0)
	for _, e := range l.Entries {
		if e.Level == level {
			out = append(out, e)
		}
	}
	return out
}

// -----------------------------------------------------------------------------
// newTestAPI wires the API with all fake dependencies and returns
// the API + a snapshot of the fake handles so tests can inspect.
// -----------------------------------------------------------------------------

type testAPI struct {
	*API
	Store    *fakeStore
	Operator *fakeOperator
	Logger   *fakeLogger
	Matrix   *MemoryMatrixQuerier
}

// newTestAPI is the canonical constructor for handler tests.
// Tests call it from t.Run and immediately get a working mux
// over the in-memory fakes.
func newTestAPI(t *testing.T) *testAPI {
	t.Helper()
	store := newFakeStore()
	op := newFakeOperator()
	logger := newFakeLogger()
	mq := NewMemoryMatrixQuerier()
	rl := NewRateLimiter(DefaultRateLimitConfig)

	a, err := New(Config{
		Logger:          logger,
		Sessions:        store,
		Telemetry:       store,
		Users:           store,
		Operator:        op,
		Matrix:          mq,
		Devices:         store,
		RateLimit:       rl,
		MaxBodyBytes:    1024 * 1024, // 1 MB in tests
	})
	if err != nil {
		t.Fatalf("api.New: %v", err)
	}
	return &testAPI{
		API:      a,
		Store:    store,
		Operator: op,
		Logger:   logger,
		Matrix:   mq,
	}
}

// readJSON is a small helper for tests — decodes the response
// body into v. Test helper, not exported.
func readJSON(t *testing.T, r io.Reader, v any) {
	t.Helper()
	if err := jsonDecode(r, v); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}
}

// jsonDecode indents the standard json.Decode so callers can
// import the helper without colliding with the package name.
func jsonDecode(r io.Reader, v any) error {
	dec := json.NewDecoder(r)
	return dec.Decode(v)
}