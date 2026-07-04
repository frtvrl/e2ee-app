// Package auth provides device-identity helpers for OpenE2EE.
//
// Implements the server-side view of ADR-0006 (Anonymous Device
// Identity):
//
//   - DeviceIdentity bundles a freshly-minted UUID v7 + Ed25519 keypair.
//     In normal operation the device generates its own identity on
//     first boot and only the public key + UUID v7 cross the network.
//     The server-side helper exists for tests, bootstrap scripts, and
//     (future) F9 anti-spoofing flows that mint identities server-side.
//   - HashDeviceID derives the server-stable device_id_hash from a
//     UUID v7 + the SERVER_SALT: SHA-256(uuid || salt)[:16] hex.
//   - PublicKeyFingerprint returns the Ed25519 public-key fingerprint
//     used in every telemetry row: SHA-256(pub)[:16] hex.
//
// PRIVACY INVARIANTS (ADR-0006 §Veri Minimizasyonu):
//
//   - The raw device UUID v7 MUST never appear in storage, telemetry,
//     or logs after registration. We only ever see the salted hash on
//     the server side.
//   - The Ed25519 private key MUST never leave the device. We never
//     persist it server-side. GenerateDeviceIdentity returns one
//     purely for local-development ergonomics and unit tests; callers
//     that wire this to the wire MUST discard the private key on the
//     server.
//
// SecureStore (Keystore / Keychain abstraction) is intentionally NOT
// in this package — it lives in the mobile client (PR-9) per the
// zero-knowledge principle in SPRINT-1-PR2-DECISIONS §1.
package auth

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"fmt"

	"github.com/google/uuid"
)

// TruncateBytes is how many bytes we keep from SHA-256 digests in this
// package. 16 bytes = 128 bits.
const TruncateBytes = 16

// TruncateHexLen is the hex-encoded length (2 * TruncateBytes).
const TruncateHexLen = 2 * TruncateBytes

// ErrEmptyInput is the universal validation error. Returned when a
// required argument is zero-length / zero-valued.
var ErrEmptyInput = errors.New("auth: empty input")

// DeviceIdentity bundles the three pieces a device owns after first
// boot: a UUID v7 (client-side only), an Ed25519 public key (sent to
// the server ONCE during registration), and an Ed25519 private key
// (NEVER leaves the device — kept here for tests and local
// development only).
type DeviceIdentity struct {
	UUID       uuid.UUID
	PublicKey  ed25519.PublicKey
	PrivateKey ed25519.PrivateKey
}

// GenerateDeviceIdentity produces a fresh identity using
// cryptographically-secure randomness.
func GenerateDeviceIdentity() (DeviceIdentity, error) {
	u, err := uuid.NewV7()
	if err != nil {
		return DeviceIdentity{}, fmt.Errorf("auth: uuid.NewV7: %w", err)
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return DeviceIdentity{}, fmt.Errorf("auth: ed25519.GenerateKey: %w", err)
	}
	return DeviceIdentity{UUID: u, PublicKey: pub, PrivateKey: priv}, nil
}

// Sign is a thin wrapper around ed25519.Sign that enforces the
// private-key length. Kept for F9 anti-spoofing (currently disabled in
// MVP per ADR-0006 §İmzalama) and for tests.
func Sign(priv ed25519.PrivateKey, message []byte) ([]byte, error) {
	if len(priv) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("private key must be %d bytes, got %d: %w",
			ed25519.PrivateKeySize, len(priv), ErrEmptyInput)
	}
	return ed25519.Sign(priv, message), nil
}

// VerifySignature is a thin wrapper around ed25519.Verify that
// enforces key and signature sizes.
func VerifySignature(pub ed25519.PublicKey, message, sig []byte) (bool, error) {
	if len(pub) != ed25519.PublicKeySize {
		return false, fmt.Errorf("public key must be %d bytes, got %d: %w",
			ed25519.PublicKeySize, len(pub), ErrEmptyInput)
	}
	if len(sig) != ed25519.SignatureSize {
		return false, fmt.Errorf("signature must be %d bytes, got %d: %w",
			ed25519.SignatureSize, len(sig), ErrEmptyInput)
	}
	return ed25519.Verify(pub, message, sig), nil
}

// -----------------------------------------------------------------------------
// Convenience accessors on DeviceIdentity
// -----------------------------------------------------------------------------

// DeviceIDHash derives the server-stable identifier for this identity
// using the given salt. Returns ErrEmptyInput on a zero-valued
// identity.
func (d DeviceIdentity) DeviceIDHash(salt []byte) (string, error) {
	if d.UUID == uuid.Nil || len(d.PublicKey) == 0 {
		return "", ErrEmptyInput
	}
	return HashDeviceID(d.UUID, salt)
}

// PublicKeyFingerprint is the method form of PublicKeyFingerprint.
// Returns ErrEmptyInput on a zero-valued identity.
func (d DeviceIdentity) PublicKeyFingerprint() (string, error) {
	if len(d.PublicKey) == 0 {
		return "", ErrEmptyInput
	}
	return PublicKeyFingerprint(d.PublicKey)
}