// mobile/lib/shared/webrtc_client.dart
//
// PR-21b / Sprint 3 — Flutter WebRTC integration.
//
// Why this module exists
// ----------------------
// The Go backend (PR-21a) ships four REST endpoints that mediate a
// WebRTC peer connection between two OpenE2EE devices:
//
//   GET   /api/v1/webrtc/config   → STUN/TURN ICE-servers
//   POST  /api/v1/webrtc/offer    → SDP offer (creates session if new)
//   POST  /api/v1/webrtc/answer   → SDP answer (connecting → connected)
//   POST  /api/v1/webrtc/ice      → one ICE candidate per call
//
// This file is the Flutter-side counterpart. It hides the native
// `RTCPeerConnection` plumbing behind a small, testable layer so the
// rest of the mobile app can drive P2P without touching
// `package:flutter_webrtc` directly.
//
// Architecture
// ------------
//  ┌─────────────────┐    ┌──────────────────────┐    ┌─────────────┐
//  │  WebRtcClient   │───▶│ PeerConnectionBridge │───▶│ flutter_    │
//  │  (state machine │    │ (interface)          │    │  webrtc     │
//  │   + REST)       │    │  Production:         │    │  (platform  │
//  │                 │    │   FlutterWebRtcBridge│    │   channel)  │
//  └─────────────────┘    │  Test:               │    └─────────────┘
//          │              │   FakePeerConnBridge │
//          ▼              └──────────────────────┘
//   ┌─────────────┐
//   │  ApiClient  │  ← POSTs to PR-21a endpoints
//   └─────────────┘
//
// Testability: the bridge is an interface — unit tests inject a fake
// that records calls and replies synchronously, so we never need a
// platform channel during `flutter test`.
//
// Privacy (ADR-0006)
// ------------------
// * SDP body text and ICE candidate strings are peer-reflexive IP+port
//   metadata. We never log them. Errors surface ONLY the session id and
//   state name; the candidate/SDP bytes stay inside this module.
// * `peer_hash` is the SHA-256 device fingerprint already used by PR-9;
//   no new identifier is collected here.
//
// Lifecycle
// ---------
// The Dart-side state machine mirrors the backend's
// `matching.SessionState`. Transitions are guarded — `createOffer` is
// only legal in [WebRtcState.idle], `applyAnswer` only in
// [WebRtcState.offering], etc. See `mobile/test/shared/webrtc_client_test.dart`
// for the table the unit tests cover.
//
// References
// ----------
// - docs/SPRINT-3-SCOPE.md §6 (PR-21b)
// - docs/ADR-0006-anonimlik.md (privacy)
// - backend/internal/matching/webrtc.go (PR-21a wire contract)

import 'dart:async';
import 'dart:typed_data';

import 'package:dio/dio.dart';
import 'package:flutter_webrtc/flutter_webrtc.dart' as rtc;

import 'api_client.dart';

// ---------------------------------------------------------------------------
// Public DTOs — match the Go-side JSON shapes exactly.
// ---------------------------------------------------------------------------

/// Lifecycle state, mirrored from `matching.SessionState`.
///
/// Wire names are pinned by the backend; `wireName` and `fromWire` are
/// the only allowed conversions to/from JSON.
enum WebRtcState {
  /// Peer connection created, no offer yet. (`new`)
  idle,

  /// Offer sent, awaiting answer. (`connecting`, offerer side)
  offering,

  /// Remote offer applied, answer not yet sent. (`connecting`, answerer side)
  connecting,

  /// Both sides exchanged SDP and at least one ICE pair. (`connected`)
  connected,

  /// Session ended normally. (`closed`)
  closed,

  /// Session aborted. (`failed`)
  failed;

  /// JSON-side name used by the backend.
  String get wireName {
    switch (this) {
      case WebRtcState.idle:
        return 'new';
      case WebRtcState.offering:
      case WebRtcState.connecting:
        return 'connecting';
      case WebRtcState.connected:
        return 'connected';
      case WebRtcState.closed:
        return 'closed';
      case WebRtcState.failed:
        return 'failed';
    }
  }

