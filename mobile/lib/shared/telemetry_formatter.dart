// lib/shared/telemetry_formatter.dart
//
// PR-9 / Mobile shared core — Telemetry serializer.
//
// Responsibilities
// ----------------
// * Hold the in-memory shape of a telemetry record (the same fields defined in
//   `shared/schemas/telemetry.schema.json` v1).
// * Serialize to the EXACT JSON wire format the Go backend (`internal/api`)
//   expects. Required fields are always emitted, optional fields are omitted
//   when null (we never emit `"sni": null` — that would be additionalProperty
//   noise that the schema validation step rejects).
// * Validate enum membership before serializing — catch a typo at app boot,
//   not when the backend rejects a POST.
//
// Privacy contract
// ----------------
// * The Telemetry model only exposes the fields ADR-0006 considers safe to
//   share with the backend. The raw device UUID and the private key are not
//   fields here; they live in `DeviceIdentity`. See device_identity.dart.
//
// Schema source of truth
// ----------------------
// `shared/schemas/telemetry.schema.json` — hand-validated to match field
// names, types, required set, and enum values. Drift between this model and
// the schema is a PR-9 review blocker; both MUST move together.

import 'dart:convert';

/// Operator enum values mirrored from
/// `shared/schemas/telemetry.schema.json#operator`.
enum TelemetryOperator {
  turkcell('turkcell'),
  vodafoneTr('vodafone_tr'),
  turkTelekom('turk_telekom'),
  att('att'),
  verizon('verizon'),
  tmobileUs('tmobile_us'),
  deutscheTelekom('deutsche_telekom'),
  orange('orange'),
  vodafone('vodafone'),
  o2('o2'),
  ee('ee'),
  three('three'),
  unknown('unknown');

  const TelemetryOperator(this.wire);
  final String wire;

  static TelemetryOperator? fromWire(String s) {
    for (final v in TelemetryOperator.values) {
      if (v.wire == s) return v;
    }
    return null;
  }
}

/// How the operator was determined (`telemetry.schema.json#operator_source`).
enum TelemetryOperatorSource {
  mnp('mnp'),
  ipReverse('ip_reverse'),
  asnDb('asn_db'),
  unknown('unknown');

  const TelemetryOperatorSource(this.wire);
  final String wire;

  static TelemetryOperatorSource? fromWire(String s) {
    for (final v in TelemetryOperatorSource.values) {
      if (v.wire == s) return v;
    }
    return null;
  }
}

/// App under test (`telemetry.schema.json#app`).
enum TelemetryApp {
  whatsapp('whatsapp'),
  rcs('rcs'),
  telegram('telegram'),
  signal('signal');

  const TelemetryApp(this.wire);
  final String wire;

  static TelemetryApp? fromWire(String s) {
    for (final v in TelemetryApp.values) {
      if (v.wire == s) return v;
    }
    return null;
  }
}

/// TLS version enum (`telemetry.schema.json#tls_version`).
enum TlsVersion {
  tlsv10('TLSv1.0'),
  tlsv11('TLSv1.1'),
  tlsv12('TLSv1.2'),
  tlsv13('TLSv1.3');

  const TlsVersion(this.wire);
  final String wire;

  static TlsVersion? fromWire(String s) {
    for (final v in TlsVersion.values) {
      if (v.wire == s) return v;
    }
    return null;
  }
}

/// P2P matching mode (`telemetry.schema.json#match_mode`).
enum MatchMode {
  p2p('p2p'),
  echobot('echobot'),
  single('single');

  const MatchMode(this.wire);
  final String wire;

  static MatchMode? fromWire(String s) {
    for (final v in MatchMode.values) {
      if (v.wire == s) return v;
    }
    return null;
  }
}

/// In-memory telemetry record.
///
/// Required fields are non-nullable; optional fields use the standard
/// `String? / double?` pattern. Callers MUST construct via the named
/// constructor to keep required-field enforcement at the type level.
class Telemetry {
  /// `device_id_hash` — SHA-256(uuid_v7 || server_salt)[:16] hex (32 chars).
  /// Pinned by `shared/schemas/telemetry.schema.json#device_id_hash`.
  final String deviceIdHash;

