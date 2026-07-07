// main_test.go — Sprint 7 SEC-1 posture tests for backend/cmd/server.
//
// Pins the JWT_SECRET fail-closed behavior called out in
// cmd/server/main.go:138 (silent dev fallback replaced with a
// fail-closed default gated on OE2EE_ENV=dev). Three cases are
// required by the task brief (Sprint 7 Item 8, §5-6):
//
//  1. JWT_SECRET unset + OE2EE_ENV != "dev" → loadConfig()
//     returns an error so main() logs ERROR and exits 1.
//  2. JWT_SECRET unset + OE2EE_ENV = "dev" → loadConfig()
//     succeeds and sets JWTSecretFallbackDev=true so main()
//     emits the WARN with `fallback_dev=true`.
//  3. JWT_SECRET set (any OE2EE_ENV)        → loadConfig()
//     succeeds; JWTSecretFallbackDev=false; no WARN emitted.
//
// Plus three regression-pinning tests:
//
//  4. JWT_SECRET set + OE2EE_ENV=dev → explicit secret wins,
//     JWTSecretFallbackDev must be false.
//  5. logJWTSecretPosture + dev-fallback flag → emits a WARN
//     with `fallback_dev=true` and "DEV FALLBACK" in the msg.
//  6. logJWTSecretPosture + no fallback → must be silent.
//
// All env mutations use `t.Setenv` so the test runner auto-
// restores the parent environment at test end — no test can leak
// into another, and CI's pre-existing JWT_SECRET (if any) is
// preserved across runs.
package main

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLoadConfig_JWTSecret_Unset_NonDev_FailsClosed is case (1).
// Without JWT_SECRET and without dev mode, loadConfig MUST return
// an error so main() logs ERROR and exits 1. This is the
// production fail-closed posture for Sprint 7 SEC-1 — the
// previous behavior (silent fallback to a known constant) made
// it possible to ship a backend signing real tokens with a
// well-known secret.
func TestLoadConfig_JWTSecret_Unset_NonDev_FailsClosed(t *testing.T) {
	// Ensure both env vars start cleared for this test. t.Setenv
	// restores the previous value when the test ends.
	t.Setenv("JWT_SECRET", "")
	t.Setenv("OE2EE_ENV", "")

	_, err := loadConfig()
	require.Error(t, err, "loadConfig must fail when JWT_SECRET is unset and not in dev mode")
	assert.Contains(t, err.Error(), "JWT_SECRET",
		"error message must name the env var so the operator knows what to fix")
	assert.Contains(t, err.Error(), "OE2EE_ENV=dev",
		"error message must mention the dev-mode escape hatch so the operator has a path forward")
}

// TestLoadConfig_JWTSecret_Unset_NonDev_ProductionEnv_FailsClosed
// is the same posture under an explicit production env value.
// "production" / "prod" / anything not "dev" must all behave
// like the unset case — only literal "dev" opens the fallback.
func TestLoadConfig_JWTSecret_Unset_NonDev_ProductionEnv_FailsClosed(t *testing.T) {
	t.Setenv("JWT_SECRET", "")
	t.Setenv("OE2EE_ENV", "production")

	_, err := loadConfig()
	require.Error(t, err, "OE2EE_ENV=production must NOT open the dev fallback")
}

// TestLoadConfig_JWTSecret_Unset_Dev_Fallback is case (2). With
// JWT_SECRET unset and OE2EE_ENV=dev, loadConfig MUST succeed
// AND set JWTSecretFallbackDev=true so main() knows to emit the
// WARN. The dev secret must be the well-known devJWTSecret
// constant — we don't compare for equality because the value is
// deliberately kept in one place, but we do assert non-empty.
func TestLoadConfig_JWTSecret_Unset_Dev_Fallback(t *testing.T) {
	t.Setenv("JWT_SECRET", "")
	t.Setenv("OE2EE_ENV", "dev")

	cfg, err := loadConfig()
	require.NoError(t, err, "loadConfig must succeed when OE2EE_ENV=dev opens the fallback")
	assert.True(t, cfg.JWTSecretFallbackDev,
		"JWTSecretFallbackDev must be true so main() emits the WARN log")
	assert.NotEmpty(t, cfg.JWTSecret,
		"JWTSecret must be populated with the dev default, not empty")
	assert.Equal(t, []byte(devJWTSecret), cfg.JWTSecret,
		"JWTSecret must equal the documented devJWTSecret constant")
}