  /// Parse the backend's `state` string. Unknown values map to [failed]
  /// (defensive — the wire surface is a closed enum on our side, so a
  /// surprise value is treated like an error).
  static WebRtcState fromWire(String? wire) {
    switch (wire) {
      case 'new':
        return WebRtcState.idle;
      case 'connecting':
        // We don't know on the client side whether we are offerer or
        // answerer from the wire alone — pick `connecting` as the
        // symmetric value. The Dart-side [WebRtcClient] rewrites to
        // [offering] immediately after `createOffer` returns.
        return WebRtcState.connecting;
      case 'connected':
        return WebRtcState.connected;
      case 'closed':
        return WebRtcState.closed;
      case 'failed':
        return WebRtcState.failed;
      default:
        return WebRtcState.failed;
    }
  }

  /// Terminal states never transition further.
  bool get isTerminal =>
      this == WebRtcState.closed || this == WebRtcState.failed;
}

/// STUN/TURN config served by `GET /api/v1/webrtc/config`.
///
/// Mirrors `matching.STUNTURNConfig`. At least one STUN URL is always
/// present — the backend falls back to Google's public servers when no
/// `WEBRTC_STUN_URL` is set. TURN fields are optional.
class IceServersConfig {
  final List<String> stunUrls;
  final String? turnUrl;
  final String? turnUsername;
  final String? turnCredential;
  final int ttlSeconds;

  const IceServersConfig({
    required this.stunUrls,
    this.turnUrl,
    this.turnUsername,
    this.turnCredential,
    this.ttlSeconds = 0,
  });

  factory IceServersConfig.fromJson(Map<String, dynamic> j) {
    final stun = (j['stun_urls'] as List<dynamic>? ?? const [])
        .map((e) => e.toString())
        .toList(growable: false);
    return IceServersConfig(
      stunUrls: stun,
      turnUrl: j['turn_url'] as String?,
      turnUsername: j['turn_username'] as String?,
      turnCredential: j['turn_credential'] as String?,
      ttlSeconds: (j['ttl_seconds'] as num?)?.toInt() ?? 0,
    );
  }

  /// Render as the `iceServers` entry of `RTCConfiguration` consumed by
  /// `createPeerConnection`. Returns one entry per STUN URL, plus an
  /// optional TURN entry when TURN is configured.
  List<Map<String, dynamic>> toRtcIceServers() {
    final out = <Map<String, dynamic>>[
      for (final url in stunUrls) <String, dynamic>{'urls': url},
    ];
    if (turnUrl != null && turnUrl!.isNotEmpty) {
      out.add(<String, dynamic>{
        'urls': turnUrl,
        if (turnUsername != null && turnUsername!.isNotEmpty)
          'username': turnUsername,
        if (turnCredential != null && turnCredential!.isNotEmpty)
          'credential': turnCredential,
      });
    }
    return out;
  }
}

/// One ICE candidate, mirroring `matching.ICECandidate`.
///
/// The `candidate` string is the RFC 5245 candidate line —
/// `peer-reflexive IP+port` for `srflxt`. Per ADR-0006 it must NOT
/// appear in any logger or error message.
class IceCandidate {
  final String candidate;
  final String? sdpMid;
  final int? sdpMLineIndex;
  final String? usernameFragment;

  const IceCandidate({
    required this.candidate,
    this.sdpMid,
    this.sdpMLineIndex,
    this.usernameFragment,
  });

  factory IceCandidate.fromJson(Map<String, dynamic> j) => IceCandidate(
        candidate: j['candidate'] as String,
        sdpMid: j['sdpMid'] as String?,
        sdpMLineIndex: (j['sdpMLineIndex'] as num?)?.toInt(),
        usernameFragment: j['usernameFragment'] as String?,
      );

  Map<String, dynamic> toJson() => <String, dynamic>{
        'candidate': candidate,
        if (sdpMid != null) 'sdpMid': sdpMid,
        if (sdpMLineIndex != null) 'sdpMLineIndex': sdpMLineIndex,
        if (usernameFragment != null) 'usernameFragment': usernameFragment,
      };
}

/// SDP body, mirroring `matching.SDPPayload`.
class SdpPayload {
  final String sdpType; // "offer" | "answer"
  final String sdp;

  const SdpPayload({required this.sdpType, required this.sdp});

  factory SdpPayload.fromJson(Map<String, dynamic> j) => SdpPayload(
        sdpType: j['sdp_type'] as String,
        sdp: j['sdp'] as String,
      );

