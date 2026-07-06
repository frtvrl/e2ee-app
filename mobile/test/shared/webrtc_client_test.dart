// mobile/test/shared/webrtc_client_test.dart
//
// PR-21b / Sprint 3 — `WebRtcClient` + bridge contract tests.
//
// Test goals (per docs/SPRINT-3-SCOPE.md §6):
//
//   1. DTO roundtrips against `matching.{SDPPayload, ICECandidate,
//      WebRTCSignallingResponse, STUNTURNConfig}`.
//   2. State-machine guards mirror the Go-side
//      `matching.SessionState` (idle → offering/connecting →
//      connected → closed/failed).
//   3. SDP/ICE exchange loop: GET /webrtc/config +
//      POST /webrtc/{offer,answer,ice} all line up.
//   4. Local ICE trickle: candidates fired BEFORE and AFTER the
//      offer are both POSTed once we have a session id.
//   5. Privacy (ADR-0006): SDP body and candidate strings NEVER
//      surface in error messages / log lines.

import 'dart:async';
import 'dart:convert';
import 'dart:typed_data';

import 'package:dio/dio.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:opene2ee/shared/api_client.dart';
import 'package:opene2ee/shared/webrtc_client.dart';

// ---------------------------------------------------------------------------
// Test doubles
// ---------------------------------------------------------------------------

/// Per-path canned HTTP adapter. Same shape as the one PR-9 / PR-21a
/// use; inlined here so this PR depends on nothing outside `shared/`.
class _StubAdapter implements HttpClientAdapter {
  final Map<String, _StubSpec> specs = <String, _StubSpec>{};
  final List<RequestOptions> seen = <RequestOptions>[];

  @override
  void close({bool force = false}) {}

  @override
  Future<ResponseBody> fetch(
    RequestOptions options,
    Stream<Uint8List>? requestStream,
    Future<dynamic>? cancelFuture,
  ) async {
    seen.add(options);
    final spec = specs[options.path];
    if (spec == null) {
      return ResponseBody.fromBytes(
        utf8.encode(jsonEncode(<String, dynamic>{
          'code': 'not_stubbed',
          'message': 'no stub for ${options.path}',
        })),
        500,
        headers: <String, List<String>>{
          'Content-Type': <String>['application/json'],
          'X-API-Version': <String>['1'],
        },
      );
    }
    return ResponseBody.fromBytes(
      utf8.encode(jsonEncode(spec.body)),
      spec.status,
      headers: <String, List<String>>{
        'Content-Type': <String>['application/json'],
        'X-API-Version': <String>['1'],
      },
    );
  }
}

class _StubSpec {
  final int status;
  final Map<String, dynamic> body;
  const _StubSpec(this.status, this.body);
}

/// In-memory fake of `PeerConnectionBridge` — records every native
/// call so tests assert on the bridge contract end-to-end without a
/// platform channel.
class FakePeerConnectionBridge implements PeerConnectionBridge {
  final List<String> calls = <String>[];
  bool initialized = false;
  bool closed = false;

  // Outbound (client → bridge).
  final List<SdpPayload> createdOffers = <SdpPayload>[];
  final List<SdpPayload> createdAnswers = <SdpPayload>[];
  final List<SdpPayload> appliedOffers = <SdpPayload>[];
  final List<SdpPayload> appliedAnswers = <SdpPayload>[];
  final List<IceCandidate> addedRemoteCandidates = <IceCandidate>[];
  final List<String> sentTexts = <String>[];

  // Inbound (bridge → client) callbacks.
  void Function(IceCandidate)? onIceCandidateCb;
  void Function(WebRtcState)? onStateChangedCb;
  void Function(WebRtcDataChannelPayload)? onDataChannelMessageCb;

  @override
  Future<void> initialize({
    required List<Map<String, dynamic>> iceServers,
    String dataChannelLabel = 'opene2ee',
  }) async {
    calls.add('initialize');
    initialized = true;
  }