// TestLoadConfig_JWTSecret_Set_NoFallback is case (3). With
// JWT_SECRET set, loadConfig MUST succeed and
// JWTSecretFallbackDev MUST be false regardless of OE2EE_ENV.
func TestLoadConfig_JWTSecret_Set_NoFallback(t *testing.T) {
	t.Setenv("JWT_SECRET", "this-is-a-real-32-byte-min-secret!")
	t.Setenv("OE2EE_ENV", "production")

	cfg, err := loadConfig()
	require.NoError(t, err)
	assert.False(t, cfg.JWTSecretFallbackDev,
		"JWTSecretFallbackDev must be false when JWT_SECRET is explicitly set")
	assert.Equal(t, []byte("this-is-a-real-32-byte-min-secret!"), cfg.JWTSecret,
		"JWTSecret must echo the env value verbatim")
}

// TestLoadConfig_JWTSecret_Set_OverridesDevMode is case (4) —
// the regression pin. Even with OE2EE_ENV=dev, an explicit
// JWT_SECRET MUST take precedence over the dev fallback. This
// stops a misconfigured dev deploy from silently downgrading to
// the dev secret when the operator intended to set a real one.
func TestLoadConfig_JWTSecret_Set_OverridesDevMode(t *testing.T) {
	t.Setenv("JWT_SECRET", "real-secret-overrides-dev-mode")
	t.Setenv("OE2EE_ENV", "dev")

	cfg, err := loadConfig()
	require.NoError(t, err)
	assert.False(t, cfg.JWTSecretFallbackDev,
		"explicit JWT_SECRET must beat the dev fallback even in dev mode")
	assert.Equal(t, []byte("real-secret-overrides-dev-mode"), cfg.JWTSecret,
		"explicit JWT_SECRET value must be preserved verbatim")
}

// TestLoadConfig_JWTSecret_Whitespace_RejectedTreatedAsUnset —
// trim is intentional. A whitespace-only JWT_SECRET must NOT be
// accepted as a real secret; it must either fall back (dev) or
// fail (non-dev) just like the unset case. This pins the
// `strings.TrimSpace` contract at the call site.
func TestLoadConfig_JWTSecret_Whitespace_RejectedTreatedAsUnset(t *testing.T) {
	t.Setenv("JWT_SECRET", "   ")
	t.Setenv("OE2EE_ENV", "production")

	_, err := loadConfig()
	require.Error(t, err,
		"whitespace-only JWT_SECRET must NOT be accepted as a real secret in production")
}

// TestIsDevMode is a sanity check on the dev-mode gate. The
// implementation trims whitespace before comparing — this is the
// same `strings.TrimSpace` discipline applied to JWT_SECRET in
// loadConfig() so an operator who copy-pastes `OE2EE_ENV=dev `
// from a shell snippet (with a trailing space) still gets dev
// mode. The match is case-sensitive: "DEV" / "Dev" / etc. must
// NOT open the fallback.
func TestIsDevMode(t *testing.T) {
	cases := []struct {
		env  string
		want bool
	}{
		{"", false},
		{"dev", true},
		{"dev ", true},    // trailing space — trimmed, opens
		{" dev", true},    // leading space — trimmed, opens
		{"  dev  ", true}, // both — trimmed, opens
		{"production", false},
		{"prod", false},
		{"DEV", false},           // case-sensitive on purpose
		{"Dev", false},           // case-sensitive on purpose
		{"development", false},   // prefix match NOT accepted
		{" development ", false}, // internal text "dev" but trimmed ≠ "dev"
	}
	for _, tc := range cases {
		tc := tc
		name := "unset"
		if tc.env != "" {
			name = tc.env
		}
		t.Run(name, func(t *testing.T) {
			t.Setenv("OE2EE_ENV", tc.env)
			assert.Equal(t, tc.want, isDevMode(),
				"isDevMode(OE2EE_ENV=%q) should be %v", tc.env, tc.want)
		})
	}
}