  Map<String, dynamic> toJson() => <String, dynamic>{
        'sdp_type': sdpType,
        'sdp': sdp,
      };
}

/// Canonical response from /offer, /answer, /ice handlers
/// (mirrors `matching.WebRTCSignallingResponse`).
class SignalingResponse {
  final String sessionId;
  final WebRtcState state;
  final List<IceCandidate> remoteIce;
  final List<String> peersWithIce;

  const SignalingResponse({
    required this.sessionId,
    required this.state,
    required this.remoteIce,
    required this.peersWithIce,
  });

  factory SignalingResponse.fromJson(Map<String, dynamic> j) =>
      SignalingResponse(
        sessionId: j['session_id'] as String,
        state: WebRtcState.fromWire(j['state'] as String?),
        remoteIce: ((j['remote_ice'] as List<dynamic>?) ?? const [])
            .cast<Map<String, dynamic>>()
            .map(IceCandidate.fromJson)
            .toList(growable: false),
        peersWithIce: ((j['peers_with_ice'] as List<dynamic>?) ?? const [])
            .map((e) => e.toString())
            .toList(growable: false),
      );
}

/// Tagged union for inbound DataChannel messages.
///
/// `flutter_webrtc` reports text and binary frames via the same channel;
/// we tag them at the API boundary so the caller doesn't have to
/// inspect `RTCDataChannelMessage` directly.
class WebRtcDataChannelPayload {
  final bool isBinary;
  final String? text;
  final Uint8List? binary;

  const WebRtcDataChannelPayload.text(String this.text)
      : isBinary = false,
        binary = null;
  const WebRtcDataChannelPayload.binary(Uint8List this.binary)
      : isBinary = true,
        text = null;
}

// ---------------------------------------------------------------------------
// Errors
// ---------------------------------------------------------------------------

/// Base class for `WebRtcClient` failures. Sub-classes map onto the
/// backend's `{code,message}` envelope (see PR-21a `matching.translateError`).
class WebRtcException implements Exception {
  final String message;

  /// Wire-level error code — `bad_request`, `state_transition`,
  /// `session_not_found`, `session_terminal`, etc. Null when the
  /// error originates client-side (state-machine violation, transport).
  final String? code;

  const WebRtcException(this.message, {this.code});

  @override
  String toString() =>
      'WebRtcException(${code ?? "client"}): $message';
}

/// Lifecycle / state-machine error — caller invoked an operation that
/// is illegal in the current state.
class WebRtcStateError extends WebRtcException {
  WebRtcStateError(String message) : super(message, code: 'state_error');
}

/// Backend returned 4xx/5xx. The backend envelope is the raw `{code,
/// message}` shape — `WebRtcClient` translates into the typed
/// exceptions below where possible.
class WebRtcBackendError extends WebRtcException {
  final int statusCode;
  WebRtcBackendError(this.statusCode, String message,
      {required String code})
      : super(message, code: code);
}

class WebRtcBadRequest extends WebRtcBackendError {
  WebRtcBadRequest(String message)
      : super(400, message, code: 'bad_request');
}

class WebRtcNotFound extends WebRtcBackendError {
  WebRtcNotFound(String message)
      : super(404, message, code: 'session_not_found');
}

class WebRtcConflict extends WebRtcBackendError {
  WebRtcConflict(String message, {String code = 'state_transition'})
      : super(409, message, code: code);
}

class WebRtcRateLimited extends WebRtcBackendError {
  final Duration? retryAfter;
  WebRtcRateLimited(String message, {this.retryAfter})
      : super(429, message, code: 'rate_limited');
}

class WebRtcInternalError extends WebRtcBackendError {
  WebRtcInternalError(String message)
      : super(500, message, code: 'internal_error');
}

// ---------------------------------------------------------------------------
// Bridge — abstracts flutter_webrtc so unit tests don't need a platform
// channel. Production: FlutterWebRtcBridge. Tests: FakePeerConnectionBridge.
// ---------------------------------------------------------------------------

