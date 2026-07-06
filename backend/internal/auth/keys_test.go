// keys_test.go — exercises HashDeviceID and PublicKeyFingerprint.
//
// These primitives are the server-side view of ADR-0006 §Anonim Cihaz
// Kimliği. The input-order contract is pinned by known-answer tests
// below — any drift would surface immediately. The contract MUST
// match the mobile (Dart / pointycastle) implementation that ships in
// PR-9.
package auth

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// -----------------------------------------------------------------------------
// HashDeviceID
// -----------------------------------------------------------------------------

func TestHashDeviceID_Deterministic(t *testing.T) {
	salt := []byte("test-salt-v1")
	id, err := GenerateDeviceIdentity()
	require.NoError(t, err)

	h1, err := HashDeviceID(id.UUID, salt)
	require.NoError(t, err)
	h2, err := HashDeviceID(id.UUID, salt)
	require.NoError(t, err)

	assert.Equal(t, h1, h2, "same (uuid, salt) must produce same hash")
	assert.Len(t, h1, TruncateHexLen, "hash must be %d hex chars", TruncateHexLen)
}

func TestHashDeviceID_DifferentSalts(t *testing.T) {
	id, err := GenerateDeviceIdentity()
	require.NoError(t, err)

	h1, err := HashDeviceID(id.UUID, []byte("salt-a"))
	require.NoError(t, err)
	h2, err := HashDeviceID(id.UUID, []byte("salt-b"))
	require.NoError(t, err)

	assert.NotEqual(t, h1, h2, "different salts must produce different hashes")
}

func TestHashDeviceID_DifferentUUIDs(t *testing.T) {
	salt := []byte("test-salt")
	a, _ := GenerateDeviceIdentity()
	b, _ := GenerateDeviceIdentity()

	h1, err := HashDeviceID(a.UUID, salt)
	require.NoError(t, err)
	h2, err := HashDeviceID(b.UUID, salt)
	require.NoError(t, err)

	assert.NotEqual(t, h1, h2, "different UUIDs must produce different hashes")
}

func TestHashDeviceID_RejectsZeroInputs(t *testing.T) {
	t.Run("zero uuid", func(t *testing.T) {
		_, err := HashDeviceID(uuid.Nil, []byte("salt"))
		require.ErrorIs(t, err, ErrEmptyInput)
	})
	t.Run("nil salt", func(t *testing.T) {
		_, err := HashDeviceID(uuid.New(), nil)
		require.ErrorIs(t, err, ErrEmptyInput)
	})
	t.Run("empty salt []byte{}", func(t *testing.T) {
		_, err := HashDeviceID(uuid.New(), []byte{})
		require.ErrorIs(t, err, ErrEmptyInput)
	})
}

func TestHashDeviceID_LowercaseHex(t *testing.T) {
	id, err := GenerateDeviceIdentity()
	require.NoError(t, err)

	h, err := HashDeviceID(id.UUID, []byte("salt"))
	require.NoError(t, err)

	assert.Equal(t, strings.ToLower(h), h, "hash must be lowercase hex")
	assert.Len(t, h, TruncateHexLen, "hash must be %d hex chars", TruncateHexLen)
}

// TestHashDeviceID_KnownAnswer pins the SHA-256 + truncation contract.
// A regression that changes the input order, the SHA-256 algorithm, or
// the truncation length would surface here.
//
// Reference vector computed offline (uuid-bytes || salt-bytes, per
// ADR-0006 §Backend'de Saklanan — "SHA-256(uuid_v7 + server_salt)[:16]"):
//   uuid  = 01900000-0000-7000-8000-000000000001
//   salt  = "opene2ee-v1-salt"  (15 bytes)
//   input = 01 90 00 00 00 00 70 00 80 00 00 00 00 00 00 01
//         || 6f 70 65 6e 65 32 65 65 2d 76 31 2d 73 61 6c 74
//   SHA-256(input)[:16] hex:
//   → 40903d91f8f04d77d94e3d3b8eb97483
//
// IMPORTANT: this is the value the SERVER computes. The MOBILE side
// MUST use the same input order — uuid first, salt second — or the
// hashes will not match the row stored at registration time.
func TestHashDeviceID_KnownAnswer(t *testing.T) {
	u, err := uuid.Parse("01900000-0000-7000-8000-000000000001")
	require.NoError(t, err)
	salt := []byte("opene2ee-v1-salt")

	const expected = "40903d91f8f04d77d94e3d3b8eb97483"
	got, err := HashDeviceID(u, salt)
	require.NoError(t, err)
	assert.Equal(t, expected, got,
		"HashDeviceID drifted from known vector — check input order (uuid || salt) "+
			"and SHA-256 + 16-byte truncation")
}

// -----------------------------------------------------------------------------
// PublicKeyFingerprint
// -----------------------------------------------------------------------------

func TestPublicKeyFingerprint_Deterministic(t *testing.T) {
	id, err := GenerateDeviceIdentity()
	require.NoError(t, err)

	fp1, err := PublicKeyFingerprint(id.PublicKey)
	require.NoError(t, err)
	fp2, err := PublicKeyFingerprint(id.PublicKey)
	require.NoError(t, err)

	assert.Equal(t, fp1, fp2)
	assert.Len(t, fp1, TruncateHexLen)
}

func TestPublicKeyFingerprint_UniquePerKey(t *testing.T) {
	a, _ := GenerateDeviceIdentity()
	b, _ := GenerateDeviceIdentity()

	fpA, err := PublicKeyFingerprint(a.PublicKey)
	require.NoError(t, err)
	fpB, err := PublicKeyFingerprint(b.PublicKey)
	require.NoError(t, err)

	assert.NotEqual(t, fpA, fpB)
}

func TestPublicKeyFingerprint_RejectsWrongSize(t *testing.T) {
	for _, sz := range []int{0, 16, 31, 33, 64} {
		_, err := PublicKeyFingerprint(make([]byte, sz))
		require.ErrorIs(t, err, ErrEmptyInput,
			"PublicKeyFingerprint(%d bytes) must reject", sz)
	}
}

// TestPublicKeyFingerprint_KnownAnswer pins the fingerprint format.
//
// 32 zero bytes → SHA-256 = 66687aadf862bd776c8fc18b8e9f8e20...
// Truncated to 16 bytes hex: 66687aadf862bd776c8fc18b8e9f8e20
func TestPublicKeyFingerprint_KnownAnswer(t *testing.T) {
	const expected = "66687aadf862bd776c8fc18b8e9f8e20"
	got, err := PublicKeyFingerprint(make([]byte, 32))
	require.NoError(t, err)
	assert.Equal(t, expected, got)
}

// -----------------------------------------------------------------------------
// Salt rotation — same UUID, different salt → different hash.
// (Regression-guard against accidentally dropping the salt from the input.)
// -----------------------------------------------------------------------------

func TestHashDeviceID_SaltRotationChangesHash(t *testing.T) {
	id, err := GenerateDeviceIdentity()
	require.NoError(t, err)

	h2024, err := HashDeviceID(id.UUID, []byte("opene2ee-salt-2024"))
	require.NoError(t, err)
	h2025, err := HashDeviceID(id.UUID, []byte("opene2ee-salt-2025"))
	require.NoError(t, err)
	h2026, err := HashDeviceID(id.UUID, []byte("opene2ee-salt-2026"))
	require.NoError(t, err)

	assert.NotEqual(t, h2024, h2025, "yearly salt rotation must change the hash")
	assert.NotEqual(t, h2025, h2026)
	assert.NotEqual(t, h2024, h2026)
}