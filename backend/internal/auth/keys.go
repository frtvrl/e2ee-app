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
// salt. Output: hex-encoded SHA-256(uuid || salt)[:TruncateBytes] —
// 32 hex characters.
//
// Input order is uuid-bytes FIRST, then salt-bytes, per ADR-0006
// §"Backend'de Saklanan". The "uuid first" ordering (a) is pinned by
// the cross-system contract in mobile/lib/shared/device_identity.dart
// so the device_id_hash a device sends at registration matches the
// row the server stores, and (b) makes SHA-256 collisions between
// different (uuid, salt) pairs and any convenience "salt-prefix"
// hash pre-existing in the codebase structurally impossible. The
// exact contract is pinned by TestHashDeviceID_KnownAnswer below.
//
// Determinism: same (uuid, salt) → same hash. Useful for idempotent
// re-registration logic in storage (UpsertDevice).
//
// Reference vector (pinned by TestHashDeviceID_KnownAnswer):
//   uuid = 01900000-0000-7000-8000-000000000001
//   salt = "opene2ee-v1-salt"
//   →    = "40903d91f8f04d77d94e3d3b8eb97483"
func HashDeviceID(u uuid.UUID, serverSalt []byte) (string, error) {
	if u == uuid.Nil {
		return "", fmt.Errorf("zero uuid: %w", ErrEmptyInput)
	}
	if len(serverSalt) == 0 {
		return "", fmt.Errorf("empty server salt: %w", ErrEmptyInput)
	}
	h := sha256.New()
	h.Write(u[:])
	h.Write(serverSalt)
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