// TestLogJWTSecretPosture_DevFallback_EmitsWarn is case (5).
// When JWTSecretFallbackDev is true, logJWTSecretPosture MUST
// emit a WARN-level record with:
//
//   - `fallback_dev=true` structured field (observability contract)
//   - "DEV FALLBACK" in the msg (visibility for humans reading
//     raw JSON logs in kubectl/docker)
//
// We assert against a parsed JSON record so the structured-field
// contract is pinned, not just string-matched. If a future
// refactor moves the field name, this test will fail loudly.
func TestLogJWTSecretPosture_DevFallback_EmitsWarn(t *testing.T) {
	t.Setenv("OE2EE_ENV", "dev")

	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		// LevelDebug so even if someone lowers the global
		// handler minimum the test still captures the WARN.
		Level: slog.LevelDebug,
	}))
	cfg := Config{
		JWTSecret:            []byte(devJWTSecret),
		JWTSecretFallbackDev: true,
	}

	logJWTSecretPosture(logger, cfg)

	require.NotEmpty(t, buf.String(), "log buffer must contain a record")
	// JSON handler writes one record per line.
	records := bytes.Split(bytes.TrimSpace(buf.Bytes()), []byte("\n"))
	require.Len(t, records, 1, "exactly one log record expected")

	var rec map[string]any
	require.NoError(t, json.Unmarshal(records[0], &rec),
		"record must be valid JSON so log shippers can parse it")

	assert.Equal(t, "WARN", rec["level"],
		"record must log at WARN level so ops alerting picks it up")
	assert.Equal(t, true, rec["fallback_dev"],
		"record must include structured fallback_dev=true field")
	assert.Equal(t, "dev", rec["oe2ee_env"],
		"record must echo the OE2EE_ENV value at the moment of fallback")
	msg, ok := rec["msg"].(string)
	require.True(t, ok, "msg must be a string")
	assert.Contains(t, msg, "DEV FALLBACK",
		"msg must include the visible DEV FALLBACK marker so the record is greppable in raw log output")
}

// TestLogJWTSecretPosture_NoDevFallback_Silent is case (6). When
// JWTSecretFallbackDev is false, logJWTSecretPosture MUST be a
// no-op. Production starts stay quiet on the happy path — this
// also stops a future "always log the JWT posture" change from
// regressing into noise.
func TestLogJWTSecretPosture_NoDevFallback_Silent(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	cfg := Config{
		JWTSecret:            []byte("real-secret-not-the-dev-default"),
		JWTSecretFallbackDev: false,
	}

	logJWTSecretPosture(logger, cfg)

	assert.Empty(t, buf.String(),
		"logJWTSecretPosture must be a no-op when JWTSecretFallbackDev is false — production starts stay quiet")
}

// TestLogJWTSecretPosture_EmitsValidJSON is a defensive sanity
// check: every record the helper emits must be valid JSON with
// the exact field names ops dashboards / alerting extract. The
// observability contract from the comment on
// logJWTSecretPosture is:
//
//	level       = "WARN"
//	fallback_dev = true   (boolean)
//	oe2ee_env    = <string>
//	msg         contains "DEV FALLBACK"
//
// If anyone renames the structured field, this test breaks.
func TestLogJWTSecretPosture_EmitsValidJSON(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	cfg := Config{
		JWTSecret:            []byte(devJWTSecret),
		JWTSecretFallbackDev: true,
	}

	logJWTSecretPosture(logger, cfg)

	out := strings.TrimSpace(buf.String())
	require.NotEmpty(t, out, "expected one record")
	// Cheap-but-effective parse: every field the contract
	// promises must be present with the right JSON shape.
	var rec map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &rec))
	assert.Equal(t, "WARN", rec["level"])
	assert.Equal(t, true, rec["fallback_dev"])
	assert.IsType(t, "", rec["oe2ee_env"])
}