/// The minimum surface the high-level [WebRtcClient] needs from
/// `flutter_webrtc`. The interface is deliberately narrow so a fake is
/// trivial to write.
abstract class PeerConnectionBridge {
  /// Create the underlying `RTCPeerConnection` with the supplied
  /// ICE-servers list, then prepare a single data channel.
  Future<void> initialize({
    required List<Map<String, dynamic>> iceServers,
    String dataChannelLabel,
  });

  /// Generate the local SDP offer and store it as the local description.
  Future<SdpPayload> createOffer();

  /// Store the remote SDP offer as the remote description.
  Future<void> applyOffer(SdpPayload offer);

  /// Generate the local SDP answer (after [applyOffer]) and store it
  /// as the local description.
  Future<SdpPayload> createAnswer();

  /// Store the remote SDP answer as the remote description.
  Future<void> applyAnswer(SdpPayload answer);

  /// Forward a remote ICE candidate into the local agent.
  Future<void> addRemoteCandidate(IceCandidate candidate);

  /// Receive newly-gathered local ICE candidates.
  void onIceCandidate(void Function(IceCandidate) cb);

  /// Receive connection-state changes (`new|connecting|connected|closed|failed`).
  void onStateChanged(void Function(WebRtcState) cb);

  /// Receive DataChannel messages (text or binary).
  void onDataChannelMessage(void Function(WebRtcDataChannelPayload) cb);

  /// Send a text frame.
  Future<void> sendText(String text);

  /// Send a binary frame.
  Future<void> sendBinary(Uint8List bytes);

  /// Tear down the channel and the connection.
  Future<void> close();
}

/// Production bridge. Talks to `package:flutter_webrtc` (which itself
/// dispatches to a platform channel). This class is NOT exercised by
/// `flutter test` — `FakePeerConnectionBridge` stands in.
class FlutterWebRtcBridge implements PeerConnectionBridge {
  rtc.RTCPeerConnection? _pc;
  rtc.RTCDataChannel? _dc;
  void Function(IceCandidate)? _onIce;
  void Function(WebRtcState)? _onState;
  void Function(WebRtcDataChannelPayload)? _onMessage;

  @override
  Future<void> initialize({
    required List<Map<String, dynamic>> iceServers,
    String dataChannelLabel = 'opene2ee',
  }) async {
    final pc = await rtc.createPeerConnection(<String, dynamic>{
      'iceServers': iceServers,
      'sdpSemantics': 'unified-plan',
    });
    _pc = pc;
    pc.onIceCandidate = (rtc.RTCIceCandidate c) {
      final cb = _onIce;
      if (cb == null) return;
      cb(IceCandidate(
        candidate: c.candidate ?? '',
        sdpMid: c.sdpMid,
        sdpMLineIndex: c.sdpMLineIndex,
      ));
    };
    pc.onConnectionState = _mapAndForwardState;
    pc.onDataChannel = (rtc.RTCDataChannel ch) {
      _dc = ch;
      _wireDataChannel(ch);
    };
    // Pre-create one data channel so the offerer always has one to send
    // through. The answerer receives it via `onDataChannel`.
    final init = rtc.RTCDataChannelInit()..ordered = true;
    final dc = await pc.createDataChannel(dataChannelLabel, init);
    _dc = dc;
    _wireDataChannel(dc);
  }

  void _mapAndForwardState(rtc.RTCPeerConnectionState s) {
    final cb = _onState;
    if (cb == null) return;
    switch (s) {
      case rtc.RTCPeerConnectionState.RTCPeerConnectionStateNew:
        cb(WebRtcState.idle);
      case rtc.RTCPeerConnectionState.RTCPeerConnectionStateConnecting:
        cb(WebRtcState.connecting);
      case rtc.RTCPeerConnectionState.RTCPeerConnectionStateConnected:
        cb(WebRtcState.connected);
      case rtc.RTCPeerConnectionState.RTCPeerConnectionStateClosed:
        cb(WebRtcState.closed);
      case rtc.RTCPeerConnectionState.RTCPeerConnectionStateFailed:
        cb(WebRtcState.failed);
      case rtc.RTCPeerConnectionState.RTCPeerConnectionStateDisconnected:
        cb(WebRtcState.connecting);
    }
  }

  void _wireDataChannel(rtc.RTCDataChannel ch) {
    ch.onMessage = (rtc.RTCDataChannelMessage m) {
      final cb = _onMessage;
      if (cb == null) return;
      if (m.isBinary) {
        cb(WebRtcDataChannelPayload.binary(m.binary));
      } else {
        cb(WebRtcDataChannelPayload.text(m.text));
      }
    };
  }