  @override
  Future<SdpPayload> createOffer() async {
    calls.add('createOffer');
    const sdp = SdpPayload(
      sdpType: 'offer',
      sdp: 'v=0\r\no=- 0 0 IN IP4 127.0.0.1\r\ns=-\r\nt=0 0\r\n'
          'm=application 9 UDP/DTLS/SCTP webrtc-datachannel',
    );
    createdOffers.add(sdp);
    return sdp;
  }

  @override
  Future<void> applyOffer(SdpPayload offer) async {
    calls.add('applyOffer');
    appliedOffers.add(offer);
  }

  @override
  Future<SdpPayload> createAnswer() async {
    calls.add('createAnswer');
    const sdp = SdpPayload(
      sdpType: 'answer',
      sdp: 'v=0\r\no=- 1 1 IN IP4 127.0.0.1\r\ns=-\r\nt=0 0\r\n'
          'm=application 9 UDP/DTLS/SCTP webrtc-datachannel',
    );
    createdAnswers.add(sdp);
    return sdp;
  }

  @override
  Future<void> applyAnswer(SdpPayload answer) async {
    calls.add('applyAnswer');
    appliedAnswers.add(answer);
  }

  @override
  Future<void> addRemoteCandidate(IceCandidate candidate) async {
    calls.add('addRemoteCandidate');
    addedRemoteCandidates.add(candidate);
  }

  @override
  void onIceCandidate(void Function(IceCandidate) cb) {
    onIceCandidateCb = cb;
  }

  @override
  void onStateChanged(void Function(WebRtcState) cb) {
    onStateChangedCb = cb;
  }

  @override
  void onDataChannelMessage(void Function(WebRtcDataChannelPayload) cb) {
    onDataChannelMessageCb = cb;
  }

  @override
  Future<void> sendText(String text) async {
    calls.add('sendText');
    sentTexts.add(text);
  }

  @override
  Future<void> sendBinary(Uint8List bytes) async {
    calls.add('sendBinary');
  }

  @override
  Future<void> close() async {
    calls.add('close');
    closed = true;
  }

  // Test helpers — fire inbound events as the native side would.
  void fireIceCandidate(IceCandidate c) => onIceCandidateCb?.call(c);
  void fireStateChanged(WebRtcState s) => onStateChangedCb?.call(s);
  void fireDataChannelMessage(WebRtcDataChannelPayload p) =>
      onDataChannelMessageCb?.call(p);
}

const String _kPeerHash =
    'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa'; // 32 hex chars (16..64)

ApiClient _buildClient(_StubAdapter adapter) {
  final dio = Dio(BaseOptions())..httpClientAdapter = adapter;
  return ApiClient(
    config: const ApiConfig(baseUrl: 'https://api.test'),
    dio: dio,
  );
}

_StubAdapter _configOk() => _StubAdapter()
  ..specs['/api/v1/webrtc/config'] = const _StubSpec(
    200,
    <String, dynamic>{
      'stun_urls': <String>['stun:stun.l.google.com:19302'],
      'ttl_seconds': 3600,
    },
  );

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

