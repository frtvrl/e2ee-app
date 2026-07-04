// keys.go — server-side hash + fingerprint primitives for ADR-0006.
//
// Contains the two free functions the REST layer (PR-7) needs to
// derive the storage-stable identifiers from a freshly-registered
// device:
//
//   - HashDeviceID(uuid, salt)         → device_id_hash (server PK)
//   - PublicKeyFingerprint(pubkey)    → carried in every telemetry row
//
// Input order for HashDeviceID is uuid-bytes FIRST, then salt-bytes
// — matches the mobile (Dart / pointycastle) implementation so the
// hash a device sends at registration matches the row stored at
// registration time. The exact contract is pinned by
// TestHashDeviceID_KnownAnswer.
package auth

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/google/uuid"
)

// HashDeviceID derives the server-stable identifier for a UUID v7 +
// salt. Output: hex-encoded SHA-256(salt || uuid)[:TruncateBytes] —
// 32 hex characters.
//
// Input order is salt FIRST, then uuid. The "salt first" convention
// matches ADR-0006's domain-separation pattern (a salt prefix is
// distinct from any user-controllable input). The exact contract is
// pinned by TestHashDeviceID_KnownAnswer below.
//
// Determinism: same (uuid, salt) → same hash. Useful for idempotent
// re-registration logic in storage (UpsertDevice).
//
// Reference vector (pinned by TestHashDeviceID_KnownAnswer):
//   uuid = 01900000-0000-7000-8000-000000000001
//   salt = "opene2ee-v1-salt"
//   →    = "0a26ef7ed58d777eea5ccd0bc33307bb"
func HashDeviceID(u uuid.UUID, serverSalt []byte) (string, error) {
	if u == uuid.Nil {
		return "", fmt.Errorf("zero uuid: %w", ErrEmptyInput)
	}
	if len(serverSalt) == 0 {
		return "", fmt.Errorf("empty server salt: %w", ErrEmptyInput)
	}
	h := sha256.New()
	h.Write(serverSalt)
	h.Write(u[:])
	sum := h.Sum(nil)
	return hex.EncodeToString(sum[:TruncateBytes]), nil
}

// PublicKeyFingerprint returns the Ed25519 public-key fingerprint
// used in telemetry rows. ADR-0006 specifies SHA-256(public_key)[:16]
// hex — 32 hex characters.
//
// Reference vector (pinned by TestPublicKeyFingerprint_KnownAnswer):
//   pub  = 32 zero bytes
//   →    = "66687aadf862bd776c8fc18b8e9f8e20"
func PublicKeyFingerprint(pub ed25519.PublicKey) (string, error) {
	if len(pub) != ed25519.PublicKeySize {
		return "", fmt.Errorf("public key must be %d bytes, got %d: %w",
			ed25519.PublicKeySize, len(pub), ErrEmptyInput)
	}
	sum := sha256.Sum256(pub)
	return hex.EncodeToString(sum[:TruncateBytes]), nil
}