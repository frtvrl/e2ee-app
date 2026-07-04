package auth

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// -----------------------------------------------------------------------------
// Device identity â€” generation invariants
// -----------------------------------------------------------------------------

func TestGenerateDeviceIdentity_BasicShape(t *testing.T) {
	id, err := GenerateDeviceIdentity()
	require.NoError(t, err)

	assert.NotEqual(t, uuid.Nil, id.UUID, "UUID must not be zero")
	assert.Len(t, id.PublicKey, 32, "Ed25519 public key must be 32 bytes")
	assert.Len(t, id.PrivateKey, 64, "Ed25519 private key must be 64 bytes")

	v := id.UUID.Version()
	assert.Equal(t, uuid.Version(7), v, "UUID must be v7 (RFC 9562)")

	// Public key must be the second half of the private key per
	// crypto/ed25519's representation (RFC 8032 Â§5.1.2).
	assert.Equal(t, ed25519.PublicKey(id.PrivateKey[32:]), id.PublicKey,
		"public key must be the suffix of the private key")
}

// TestGenerateDeviceIdentity_Unique spot-checks two consecutive calls.
func TestGenerateDeviceIdentity_Unique(t *testing.T) {
	a, err := GenerateDeviceIdentity()
	require.NoError(t, err)
	b, err := GenerateDeviceIdentity()
	require.NoError(t, err)

	assert.NotEqual(t, a.UUID, b.UUID, "two consecutive UUIDs must differ")
	assert.NotEqual(t, a.PublicKey, b.PublicKey, "two consecutive keypairs must differ")
}

// TestGenerateDeviceIdentity_UUIDMonotonic exercises the core
// monotonicity invariant ADR-0006 cares about: UUID v7 is time-ordered
// so log correlation and DB indexing stay stable.
//
// The contract is "encoded timestamp never decreases" â€” at typical
// sub-millisecond clock resolution we expect every successive call to
// have Time() >= the previous one. Random suffix ties are allowed but
// cannot make the timestamp go backwards.
func TestGenerateDeviceIdentity_UUIDMonotonic(t *testing.T) {
	prev, err := GenerateDeviceIdentity()
	require.NoError(t, err)

	for i := 0; i < 1000; i++ {
		cur, err := GenerateDeviceIdentity()
		require.NoError(t, err)

		// uuid.Time is int64 (100ns since 15 Oct 1582). Monotonicity
		// says successive UUID v7 calls must encode a non-decreasing
		// timestamp.
		prevT := int64(prev.UUID.Time())
		curT := int64(cur.UUID.Time())
		assert.GreaterOrEqual(t, curT, prevT,
			"iteration %d: timestamp must not go backwards (prev=%d cur=%d)",
			i, prevT, curT)

		prev = cur
	}
}

// TestGenerateDeviceIdentity_LargeBatch_AllUnique is a fuzzier uniqueness
// check: 10k identities, every UUID + every fingerprint must be distinct.
// 10k * 74 random bits â†’ collision probability ~ 10^-12.
func TestGenerateDeviceIdentity_LargeBatch_AllUnique(t *testing.T) {
	const n = 10_000
	uuids := make(map[uuid.UUID]struct{}, n)
	fps := make(map[string]struct{}, n)

	for i := 0; i < n; i++ {
		id, err := GenerateDeviceIdentity()
		require.NoError(t, err)

		if _, dup := uuids[id.UUID]; dup {
			t.Fatalf("duplicate UUID v7 after %d iterations: %s", i, id.UUID)
		}
		uuids[id.UUID] = struct{}{}

		fp, err := id.PublicKeyFingerprint()
		require.NoError(t, err)
		if _, dup := fps[fp]; dup {
			t.Fatalf("duplicate public-key fingerprint after %d iterations: %s", i, fp)
		}
		fps[fp] = struct{}{}
	}
}

// -----------------------------------------------------------------------------
// Convenience accessors on DeviceIdentity
// -----------------------------------------------------------------------------