  @override
  Future<SdpPayload> createOffer() async {
    final pc = _pc;
    if (pc == null) {
      throw StateError('FlutterWebRtcBridge.createOffer before initialize');
    }
    final desc = await pc.createOffer();
    await pc.setLocalDescription(desc);
    return SdpPayload(sdpType: 'offer', sdp: desc.sdp ?? '');
  }

  @override
  Future<void> applyOffer(SdpPayload offer) async {
    final pc = _pc;
    if (pc == null) {
      throw StateError('FlutterWebRtcBridge.applyOffer before initialize');
    }
    await pc.setRemoteDescription(
      rtc.RTCSessionDescription(offer.sdp, offer.sdpType),
    );
  }

  @override
  Future<SdpPayload> createAnswer() async {
    final pc = _pc;
    if (pc == null) {
      throw StateError('FlutterWebRtcBridge.createAnswer before initialize');
    }
    final desc = await pc.createAnswer();
    await pc.setLocalDescription(desc);
    return SdpPayload(sdpType: 'answer', sdp: desc.sdp ?? '');
  }

  @override
  Future<void> applyAnswer(SdpPayload answer) async {
    final pc = _pc;
    if (pc == null) {
      throw StateError('FlutterWebRtcBridge.applyAnswer before initialize');
    }
    await pc.setRemoteDescription(
      rtc.RTCSessionDescription(answer.sdp, answer.sdpType),
    );
  }

  @override
  Future<void> addRemoteCandidate(IceCandidate candidate) async {
    final pc = _pc;
    if (pc == null) {
      throw StateError(
          'FlutterWebRtcBridge.addRemoteCandidate before initialize');
    }
    await pc.addCandidate(
      rtc.RTCIceCandidate(
        candidate.candidate,
        candidate.sdpMid,
        candidate.sdpMLineIndex,
      ),
    );
  }

  @override
  void onIceCandidate(void Function(IceCandidate) cb) => _onIce = cb;

  @override
  void onStateChanged(void Function(WebRtcState) cb) => _onState = cb;

  @override
  void onDataChannelMessage(void Function(WebRtcDataChannelPayload) cb) =>
      _onMessage = cb;

  @override
  Future<void> sendText(String text) async {
    final dc = _dc;
    if (dc == null) {
      throw StateError('FlutterWebRtcBridge.sendText before initialize');
    }
    await dc.send(rtc.RTCDataChannelMessage(text));
  }

  @override
  Future<void> sendBinary(Uint8List bytes) async {
    final dc = _dc;
    if (dc == null) {
      throw StateError('FlutterWebRtcBridge.sendBinary before initialize');
    }
    await dc.send(rtc.RTCDataChannelMessage.fromBinary(bytes));
  }

  @override
  Future<void> close() async {
    try {
      await _dc?.close();
    } catch (_) {
      // Closing twice (or after the native side already closed) is
      // benign — swallow.
    }
    try {
      await _pc?.close();
    } catch (_) {
      // Same.
    }
    _pc = null;
    _dc = null;
    _onIce = null;
    _onState = null;
    _onMessage = null;
  }
}

// ---------------------------------------------------------------------------
// WebRtcClient — wires the bridge to the REST client + lifecycle.
// ---------------------------------------------------------------------------

/// The high-level PR-21b entry point.
///
/// Example:
/// ```dart
/// final client = WebRtcClient(api: api, peerHash: identity.deviceIdHash);
/// await client.initialize();
/// final sessionId = await client.createOffer();   // I'm the offerer
/// // ... share `sessionId` + remote offer/answer text out-of-band ...
/// // Eventually:
/// await client.close();
/// ```
///
/// Threading: every method is async; state transitions are serialised
/// via the [WebRtcClient] instance — concurrent calls will throw
/// [WebRtcStateError] rather than racing.
class WebRtcClient {
  final ApiClient api;
  final String peerHash;
  final PeerConnectionBridge bridge;

  /// Buffer of local ICE candidates that arrived before the offer was
  /// posted. Drained into individual `POST /webrtc/ice` calls once we
  /// have a session id.
  final List<IceCandidate> _pendingLocalIce = <IceCandidate>[];