void main() {
  // -------------------------------------------------------------------------
  // DTO roundtrips
  // -------------------------------------------------------------------------

  group('WebRtcState wire mapping', () {
    test('wireName matches backend enum', () {
      expect(WebRtcState.idle.wireName, 'new');
      expect(WebRtcState.offering.wireName, 'connecting');
      expect(WebRtcState.connected.wireName, 'connected');
      expect(WebRtcState.closed.wireName, 'closed');
      expect(WebRtcState.failed.wireName, 'failed');
    });

    test('fromWire: unknown defaults to failed', () {
      expect(WebRtcState.fromWire('garbage'), WebRtcState.failed);
    });

    test('isTerminal only for closed/failed', () {
      expect(WebRtcState.connected.isTerminal, isFalse);
      expect(WebRtcState.closed.isTerminal, isTrue);
      expect(WebRtcState.failed.isTerminal, isTrue);
    });
  });

  group('IceServersConfig', () {
    test('parses STUN + optional TURN', () {
      final cfg = IceServersConfig.fromJson(<String, dynamic>{
        'stun_urls': <String>['stun:stun.l.google.com:19302'],
        'turn_url': 'turn:turn.example:3478',
        'turn_username': 'u',
        'turn_credential': 'c',
        'ttl_seconds': 3600,
      });
      expect(cfg.stunUrls, hasLength(1));
      expect(cfg.turnUrl, 'turn:turn.example:3478');
      expect(cfg.toRtcIceServers(), hasLength(2));
    });

    test('tolerates empty STUN-only config', () {
      final cfg = IceServersConfig.fromJson(<String, dynamic>{
        'stun_urls': <String>['stun:stun.l.google.com:19302'],
      });
      expect(cfg.toRtcIceServers(), hasLength(1));
    });
  });

  group('IceCandidate / SdpPayload JSON', () {
    test('IceCandidate roundtrip preserves candidate', () {
      const c = IceCandidate(
        candidate: 'candidate:1 1 udp 2122260223 10.0.0.1 12345 typ host',
      );
      final restored = IceCandidate.fromJson(c.toJson());
      expect(restored.candidate, c.candidate);
    });

    test('SdpPayload roundtrip preserves sdp_type + sdp', () {
      const s = SdpPayload(sdpType: 'offer', sdp: 'v=0\r\n');
      final restored = SdpPayload.fromJson(s.toJson());
      expect(restored.sdpType, 'offer');
      expect(restored.sdp, 'v=0\r\n');
    });

    test('SignalingResponse.fromJson tolerates missing remote_ice', () {
      final r = SignalingResponse.fromJson(<String, dynamic>{
        'session_id': 's',
        'state': 'new',
      });
      expect(r.remoteIce, isEmpty);
      expect(r.peersWithIce, isEmpty);
    });
  });

  // -------------------------------------------------------------------------
  // scrubLog — ADR-0006 privacy invariant
  // -------------------------------------------------------------------------

  group('scrubLog (ADR-0006)', () {
    test('redacts candidate attribute entirely', () {
      final out = WebRtcClient.scrubLog(
        'failed at candidate:1 1 udp 2122260223 10.0.0.1 12345 typ host',
      );
      expect(out, contains('candidate:<redacted>'));
      expect(out, isNot(contains('10.0.0.1')));
      expect(out, isNot(contains('12345')));
    });

    test('redacts SDP v=0 first-line marker', () {
      final out = WebRtcClient.scrubLog('sdp body: v=0\r\no=- 0 0\r\n');
      expect(out, contains('<sdp-redacted>'));
    });

    test('leaves non-secret text untouched', () {
      const sample = 'failed: state_transition: new -> connecting not allowed';
      expect(WebRtcClient.scrubLog(sample), sample);
    });
  });

  // -------------------------------------------------------------------------
  // State-machine guards
  // -------------------------------------------------------------------------

  group('State machine guards', () {
    test('createOffer before initialize throws WebRtcStateError', () async {
      final sut = WebRtcClient(
        api: _buildClient(_StubAdapter()),
        peerHash: _kPeerHash,
        bridge: FakePeerConnectionBridge(),
      );
      await expectLater(
        () => sut.createOffer(),
        throwsA(isA<WebRtcStateError>()),
      );
    });

    test('initialize twice throws WebRtcStateError', () async {
      final adapter = _configOk();
      final sut = WebRtcClient(
        api: _buildClient(adapter),
        peerHash: _kPeerHash,
        bridge: FakePeerConnectionBridge(),
      );
      await sut.initialize();
      await expectLater(
        () => sut.initialize(),
        throwsA(isA<WebRtcStateError>()),
      );
    });
  });

  // -------------------------------------------------------------------------
  // initialize() → GET /webrtc/config + bridge.initialize(iceServers)
  // -------------------------------------------------------------------------

  group('initialize()', () {
    test('calls GET /api/v1/webrtc/config + bridge.initialize()', () async {
      final adapter = _StubAdapter()
        ..specs['/api/v1/webrtc/config'] = const _StubSpec(
          200,
          <String, dynamic>{
            'stun_urls': <String>[
              'stun:stun.l.google.com:19302',
              'stun:stun1.l.google.com:19302',
            ],
            'ttl_seconds': 86400,
          },
        );
      final fake = FakePeerConnectionBridge();
      final sut = WebRtcClient(
        api: _buildClient(adapter),
        peerHash: _kPeerHash,
        bridge: fake,
      );

      await sut.initialize();

      expect(adapter.seen, hasLength(1));
      expect(adapter.seen.single.path, '/api/v1/webrtc/config');
      expect(fake.initialized, isTrue);
    });
  });

  // -------------------------------------------------------------------------
  // createOffer() — offerer happy path
  // -------------------------------------------------------------------------

  group('createOffer()', () {
    test('POSTs /webrtc/offer, drains pending ICE, returns session_id',
        () async {
      final adapter = _StubAdapter()
        ..specs['/api/v1/webrtc/config'] = const _StubSpec(
          200,
          <String, dynamic>{
            'stun_urls': <String>['stun:stun.l.google.com:19302'],
          },
        )
        ..specs['/api/v1/webrtc/offer'] = const _StubSpec(
          201,
          <String, dynamic>{
            'session_id': 'sess-1',
            'state': 'connecting',
            'remote_ice': <Map<String, dynamic>>[
              <String, dynamic>{
                'candidate':
                    'candidate:2 1 udp 2122260223 10.0.0.2 54321 typ host',
                'sdpMid': '0',
                'sdpMLineIndex': 0,
              },
            ],
            'peers_with_ice': <String>['other'],
          },
        )
        ..specs['/api/v1/webrtc/ice'] = const _StubSpec(
          200,
          <String, dynamic>{'session_id': 'sess-1', 'state': 'connecting'},
        );
      final fake = FakePeerConnectionBridge();
      final sut = WebRtcClient(
        api: _buildClient(adapter),
        peerHash: _kPeerHash,
        bridge: fake,
      );

      await sut.initialize();
      // Local ICE arrives BEFORE offer — must drain after POST.
      fake.fireIceCandidate(const IceCandidate(
        candidate: 'candidate:1 1 udp 2122260223 10.0.0.1 1 typ host',
        sdpMid: '0',
        sdpMLineIndex: 0,
      ));
      final sid = await sut.createOffer();

      expect(sid, 'sess-1');
      expect(sut.state, WebRtcState.offering);
      expect(fake.calls,
          containsAllInOrder(<String>['initialize', 'createOffer']));
      final iceReqs = adapter.seen
          .where((r) => r.path == '/api/v1/webrtc/ice')
          .toList();
      expect(iceReqs, hasLength(1));
      expect(fake.addedRemoteCandidates, hasLength(1));
    });

    test('state transitions idle → offering exactly once', () async {
      final adapter = _StubAdapter()
        ..specs['/api/v1/webrtc/config'] = const _StubSpec(
          200,
          <String, dynamic>{
            'stun_urls': <String>['stun:stun.l.google.com:19302'],
          },
        )
        ..specs['/api/v1/webrtc/offer'] = const _StubSpec(
          201,
          <String, dynamic>{'session_id': 'sess-1', 'state': 'connecting'},
        );
      final fake = FakePeerConnectionBridge();
      final sut = WebRtcClient(
        api: _buildClient(adapter),
        peerHash: _kPeerHash,
        bridge: fake,
      );
      await sut.initialize();
      final states = <WebRtcState>[];
      final sub = sut.onStateChanged.listen(states.add);
      await sut.createOffer();
      await Future<void>.delayed(const Duration(milliseconds: 10));
      await sub.cancel();
      expect(states, equals(<WebRtcState>[WebRtcState.offering]));
    });
  });

  // -------------------------------------------------------------------------
  // acceptOffer() — answerer happy path
  // -------------------------------------------------------------------------

  group('acceptOffer()', () {
    test('applyOffer + createAnswer + POST /webrtc/answer', () async {
      final adapter = _StubAdapter()
        ..specs['/api/v1/webrtc/config'] = const _StubSpec(
          200,
          <String, dynamic>{
            'stun_urls': <String>['stun:stun.l.google.com:19302'],
          },
        )
        ..specs['/api/v1/webrtc/answer'] = const _StubSpec(
          200,
          <String, dynamic>{'session_id': 'sess-1', 'state': 'connecting'},
        );
      final fake = FakePeerConnectionBridge();
      final sut = WebRtcClient(
        api: _buildClient(adapter),
        peerHash: _kPeerHash,
        bridge: fake,
      );
      await sut.initialize();
      await sut.acceptOffer(
        sessionId: 'sess-1',
        remoteOffer: const SdpPayload(sdpType: 'offer', sdp: 'v=0\r\n'),
      );
      expect(fake.calls,
          containsAllInOrder(<String>['applyOffer', 'createAnswer']));
      expect(sut.state, WebRtcState.connecting);
    });

    test('empty session id rejected (state_error, no REST)', () async {
      final fake = FakePeerConnectionBridge();
      final sut = WebRtcClient(
        api: _buildClient(_configOk()),
        peerHash: _kPeerHash,
        bridge: fake,
      );
      await sut.initialize();
      await expectLater(
        () => sut.acceptOffer(
          sessionId: '',
          remoteOffer: const SdpPayload(sdpType: 'offer', sdp: 'v=0\r\n'),
        ),
        throwsA(isA<WebRtcStateError>()),
      );
    });
  });

  // -------------------------------------------------------------------------
  // applyAnswer() — offerer side after offer is exchanged
  // -------------------------------------------------------------------------

  group('applyAnswer()', () {
    test('transitions offering → connected', () async {
      final adapter = _StubAdapter()
        ..specs['/api/v1/webrtc/config'] = const _StubSpec(
          200,
          <String, dynamic>{
            'stun_urls': <String>['stun:stun.l.google.com:19302'],
          },
        )
        ..specs['/api/v1/webrtc/offer'] = const _StubSpec(
          201,
          <String, dynamic>{'session_id': 'sess-1', 'state': 'connecting'},
        );
      final fake = FakePeerConnectionBridge();
      final sut = WebRtcClient(
        api: _buildClient(adapter),
        peerHash: _kPeerHash,
        bridge: fake,
      );
      await sut.initialize();
      await sut.createOffer();
      await sut.applyAnswer(
        const SdpPayload(sdpType: 'answer', sdp: 'v=0\r\n'),
      );
      expect(sut.state, WebRtcState.connected);
      expect(fake.calls, contains('applyAnswer'));
    });
  });

  // -------------------------------------------------------------------------
  // Local ICE trickle — POSTed immediately once session is open
  // -------------------------------------------------------------------------

  group('Local ICE trickle', () {
    test('candidate fired AFTER offer is POSTed immediately', () async {
      final adapter = _StubAdapter()
        ..specs['/api/v1/webrtc/config'] = const _StubSpec(
          200,
          <String, dynamic>{
            'stun_urls': <String>['stun:stun.l.google.com:19302'],
          },
        )
        ..specs['/api/v1/webrtc/offer'] = const _StubSpec(
          201,
          <String, dynamic>{'session_id': 'sess-1', 'state': 'connecting'},
        )
        ..specs['/api/v1/webrtc/ice'] = const _StubSpec(
          200,
          <String, dynamic>{'session_id': 'sess-1', 'state': 'connecting'},
        );
      final fake = FakePeerConnectionBridge();
      final sut = WebRtcClient(
        api: _buildClient(adapter),
        peerHash: _kPeerHash,
        bridge: fake,
      );
      await sut.initialize();
      await sut.createOffer();
      final iceBefore = adapter.seen
          .where((r) => r.path == '/api/v1/webrtc/ice')
          .length;
      expect(iceBefore, 0);
      fake.fireIceCandidate(const IceCandidate(
        candidate: 'candidate:1 1 udp 2122260223 10.0.0.1 1 typ host',
        sdpMid: '0',
        sdpMLineIndex: 0,
      ));
      await Future<void>.delayed(const Duration(milliseconds: 20));
      final iceAfter = adapter.seen
          .where((r) => r.path == '/api/v1/webrtc/ice')
          .length;
      expect(iceAfter, 1);
    });
  });

  // -------------------------------------------------------------------------
  // close() — terminal
  // -------------------------------------------------------------------------

  group('close()', () {
    test('transitions to closed, idempotent', () async {
      final fake = FakePeerConnectionBridge();
      final sut = WebRtcClient(
        api: _buildClient(_configOk()),
        peerHash: _kPeerHash,
        bridge: fake,
      );
      await sut.initialize();
      await sut.close();
      await sut.close(); // no throw
      expect(sut.state, WebRtcState.closed);
      expect(fake.closed, isTrue);
    });
  });

  // -------------------------------------------------------------------------
  // DataChannel pass-through
  // -------------------------------------------------------------------------

  group('DataChannel', () {
    test('sendText bridges to native side', () async {
      final fake = FakePeerConnectionBridge();
      final sut = WebRtcClient(
        api: _buildClient(_configOk()),
        peerHash: _kPeerHash,
        bridge: fake,
      );
      await sut.initialize();
      await sut.sendText('hello');
      expect(fake.sentTexts.single, 'hello');
    });

    test('inbound DataChannel messages surface via onDataChannelMessage',
        () async {
      final fake = FakePeerConnectionBridge();
      final sut = WebRtcClient(
        api: _buildClient(_configOk()),
        peerHash: _kPeerHash,
        bridge: fake,
      );
      await sut.initialize();
      final received = <WebRtcDataChannelPayload>[];
      final sub = sut.onDataChannelMessage.listen(received.add);
      fake.fireDataChannelMessage(
        const WebRtcDataChannelPayload.text('hi'),
      );
      await Future<void>.delayed(const Duration(milliseconds: 10));
      await sub.cancel();
      expect(received.single.text, 'hi');
    });
  });

  // -------------------------------------------------------------------------
  // Privacy (ADR-0006) — candidate & SDP secrecy
  // -------------------------------------------------------------------------

  group('Privacy (ADR-0006)', () {
    test('REST error from /offer does NOT echo SDP', () async {
      final adapter = _StubAdapter()
        ..specs['/api/v1/webrtc/config'] = const _StubSpec(
          200,
          <String, dynamic>{
            'stun_urls': <String>['stun:stun.l.google.com:19302'],
          },
        )
        ..specs['/api/v1/webrtc/offer'] = const _StubSpec(
          409,
          <String, dynamic>{
            'code': 'state_transition',
            'message': 'cannot apply offer in state connected',
          },
        );
      final fake = FakePeerConnectionBridge();
      final sut = WebRtcClient(
        api: _buildClient(adapter),
        peerHash: _kPeerHash,
        bridge: fake,
      );
      await sut.initialize();
      WebRtcException? caught;
      try {
        await sut.createOffer();
      } on WebRtcException catch (e) {
        caught = e;
      }
      expect(caught, isNotNull);
      // Fake bridge offers SDP containing "127.0.0.1"; the typed
      // error surface must NOT echo it.
      final asText = caught.toString();
      expect(asText, isNot(contains('127.0.0.1')));
      expect(asText, isNot(contains('v=0')));
    });
  });

  // -------------------------------------------------------------------------
  // Exception translation — DioException → typed WebRtcException paths.
  // Covers the `_mapBackendException` switch (404/500 cases) and the
  // production-bridge StateError guards (the only Dart-only paths in
  // `FlutterWebRtcBridge`, which cannot be exercised end-to-end without
  // a real WebRTC platform channel).
  // -------------------------------------------------------------------------

  group('Exception translation + bridge guards', () {
    test('createOffer + 404 /webrtc/offer → WebRtcNotFound', () async {
      final adapter = _StubAdapter()
        ..specs['/api/v1/webrtc/config'] = const _StubSpec(
          200,
          <String, dynamic>{
            'stun_urls': <String>['stun:stun.l.google.com:19302'],
          },
        )
        ..specs['/api/v1/webrtc/offer'] = const _StubSpec(
          404,
          <String, dynamic>{
            'code': 'session_not_found',
            'message': 'session not present',
          },
        );
      final sut = WebRtcClient(
        api: _buildClient(adapter),
        peerHash: _kPeerHash,
        bridge: FakePeerConnectionBridge(),
      );
      await sut.initialize();
      await expectLater(
        () => sut.createOffer(),
        throwsA(isA<WebRtcNotFound>()),
      );
    });

    test('createOffer + 500 /webrtc/offer → WebRtcInternalError', () async {
      final adapter = _StubAdapter()
        ..specs['/api/v1/webrtc/config'] = const _StubSpec(
          200,
          <String, dynamic>{
            'stun_urls': <String>['stun:stun.l.google.com:19302'],
          },
        )
        ..specs['/api/v1/webrtc/offer'] = const _StubSpec(
          500,
          <String, dynamic>{
            'code': 'internal_error',
            'message': 'database down',
          },
        );
      final sut = WebRtcClient(
        api: _buildClient(adapter),
        peerHash: _kPeerHash,
        bridge: FakePeerConnectionBridge(),
      );
      await sut.initialize();
      await expectLater(
        () => sut.createOffer(),
        throwsA(isA<WebRtcInternalError>()),
      );
      expect(sut.state, WebRtcState.failed);
    });

    test('FlutterWebRtcBridge.createOffer before initialize throws StateError',
        () async {
      // Production bridge: the only Dart-reachable path without a
      // platform channel is the `if (_pc == null) throw StateError(...)`
      // guards. Each of the 7 covered methods asserts that guard.
      final bridge = FlutterWebRtcBridge();
      await expectLater(
        () => bridge.createOffer(),
        throwsA(isA<StateError>()),
      );
      await expectLater(
        () => bridge.createAnswer(),
        throwsA(isA<StateError>()),
      );
      await expectLater(
        () => bridge.applyOffer(
          const SdpPayload(sdpType: 'offer', sdp: 'v=0\r\n'),
        ),
        throwsA(isA<StateError>()),
      );
      await expectLater(
        () => bridge.applyAnswer(
          const SdpPayload(sdpType: 'answer', sdp: 'v=0\r\n'),
        ),
        throwsA(isA<StateError>()),
      );
      await expectLater(
        () => bridge.addRemoteCandidate(
          const IceCandidate(candidate: 'candidate:1 1 udp 1 127.0.0.1 1 typ host'),
        ),
        throwsA(isA<StateError>()),
      );
      await expectLater(
        () => bridge.sendText('x'),
        throwsA(isA<StateError>()),
      );
      await expectLater(
        () => bridge.sendBinary(Uint8List.fromList(<int>[1, 2])),
        throwsA(isA<StateError>()),
      );
    });

    test('WebRtcClient.dispose after close tears down broadcast streams',
        () async {
      final sut = WebRtcClient(
        api: _buildClient(_configOk()),
        peerHash: _kPeerHash,
        bridge: FakePeerConnectionBridge(),
      );
      await sut.initialize();
      await sut.dispose();
      expect(sut.state, WebRtcState.closed);
    });
  });
}