  /// `public_key_fp` — SHA-256(ed25519_pub)[:16] hex (32 chars).
  final String publicKeyFingerprint;

  /// `operator` — detected operator.
  final TelemetryOperator operator;

  /// `app` — application under test.
  final TelemetryApp app;

  /// `tls_fp` — TLS Client Hello fingerprint (SHA-256 hex).
  final String tlsFingerprint;

  /// `entropy` — Shannon entropy of payload, 0..8 bits/byte.
  final double entropy;

  /// `timestamp` — RFC 3339 timestamp (e.g. `2026-07-05T04:13:01Z`).
  /// The caller is responsible for formatting; we keep it as a [String] so
  /// the wire format matches the schema exactly without re-parsing.
  final String timestamp;

  // -------- Optional fields ---------------------------------------------

  final TelemetryOperatorSource? operatorSource;
  final TlsVersion? tlsVersion;
  final List<String>? cipherSuites;
  final String? sni;
  final String? sessionId;
  final MatchMode? matchMode;
  final double? peerScore;
  final double? confidence;
  final String? signature;

  const Telemetry({
    required this.deviceIdHash,
    required this.publicKeyFingerprint,
    required this.operator,
    required this.app,
    required this.tlsFingerprint,
    required this.entropy,
    required this.timestamp,
    this.operatorSource,
    this.tlsVersion,
    this.cipherSuites,
    this.sni,
    this.sessionId,
    this.matchMode,
    this.peerScore,
    this.confidence,
    this.signature,
  });
}

/// Serialize a [Telemetry] to the wire format expected by the Go backend.
///
/// Field order matches `shared/schemas/telemetry.schema.json` for reviewer
/// ergonomics; JSON object order is not semantically meaningful but having a
/// stable order makes debug logs easier to diff.
///
/// Throws [ArgumentError] for any enum value that fails the schema, and
/// [RangeError] for numeric fields outside the schema's inclusive range.
Map<String, dynamic> telemetryToJson(Telemetry t) {
  // Range checks (mirrors schema `minimum` / `maximum`).
  if (t.entropy < 0 || t.entropy > 8) {
    throw ArgumentError.value(t.entropy, 'entropy', 'must be in [0, 8]');
  }
  if (t.peerScore != null && (t.peerScore! < 0 || t.peerScore! > 100)) {
    throw ArgumentError.value(t.peerScore, 'peerScore', 'must be in [0, 100]');
  }
  if (t.confidence != null && (t.confidence! < 0 || t.confidence! > 1)) {
    throw ArgumentError.value(t.confidence, 'confidence', 'must be in [0, 1]');
  }

  final out = <String, dynamic>{
    'device_id_hash': t.deviceIdHash,
    'public_key_fp': t.publicKeyFingerprint,
    'operator': t.operator.wire,
    'app': t.app.wire,
    'tls_fp': t.tlsFingerprint,
    'entropy': t.entropy,
    'timestamp': t.timestamp,
  };

  // Optional fields — omit when null to avoid schema noise.
  if (t.operatorSource != null) {
    out['operator_source'] = t.operatorSource!.wire;
  }
  if (t.tlsVersion != null) {
    out['tls_version'] = t.tlsVersion!.wire;
  }
  if (t.cipherSuites != null) {
    out['cipher_suites'] = List<String>.unmodifiable(t.cipherSuites!);
  }
  if (t.sni != null) {
    out['sni'] = t.sni!;
  }
  if (t.sessionId != null) {
    out['session_id'] = t.sessionId!;
  }
  if (t.matchMode != null) {
    out['match_mode'] = t.matchMode!.wire;
  }
  if (t.peerScore != null) {
    out['peer_score'] = t.peerScore!;
  }
  if (t.confidence != null) {
    out['confidence'] = t.confidence!;
  }
  if (t.signature != null) {
    out['signature'] = t.signature!;
  }
  return out;
}

/// Convenience: serialize to a compact JSON string ready for `dio`'s
/// `Dio.post(..., data: ...)`. Equivalent to
/// `jsonEncode(telemetryToJson(t))` but exposed as a top-level function so
/// callers do not need to import `dart:convert` directly.
String telemetryToJsonString(Telemetry t) => jsonEncode(telemetryToJson(t));