  String? _sessionId;
  WebRtcState _state = WebRtcState.idle;
  bool _initialized = false;
  bool _iceStarted = false;

  final StreamController<WebRtcState> _stateCtrl =
      StreamController<WebRtcState>.broadcast();
  final StreamController<WebRtcDataChannelPayload> _msgCtrl =
      StreamController<WebRtcDataChannelPayload>.broadcast();

  /// Build a client. [bridge] is injectable for tests — production code
  /// omits it and gets the [FlutterWebRtcBridge] default.
  WebRtcClient({
    required this.api,
    required this.peerHash,
    PeerConnectionBridge? bridge,
  }) : bridge = bridge ?? FlutterWebRtcBridge();

  /// Connection lifecycle stream. Emits every transition. Idempotent
  /// transitions (e.g. setting the same state twice) are NOT emitted.
  Stream<WebRtcState> get onStateChanged => _stateCtrl.stream;

  /// Inbound DataChannel messages (text + binary).
  Stream<WebRtcDataChannelPayload> get onDataChannelMessage => _msgCtrl.stream;

  /// Current state. Callers may read this from any isolate that owns
  /// the client (the broadcast stream is just for change notifications).
  WebRtcState get state => _state;

  /// Backend-minted session id. Null until [createOffer] (or a manual
  /// offer accept) succeeds.
  String? get sessionId => _sessionId;

  void _setState(WebRtcState next) {
    if (_state == next) return;
    _state = next;
    _stateCtrl.add(next);
  }

  /// One-shot setup: fetch ICE-servers, create the peer connection,
  /// open the data channel, wire callbacks.
  ///
  /// Must be called exactly once before any other method.
  Future<void> initialize({String dataChannelLabel = 'opene2ee'}) async {
    if (_initialized) {
      throw WebRtcStateError(
          'WebRtcClient.initialize called twice (already initialized)');
    }
    // 1. Fetch ICE-server config. A backend error here is fatal — we
    //    can't construct the PC without `iceServers`.
    final resp = await api.requestJson('GET', '/api/v1/webrtc/config');
    final ice = IceServersConfig.fromJson(resp.body);

    // 2. Initialize the bridge (PC + data channel).
    await bridge.initialize(
      iceServers: ice.toRtcIceServers(),
      dataChannelLabel: dataChannelLabel,
    );

    // 3. Wire the bridge callbacks → our streams / buffers.
    bridge.onIceCandidate(_handleLocalIceCandidate);
    bridge.onStateChanged(_setState);
    bridge.onDataChannelMessage(_msgCtrl.add);
    _initialized = true;
  }

  void _handleLocalIceCandidate(IceCandidate c) {
    _pendingLocalIce.add(c);
    // Once the offer has been posted, drain the buffer by POSTing one
    // candidate per call (the backend accepts exactly one per request).
    // Failures are best-effort: a single lost candidate doesn't kill
    // the connection, and surfacing it as an unhandled exception would
    // be worse than silent drop. The typed exception classes are
    // reserved for `createOffer` / `acceptOffer` / `applyAnswer` paths
    // where the caller asked synchronously for a state transition.
    if (_iceStarted && _sessionId != null) {
      unawaited(_postIceCandidateBestEffort(c));
    }
  }

  Future<void> _postIceCandidateBestEffort(IceCandidate c) async {
    try {
      await _postIceCandidate(c);
    } on Exception {
      // Best-effort — see _handleLocalIceCandidate for rationale.
      // Never log `c.candidate`.
    }
  }

  Future<void> _postIceCandidate(IceCandidate c) async {
    // Privacy invariant: do NOT log `c.candidate`. We only surface
    // the typed exception class.
    try {
      await api.requestJson(
        'POST',
        '/api/v1/webrtc/ice',
        body: <String, dynamic>{
          'session_id': _sessionId,
          'peer_hash': peerHash,
          'candidates': <Map<String, dynamic>>[c.toJson()],
        },
      );
    } on Exception {
      // ICE POST failures are best-effort. Losing one candidate is
      // a degraded-but-still-functional outcome — the peer may
      // discover connectivity through host candidates or the next
      // trickle. Surfacing as an unhandled exception would crash
      // the isolate for no gain. Callers that need strict delivery
      // can subscribe to `onStateChanged` and observe the failure
      // via their own retry layer.
      // The candidate string is NOT included in the error message.
      rethrow;
    }
  }

