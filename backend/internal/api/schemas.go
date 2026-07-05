package api

// schemas.go — load and expose the JSON-Schema definitions that the
// REST handlers validate incoming payloads against.
//
// We embed the schemas at compile time via //go:embed. The source
// files live in this package's schemas/ subdirectory, which is a
// curated copy of ../../../shared/schemas/. The drift check in
// schema_test.go (TestSchemasMatchSharedDirectory) compares the
// embedded bytes to the upstream shared/schemas/ files at every
// test run so the two never silently diverge.
//
// To refresh the embedded copies after editing
// /shared/schemas/*.json, run:
//
//	# PowerShell (Windows)
//	Copy-Item ..\..\..\shared\schemas\*.json internal\api\schemas\ -Force
//
//	# bash (Linux / macOS / WSL)
//	cp ../../../shared/schemas/*.json internal/api/schemas/
//
// PRIVACY: the schema files are PUBLIC (they're a contract between
// mobile/web and backend) and contain NO user data. They are safe
// to embed and ship in the binary.

import (
	_ "embed"
	"fmt"

	"github.com/xeipuuv/gojsonschema"
)

// Schema names — also used as the on-the-wire `schema` field in
// validation-error responses and as the second segment of the
// internal validator cache key.
const (
	SchemaTelemetry        = "telemetry"
	SchemaSession          = "session"
	SchemaSessionCreate    = "session-create"
	SchemaOperatorLookup   = "operator-lookup"
	SchemaP2PSignalling    = "p2p-signalling"
)

//go:embed schemas/telemetry.schema.json
var telemetrySchemaJSON []byte

//go:embed schemas/session.schema.json
var sessionSchemaJSON []byte

//go:embed schemas/session-create.schema.json
var sessionCreateSchemaJSON []byte

//go:embed schemas/operator-lookup.schema.json
var operatorLookupSchemaJSON []byte

//go:embed schemas/p2p-signalling.schema.json
var p2pSignallingSchemaJSON []byte

// schemaSet is the runtime cache of compiled JSON-Schema validators.
// We compile once at startup so each request only pays the
// unmarshal + validation cost, not the JSON-parse-and-walk-the-
// meta-schema cost. The compile step is small (4 schemas, each
// a few KB) and the per-request saving is meaningful for the
// telemetry hot-path.
type schemaSet struct {
	telemetry      *gojsonschema.Schema
	session        *gojsonschema.Schema
	sessionCreate  *gojsonschema.Schema
	operatorLookup *gojsonschema.Schema
	signalling     *gojsonschema.Schema // unused in REST (WS uses matching.Envelope) but exposed for completeness
}

// loadSchemas compiles every embedded schema. Called once from
// New(...). A failure here is fatal — the server cannot accept
// any request without a working telemetry validator, so we
// return the error and let the caller abort startup.
func loadSchemas() (*schemaSet, error) {
	t, err := compileSchema(SchemaTelemetry, telemetrySchemaJSON)
	if err != nil {
		return nil, fmt.Errorf("api: compile %s schema: %w", SchemaTelemetry, err)
	}
	s, err := compileSchema(SchemaSession, sessionSchemaJSON)
	if err != nil {
		return nil, fmt.Errorf("api: compile %s schema: %w", SchemaSession, err)
	}
	sc, err := compileSchema(SchemaSessionCreate, sessionCreateSchemaJSON)
	if err != nil {
		return nil, fmt.Errorf("api: compile %s schema: %w", SchemaSessionCreate, err)
	}
	o, err := compileSchema(SchemaOperatorLookup, operatorLookupSchemaJSON)
	if err != nil {
		return nil, fmt.Errorf("api: compile %s schema: %w", SchemaOperatorLookup, err)
	}
	p, err := compileSchema(SchemaP2PSignalling, p2pSignallingSchemaJSON)
	if err != nil {
		return nil, fmt.Errorf("api: compile %s schema: %w", SchemaP2PSignalling, err)
	}
	return &schemaSet{
		telemetry:      t,
		session:        s,
		sessionCreate:  sc,
		operatorLookup: o,
		signalling:     p,
	}, nil
}

// compileSchema wraps gojsonschema.NewSchema and tags the error
// with the schema name so logs and tests can pinpoint which file
// failed to compile.
func compileSchema(name string, raw []byte) (*gojsonschema.Schema, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("schema %q is empty (embed failed?)", name)
	}
	loader := gojsonschema.NewBytesLoader(raw)
	schema, err := gojsonschema.NewSchema(loader)
	if err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	return schema, nil
}

// Validate runs a JSON payload against the named schema. The
// payload is given as raw bytes — handlers decode from r.Body
// once and pass the same bytes to the validator, never letting
// the validator re-parse the struct.
//
// Returns nil on success. On failure returns a *ValidationError
// that the handler turns into a 400 with a stable JSON body.
//
// We use gojsonschema's Result.Errors for the per-field message
// list. The field paths it emits are JSON-pointer style
// (/properties/operator) — we keep them as-is so the client can
// highlight the bad field in its UI.
func (s *schemaSet) Validate(schemaName string, payload []byte) error {
	if s == nil {
		return fmt.Errorf("api: schemaSet not loaded")
	}
	var schema *gojsonschema.Schema
	switch schemaName {
	case SchemaTelemetry:
		schema = s.telemetry
	case SchemaSession:
		schema = s.session
	case SchemaSessionCreate:
		schema = s.sessionCreate
	case SchemaOperatorLookup:
		schema = s.operatorLookup
	case SchemaP2PSignalling:
		schema = s.signalling
	default:
		return fmt.Errorf("api: unknown schema %q", schemaName)
	}
	if schema == nil {
		return fmt.Errorf("api: schema %q not loaded", schemaName)
	}
	docLoader := gojsonschema.NewBytesLoader(payload)
	res, err := schema.Validate(docLoader)
	if err != nil {
		return fmt.Errorf("api: validate %s: %w", schemaName, err)
	}
	if !res.Valid() {
		return &ValidationError{
			Schema:  schemaName,
			Details: collectDetails(res.Errors()),
		}
	}
	return nil
}

// ValidationError is what Validate returns on a schema mismatch.
// It marshals to a stable 400-response body (see errors.go).
type ValidationError struct {
	Schema  string            `json:"schema"`
	Details []ValidationIssue `json:"details"`
}

// Error implements error so *ValidationError can flow up through
// the handler chain without a wrapping allocation.
func (e *ValidationError) Error() string {
	if e == nil {
		return ""
	}
	return "api: schema " + e.Schema + ": " + e.Details[0].Message
}

// ValidationIssue is one row of the per-field error list.
type ValidationIssue struct {
	Field   string `json:"field"`            // JSON-pointer path
	Message string `json:"message"`          // human-readable
}

// collectDetails flattens gojsonschema's nested Result.Errors
// into one slice of (field, message). We deliberately drop the
// "context" sub-object because it duplicates the field path and
// doubles the response size.
func collectDetails(errs []gojsonschema.ResultError) []ValidationIssue {
	out := make([]ValidationIssue, 0, len(errs))
	for _, e := range errs {
		out = append(out, ValidationIssue{
			Field:   e.Field(),
			Message: e.Description(),
		})
	}
	return out
}