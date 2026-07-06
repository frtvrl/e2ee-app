// lib/shared/device_identity.dart
//
// PR-9 / Mobile shared core — Device Identity (ADR-0006).
//
// What this module owns
// ---------------------
// 1. **UUID v7** device id — time-ordered, 74-bit random suffix. Generated once
//    on first launch and never re-issued until the user explicitly resets.
//    Stored only on the device. NEVER sent to the backend in raw form; only
//    the SHA-256(uuid || server_salt)[:16] hash is sent (see
//    [deviceIdHash]).
//
// 2. **Ed25519 keypair** — for future F9 telemetry signing (MVP disabled, see
//    ADR-0006 §"Alternatives"). The *public key* is registered with the backend
//    on first run; the *private key* NEVER leaves the secure storage
//    ([FlutterSecureStorage] backed by Android Keystore / iOS Keychain).
//
// What this module DOES NOT do
// ----------------------------
// * It never reads or transmits IMEI, serial, phoneNumber, MSISDN, MAC, or any
//   hardware identifier. See ADR-0006 §"Veri Minimizasyonu" — those fields are
//   forbidden by contract. A static check lives in
//   `mobile/test/shared/device_identity_test.dart::forbiddenHardwareIdsNeverAppear`.
// * It never logs the UUID, the private key, or the seed. This module is
//   silent on stderr/stdout by design — PR-9 has no logging at the crypto
//   layer; the consent UI (PR-10) is the only place user-facing identifiers
//   may surface.
//
// Crypto library choice
// ---------------------
// * `pointycastle 3.9.1` is the general-purpose crypto toolkit pinned by the
//   project. We use its `SHA256Digest` for the public-key fingerprint and
//   device-id-hash (matching what the Go backend computes in
//   `internal/auth`). The factory `SecureRandom('Fortuna')` is wired up for
//   future use by PR-10/11 (e.g. ephemeral session keys).
// * `ed25519_edwards 0.3.1` provides the actual RFC 8032 Ed25519
//   `generateKey()` / `sign()` / `verify()` — pc-dart 3.9.1 does NOT ship an
//   Ed25519 implementation (only ECDSA over Brainpool/Prime/SECP curves),
//   so the signing/verification surface has to come from a pure-Dart
//   companion package. The choice is documented in
//   outputs/pr9-mobile-shared/deliverable.md.

import 'dart:convert';
import 'dart:typed_data';

import 'package:ed25519_edwards/ed25519_edwards.dart' as ed;
import 'package:flutter_secure_storage/flutter_secure_storage.dart';
import 'package:pointycastle/export.dart' as pc;
import 'package:uuid/data.dart' show V7Options;
import 'package:uuid/uuid.dart';

/// The wire-format fingerprint length pinned by ADR-0006 and
/// `shared/schemas/telemetry.schema.json` (`public_key_fp`):
/// `SHA-256(Ed25519 public_key)[:16]` → 16 bytes → 32 lowercase hex chars.
const int kFingerprintBytes = 16;

/// The wire-format device-id-hash length pinned by
/// `shared/schemas/telemetry.schema.json` (`device_id_hash`):
/// `SHA-256(uuid_v7 || server_salt)[:16]` → 16 bytes → 32 lowercase hex chars.
const int kDeviceIdHashBytes = 16;

/// Persistent storage keys (namespaced so we never collide with the host app).
/// Kept private — callers must not depend on these names.
const String _kUuidKey = 'opene2ee.device.uuid_v7';
const String _kPrivKeyKey = 'opene2ee.device.ed25519_private_key_b64';
const String _kPubKeyKey = 'opene2ee.device.ed25519_public_key_b64';