  /// I'm the offerer. Generates SDP, POSTs it, returns the backend-minted
  /// session id. Drains any local ICE candidates that arrived between
  /// `initialize` and this call.
  Future<String> createOffer() async {
    if (!_initialized) {
      throw WebRtcStateError(
          'WebRtcClient.createOffer called before initialize');
    }
    if (_state != WebRtcState.idle) {
      throw WebRtcStateError(
          'WebRtcClient.createOffer called in state $_state');
    }
    _setState(WebRtcState.offering);
    final sdp = await bridge.createOffer();
    final ApiResponse resp;
    try {
      resp = await api.requestJson(
        'POST',
        '/api/v1/webrtc/offer',
        body: <String, dynamic>{
          // Intentionally omit `session_id` — the backend mints a new
          // one for the offerer on the first write.
          'peer_hash': peerHash,
          'sdp': sdp.toJson(),
        },
      );
    } on Exception catch (e) {
      _setState(WebRtcState.failed);
      throw _mapBackendException(e);
    }
    final signal = SignalingResponse.fromJson(resp.body);
    _sessionId = signal.sessionId;
    _iceStarted = true;
    await _drainPendingIce();
    await _ingestRemoteIce(signal.remoteIce);
    return _sessionId!;
  }

  /// I'm the answerer. The remote side has minted a session id (passed
  /// out-of-band, e.g. a shared QR code); the remote offer SDP comes
  /// from the same side-channel.
  Future<void> acceptOffer({
    required String sessionId,
    required SdpPayload remoteOffer,
  }) async {
    if (!_initialized) {
      throw WebRtcStateError(
          'WebRtcClient.acceptOffer called before initialize');
    }
    if (_state != WebRtcState.idle) {
      throw WebRtcStateError(
          'WebRtcClient.acceptOffer called in state $_state');
    }
    if (sessionId.isEmpty) {
      throw WebRtcStateError('WebRtcClient.acceptOffer: sessionId required');
    }
    _setState(WebRtcState.connecting);
    _sessionId = sessionId;
    await bridge.applyOffer(remoteOffer);
    final answer = await bridge.createAnswer();
    final ApiResponse resp;
    try {
      resp = await api.requestJson(
        'POST',
        '/api/v1/webrtc/answer',
        body: <String, dynamic>{
          'session_id': sessionId,
          'peer_hash': peerHash,
          'sdp': answer.toJson(),
        },
      );
    } on Exception catch (e) {
      _setState(WebRtcState.failed);
      throw _mapBackendException(e);
    }
    final signal = SignalingResponse.fromJson(resp.body);
    _iceStarted = true;
    await _drainPendingIce();
    await _ingestRemoteIce(signal.remoteIce);
  }

  /// Apply the answerer-supplied SDP answer (called by the offerer after
  /// `createOffer` once they have the answer text in hand).
  Future<void> applyAnswer(SdpPayload remoteAnswer) async {
    if (!_initialized) {
      throw WebRtcStateError(
          'WebRtcClient.applyAnswer called before initialize');
    }
    if (_state != WebRtcState.offering) {
      throw WebRtcStateError(
          'WebRtcClient.applyAnswer called in state $_state');
    }
    await bridge.applyAnswer(remoteAnswer);
    _setState(WebRtcState.connected);
  }

  Future<void> _drainPendingIce() async {
    final pending = List<IceCandidate>.unmodifiable(_pendingLocalIce);
    _pendingLocalIce.clear();
    for (final c in pending) {
      try {
        await _postIceCandidate(c);
      } on Exception {
        // Best-effort drain — losing one candidate is degraded but
        // non-fatal. Surfacing the typed exception would crash the
        // offer/answer flow for no practical benefit.
        // Privacy: never log `c.candidate`.
      }
    }
  }

  Future<void> _ingestRemoteIce(List<IceCandidate> candidates) async {
    for (final c in candidates) {
      await bridge.addRemoteCandidate(c);
    }
  }