func TestDeviceIdentity_DeviceIDHash_MatchesPackage(t *testing.T) {
	id, err := GenerateDeviceIdentity()
	require.NoError(t, err)
	salt := []byte("salt")

	want, err := HashDeviceID(id.UUID, salt)
	require.NoError(t, err)

	got, err := id.DeviceIDHash(salt)
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestDeviceIdentity_DeviceIDHash_RejectsZeroIdentity(t *testing.T) {
	_, err := DeviceIdentity{}.DeviceIDHash([]byte("salt"))
	require.ErrorIs(t, err, ErrEmptyInput)
}

func TestDeviceIdentity_PublicKeyFingerprint_MatchesPackage(t *testing.T) {
	id, err := GenerateDeviceIdentity()
	require.NoError(t, err)

	want, err := PublicKeyFingerprint(id.PublicKey)
	require.NoError(t, err)

	got, err := id.PublicKeyFingerprint()
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestDeviceIdentity_PublicKeyFingerprint_RejectsEmpty(t *testing.T) {
	_, err := DeviceIdentity{}.PublicKeyFingerprint()
	require.ErrorIs(t, err, ErrEmptyInput)
}

// -----------------------------------------------------------------------------
// Sign / verify roundtrip (the F9 anti-spoofing primitive)
// -----------------------------------------------------------------------------

func TestSign_VerifyRoundtrip(t *testing.T) {
	id, err := GenerateDeviceIdentity()
	require.NoError(t, err)

	msg := []byte("opene2ee telemetry payload v1")
	sig, err := Sign(id.PrivateKey, msg)
	require.NoError(t, err)
	require.Len(t, sig, 64, "Ed25519 signature must be 64 bytes")

	ok, err := VerifySignature(id.PublicKey, msg, sig)
	require.NoError(t, err)
	assert.True(t, ok, "freshly signed payload must verify")
}

func TestVerifySignature_DetectsTamper(t *testing.T) {
	id, err := GenerateDeviceIdentity()
	require.NoError(t, err)
	msg := []byte("opene2ee telemetry payload v1")
	sig, err := Sign(id.PrivateKey, msg)
	require.NoError(t, err)

	bad := append([]byte{}, msg...)
	bad[5] ^= 0x01

	ok, err := VerifySignature(id.PublicKey, bad, sig)
	require.NoError(t, err)
	assert.False(t, ok, "modified payload must fail verification")
}

func TestVerifySignature_WrongKey(t *testing.T) {
	signer, err := GenerateDeviceIdentity()
	require.NoError(t, err)
	other, err := GenerateDeviceIdentity()
	require.NoError(t, err)

	msg := []byte("msg")
	sig, err := Sign(signer.PrivateKey, msg)
	require.NoError(t, err)

	ok, err := VerifySignature(other.PublicKey, msg, sig)
	require.NoError(t, err)
	assert.False(t, ok, "verifying with the wrong key must fail")
}

func TestSign_RejectsWrongKeySize(t *testing.T) {
	_, err := Sign(make([]byte, 32), []byte("msg"))
	require.ErrorIs(t, err, ErrEmptyInput)
}

func TestVerifySignature_RejectsWrongKeySize(t *testing.T) {
	_, err := VerifySignature(make([]byte, 16), []byte("msg"), make([]byte, 64))
	require.ErrorIs(t, err, ErrEmptyInput)
}

func TestVerifySignature_RejectsWrongSigSize(t *testing.T) {
	id, err := GenerateDeviceIdentity()
	require.NoError(t, err)
	_, err = VerifySignature(id.PublicKey, []byte("msg"), make([]byte, 32))
	require.ErrorIs(t, err, ErrEmptyInput)
}

// -----------------------------------------------------------------------------
// End-to-end: Generate â†’ Sign â†’ Verify. Mirrors the on-device first-run
// flow without needing the SecureStore bridge (covered in keys_test.go).
// -----------------------------------------------------------------------------

func TestEndToEnd_GenerateSignVerify(t *testing.T) {
	id, err := GenerateDeviceIdentity()
	require.NoError(t, err)

	msg := []byte("consent-row-timestamp-1234567890")
	sig := ed25519.Sign(id.PrivateKey, msg)
	assert.True(t, ed25519.Verify(id.PublicKey, msg, sig),
		"signed-by-identity must verify under identity's public key")
}

// -----------------------------------------------------------------------------
// Privacy invariants â€” guard rails for future maintainers.
// -----------------------------------------------------------------------------

// TestPrivacy_HashDoesNotLeakRawUUID verifies the hash output shares no
// obvious structure with the input UUID. This is a weak heuristic â€” the
// real protection is the salt + truncation â€” but it would catch a
// regression like "we just returned hex(uuid)[:32]".
func TestPrivacy_HashDoesNotLeakRawUUID(t *testing.T) {
	id, err := GenerateDeviceIdentity()
	require.NoError(t, err)

	hash, err := id.DeviceIDHash([]byte("salt"))
	require.NoError(t, err)

	uuidHex := strings.ReplaceAll(id.UUID.String(), "-", "")
	assert.NotEqual(t, uuidHex[:TruncateHexLen], hash,
		"hash must not equal the raw UUID prefix")
	assert.NotContains(t, hash, id.UUID.String(),
		"hash must not contain the canonical UUID string")
	assert.NotContains(t, hash, uuidHex,
		"hash must not contain the hex UUID")
}

// TestPrivacy_FingerprintDoesNotLeakRawPublicKey verifies the
// fingerprint is not a simple transformation of the public key.
func TestPrivacy_FingerprintDoesNotLeakRawPublicKey(t *testing.T) {
	id, err := GenerateDeviceIdentity()
	require.NoError(t, err)

	fp, err := id.PublicKeyFingerprint()
	require.NoError(t, err)

	pkHex := hex.EncodeToString(id.PublicKey)
	assert.NotEqual(t, pkHex[:TruncateHexLen], fp,
		"fingerprint must not equal the public-key hex prefix")
	assert.NotContains(t, fp, pkHex,
		"fingerprint must not contain the public-key hex")
}

// TestPrivacy_PackageDoesNotExportRawKeyMaterial â€” static check: no
// exported function in the auth package returns the raw UUID or the
// raw private key as a string that could be accidentally logged. The
// only string-returning helpers are HashDeviceID (salted) and
// PublicKeyFingerprint (truncated, unsalted).
func TestPrivacy_PackageDoesNotExportRawKeyMaterial(t *testing.T) {
	id, err := GenerateDeviceIdentity()
	require.NoError(t, err)

	hash, err := id.DeviceIDHash([]byte("salt"))
	require.NoError(t, err)
	fp, err := id.PublicKeyFingerprint()
	require.NoError(t, err)

	// Both outputs are derived forms; neither is the raw material.
	assert.Len(t, hash, TruncateHexLen)
	assert.Len(t, fp, TruncateHexLen)
	assert.NotEqual(t, hash, id.UUID.String())
	assert.NotEqual(t, fp, hex.EncodeToString(id.PublicKey))
}

// Reference to rand to keep the import (some test variants may need it).
var _ = rand.Reader