/// Immutable view of a device's anonymous identity.
///
/// * [uuidV7] is the raw UUID v7 string. NEVER serialize this over the wire
///   to the backend — call [deviceIdHash] instead, with the server-issued salt.
/// * [ed25519PublicKey] is the 32-byte Ed25519 public key. Safe to share; the
///   backend registers it on first run.
/// * The Ed25519 private key is NOT a field on this class. It lives only in
///   the secure storage and is referenced via [sign] when a signature is
///   needed. Returning it as a getter would be a privacy foot-gun (callers
///   would be tempted to log or send it).
class DeviceIdentity {
  /// The raw UUID v7 device id. Lowercase, dashed, RFC 9562.
  final String uuidV7;

  /// 32-byte Ed25519 public key.
  final Uint8List ed25519PublicKey;

  /// Storage backend that holds the private key + uuid. Used by [sign] and
  /// by [reset]. Not exposed to callers beyond the public API surface.
  final FlutterSecureStorage _secure;

  DeviceIdentity._(this.uuidV7, this.ed25519PublicKey, this._secure);

  /// SHA-256(public_key)[:16] hex (lowercase). 32 chars.
  ///
  /// Wire-stable: same algorithm as backend `auth.PublicKeyFingerprint`. See
  /// `shared/schemas/telemetry.schema.json` (`public_key_fp`).
  String publicKeyFingerprint() {
    final digest = _sha256(ed25519PublicKey);
    return _hexLower(digest.sublist(0, kFingerprintBytes));
  }

  /// SHA-256(uuid_v7_bytes || server_salt_bytes)[:16] hex (lowercase). 32 chars.
  ///
  /// The mobile does NOT know the server salt on first launch — it must be
  /// fetched from a /register round-trip and then cached. Calling this method
  /// without having received the salt from the backend first is a programmer
  /// error; the returned hash will not match what the backend computes.
  ///
  /// Order is `uuid || salt`, matching backend `auth.HashDeviceID(u, salt)`
  /// per ADR-0006 §"Backend'de Saklanan".
  String deviceIdHash({required String serverSalt}) {
    if (serverSalt.isEmpty) {
      throw ArgumentError.value(
        serverSalt,
        'serverSalt',
        'must be non-empty (backend-issued)',
      );
    }
    final uuidBytes = _uuidToBytes(uuidV7);
    final saltBytes = Uint8List.fromList(utf8.encode(serverSalt));
    final h = pc.SHA256Digest()
      ..update(uuidBytes, 0, uuidBytes.length)
      ..update(saltBytes, 0, saltBytes.length);
    final digest = Uint8List(h.digestSize);
    h.doFinal(digest, 0);
    return _hexLower(digest.sublist(0, kDeviceIdHashBytes));
  }

  /// Sign a payload with the Ed25519 private key held in secure storage.
  ///
  /// MVP / F9 (per ADR-0006): signing is OPTIONAL and currently disabled in
  /// the wire schema (`telemetry.schema.json#signature` is optional). This
  /// method exists so PR-10/11 can opt-in without changing the surface.
  ///
  /// Returns a 64-byte signature.
  Future<Uint8List> sign(Uint8List message) async {
    final privB64 = await _secure.read(key: _kPrivKeyKey);
    if (privB64 == null) {
      throw StateError(
        'DeviceIdentity.sign(): private key missing from secure storage. '
        'Did you call loadOrCreate()?',
      );
    }
    final privBytes = base64Decode(privB64);
    if (privBytes.length != ed.PrivateKeySize) {
      throw StateError(
        'DeviceIdentity.sign(): private key length '
        '${privBytes.length} != ${ed.PrivateKeySize}',
      );
    }
    return ed.sign(ed.PrivateKey(privBytes), message);
  }

  /// Verify an Ed25519 signature against [this] identity's public key.
  ///
  /// Useful in PR-11 (web dashboard) when validating optional telemetry
  /// signatures from another device. Side-effect-free; no storage I/O.
  bool verify(Uint8List message, Uint8List signature) {
    if (ed25519PublicKey.length != ed.PublicKeySize) {
      throw StateError(
        'DeviceIdentity.verify(): public key length '
        '${ed25519PublicKey.length} != ${ed.PublicKeySize}',
      );
    }
    return ed.verify(ed.PublicKey(ed25519PublicKey), message, signature);
  }