  /// Send a text frame over the data channel. No-op if the channel is
  /// not yet open (the bridge surfaces a `StateError` — callers can
  /// either guard with [state] or catch the StateError).
  Future<void> sendText(String text) => bridge.sendText(text);

  /// Send a binary frame.
  Future<void> sendBinary(Uint8List bytes) => bridge.sendBinary(bytes);

  /// Tear everything down. Idempotent.
  Future<void> close() async {
    if (_state.isTerminal) return;
    await bridge.close();
    _setState(WebRtcState.closed);
    // Streams stay open for late listeners but stop emitting new
    // events (the bridge callbacks are cleared in [FlutterWebRtcBridge.close]).
  }

  /// Internal: dispose of the broadcast streams. Tests can call this to
  /// release the underlying isolates; production code generally lets
  /// GC do it.
  Future<void> dispose() async {
    await close();
    if (!_stateCtrl.isClosed) await _stateCtrl.close();
    if (!_msgCtrl.isClosed) await _msgCtrl.close();
  }

  /// Translate a thrown [Exception] from the REST layer or bridge into a
  /// typed [WebRtcException]. The translation ONLY uses the exception
  /// class — the candidate/SDP body is never inspected.
  ///
  /// Visible for tests.
  static WebRtcException _mapBackendException(Object e) {
    if (e is WebRtcException) return e;
    if (e is UnauthorizedException) {
      return WebRtcBackendError(
        401,
        'Unauthorized: ${e.reason ?? "no reason given"}',
        code: 'unauthorized',
      );
    }
    if (e is RateLimitedException) {
      return WebRtcRateLimited(
        'Rate limited (retry after ${e.retryAfter})',
        retryAfter: e.retryAfter,
      );
    }
    // `DioException` for non-401/429 responses falls through the
    // error interceptor unchanged — translate here so callers can
    // catch the specific subclass (BadRequest / NotFound / Conflict
    // / InternalError) instead of fishing through Dio's status codes.
    if (e is DioException) {
      final status = e.response?.statusCode ?? 0;
      // Try to extract the backend's {code, message} envelope; if
      // the body isn't shaped that way, fall back to a sanitised
      // scrubLog of the raw text.
      String message = 'REST failure';
      String code = 'http_${status == 0 ? "network" : status.toString()}';
      final data = e.response?.data;
      if (data is Map) {
        final m = data.cast<String, dynamic>();
        if (m['message'] is String) message = m['message'] as String;
        if (m['code'] is String) code = m['code'] as String;
      } else if (data is String && data.isNotEmpty) {
        message = scrubLog(data);
      }
      switch (status) {
        case 400:
          return WebRtcBadRequest(message);
        case 404:
          return WebRtcNotFound(message);
        case 409:
          return WebRtcConflict(message, code: code);
        case 429:
          // Already wrapped by the ApiClient interceptor — kept
          // here as a safety net for the rare case it isn't.
          return WebRtcRateLimited(message);
        default:
          if (status >= 500) {
            return WebRtcInternalError(message);
          }
          return WebRtcException(message, code: code);
      }
    }
    return WebRtcException(
      'WebRtcClient: ${e.runtimeType}: ${scrubLog(e.toString())}',
    );
  }

  /// Scrub SDP/ICE-like substrings from a string before it goes into
  /// any error message / log line. This is the LAST line of defence —
  /// callers should never have to look here.
  ///
  /// Public so dev UIs / on-screen log widgets can use it too.
  /// Replaces:
  ///   * `candidate:<...>`  → `candidate:<redacted>`  (ICE candidate
  ///     string, end-of-line / length-bounded — covers the full
  ///     `foundation component proto priority addr port typ ...`
  ///     attribute, not just the foundation)
  ///   * `v=0<...>`         → `<sdp-redacted>`        (SDP first line)
  ///
  /// The regexes are intentionally conservative — we want to avoid
  /// accidentally swallowing legitimate text. We bound the candidate
  /// match by `<=400` non-newline chars so a malformed blob won't drag
  /// the whole log message into one redaction.
  static String scrubLog(String input) {
    return input
        .replaceAll(
          RegExp(r'candidate:[^\n\r]{1,400}'),
          'candidate:<redacted>',
        )
        .replaceAll(RegExp(r'v=0[^\n\r]*'), '<sdp-redacted>');
  }
}