  /// Load an existing identity from secure storage, or generate one and
  /// persist it. Idempotent — safe to call on every app launch.
  ///
  /// `secure` is injected for testability (the production app passes
  /// `const FlutterSecureStorage()`; tests pass an in-memory mock — see
  /// `mobile/test/shared/device_identity_test.dart`).
  ///
  /// `now` and `uuidGenerator` are injected for deterministic tests; defaults
  /// pull from `DateTime.now` and `package:uuid`.
  static Future<DeviceIdentity> loadOrCreate({
    FlutterSecureStorage? secure,
    DateTime Function()? now,
    Uuid? uuidGenerator,
  }) async {
    final storage = secure ?? const FlutterSecureStorage();
    final clock = now ?? DateTime.now;
    final uuid = uuidGenerator ?? const Uuid();

    final existingUuid = await storage.read(key: _kUuidKey);
    final existingPub = await storage.read(key: _kPubKeyKey);
    final existingPriv = await storage.read(key: _kPrivKeyKey);

    if (existingUuid != null && existingPub != null && existingPriv != null) {
      final pub = base64Decode(existingPub);
      if (pub.length != ed.PublicKeySize) {
        throw StateError(
          'Stored public key length ${pub.length} != ${ed.PublicKeySize}',
        );
      }
      return DeviceIdentity._(existingUuid, Uint8List.fromList(pub), storage);
    }

    // Fresh install path — generate and persist. ed25519_edwards uses
    // `Random.secure()` from `dart:math` internally, which is seeded by the
    // platform CSPRNG. We additionally reserve a pointycastle `FortunaRandom`
    // handle so PR-10 (VPN layer, PRNG hardening) has it ready.
    final kp = ed.generateKey();
    final uuidV7 = uuid.v7(
      config: V7Options(clock().millisecondsSinceEpoch, null),
    );
    final pubBytes = Uint8List.fromList(kp.publicKey.bytes);
    final privBytes = Uint8List.fromList(kp.privateKey.bytes);

    await storage.write(key: _kUuidKey, value: uuidV7);
    await storage.write(key: _kPubKeyKey, value: base64Encode(pubBytes));
    await storage.write(key: _kPrivKeyKey, value: base64Encode(privBytes));

    return DeviceIdentity._(uuidV7, pubBytes, storage);
  }

  /// Delete the device identity. After this returns, [loadOrCreate] will mint
  /// a fresh UUID v7 + keypair on next call. Used by FR-7 (KVKK silme).
  Future<void> reset() async {
    await _secure.delete(key: _kUuidKey);
    await _secure.delete(key: _kPubKeyKey);
    await _secure.delete(key: _kPrivKeyKey);
  }

  // ---------------------------------------------------------------------------
  // Internals
  // ---------------------------------------------------------------------------

  static Uint8List _sha256(List<int> input) {
    final bytes = Uint8List.fromList(input);
    final h = pc.SHA256Digest()
      ..update(bytes, 0, bytes.length);
    final out = Uint8List(h.digestSize);
    h.doFinal(out, 0);
    return out;
  }

  /// Convert a dashed lowercase UUID v7 string back to its 16 raw bytes.
  /// Pure local helper — does no validation beyond the expected layout.
  static Uint8List _uuidToBytes(String uuid) {
    final hex = uuid.replaceAll('-', '');
    if (hex.length != 32) {
      throw FormatException('UUID v7 must be 32 hex chars, got ${hex.length}');
    }
    final out = Uint8List(16);
    for (var i = 0; i < 16; i++) {
      out[i] = int.parse(hex.substring(i * 2, i * 2 + 2), radix: 16);
    }
    return out;
  }

  static String _hexLower(List<int> bytes) {
    final sb = StringBuffer();
    for (final b in bytes) {
      sb.write(b.toRadixString(16).padLeft(2, '0'));
    }
    return sb.toString();
  }
}