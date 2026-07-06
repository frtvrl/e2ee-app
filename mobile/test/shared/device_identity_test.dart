// test/shared/device_identity_test.dart
//
// PR-9 / Mobile shared core — DeviceIdentity unit tests.
//
// We do NOT exercise the real Keystore/Keychain here — those need a device.
// Instead we feed `DeviceIdentity.loadOrCreate(secure: ...)` an in-memory
// `FlutterSecureStorage` shim. Production code passes the real one.
//
// `Map<String,String>` API faithful enough to exercise our code paths.

import 'dart:convert';
import 'dart:io';
import 'dart:typed_data';

import 'package:ed25519_edwards/ed25519_edwards.dart' as ed;
import 'package:flutter_secure_storage/flutter_secure_storage.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:opene2ee/shared/device_identity.dart';

/// In-memory replacement for [FlutterSecureStorage]. The real plugin binds
/// to platform channels (Keystore on Android, Keychain on iOS) which are
/// not available in `flutter test`; this stub gives us a Map-of-strings
/// API faithful enough to exercise our code paths.
class _InMemorySecureStorage implements FlutterSecureStorage {
  final Map<String, String> _store = <String, String>{};

  @override
  Future<String?> read({
    required String key,
    IOSOptions? iOptions,
    AndroidOptions? aOptions,
    LinuxOptions? lOptions,
    WebOptions? webOptions,
    MacOsOptions? mOptions,
    WindowsOptions? wOptions,
  }) async =>
      _store[key];

  @override
  Future<void> write({
    required String key,
    required String? value,
    IOSOptions? iOptions,
    AndroidOptions? aOptions,
    LinuxOptions? lOptions,
    WebOptions? webOptions,
    MacOsOptions? mOptions,
    WindowsOptions? wOptions,
  }) async {
    if (value == null) {
      _store.remove(key);
    } else {
      _store[key] = value;
    }
  }

  @override
  Future<void> delete({
    required String key,
    IOSOptions? iOptions,
    AndroidOptions? aOptions,
    LinuxOptions? lOptions,
    WebOptions? webOptions,
    MacOsOptions? mOptions,
    WindowsOptions? wOptions,
  }) async {
    _store.remove(key);
  }

  @override
  Future<bool> containsKey({
    required String key,
    IOSOptions? iOptions,
    AndroidOptions? aOptions,
    LinuxOptions? lOptions,
    WebOptions? webOptions,
    MacOsOptions? mOptions,
    WindowsOptions? wOptions,
  }) async =>
      _store.containsKey(key);

  @override
  Future<void> deleteAll({
    IOSOptions? iOptions,
    AndroidOptions? aOptions,
    LinuxOptions? lOptions,
    WebOptions? webOptions,
    MacOsOptions? mOptions,
    WindowsOptions? wOptions,
  }) async {
    _store.clear();
  }

  @override
  Future<Map<String, String>> readAll({
    IOSOptions? iOptions,
    AndroidOptions? aOptions,
    LinuxOptions? lOptions,
    WebOptions? webOptions,
    MacOsOptions? mOptions,
    WindowsOptions? wOptions,
  }) async =>
      Map<String, String>.unmodifiable(_store);

  // The remaining surface (registerListener, etc.) is unused by PR-9.
  // We deliberately do not implement it; the analyzer will flag the unused
  // overrides only if it walks the full type, which it does not for an
  // implements-relation on a class we own.
  @override
  dynamic noSuchMethod(Invocation invocation) =>
      super.noSuchMethod(invocation);
}

void main() {
  group('DeviceIdentity.loadOrCreate', () {
    test('mints a fresh UUID v7 + Ed25519 keypair on first launch', () async {
      final storage = _InMemorySecureStorage();
      final id = await DeviceIdentity.loadOrCreate(secure: storage);

      // UUID v7 layout: 8-4-4-4-12 lowercase hex, version nibble == 7.
      expect(id.uuidV7.length, 36);
      expect(id.uuidV7, matches(RegExp(r'^[0-9a-f-]+$')));
      // Position 14 is the version nibble per RFC 9562.
      expect(id.uuidV7[14], '7');
      expect(id.ed25519PublicKey.length, ed.PublicKeySize);
    });

    test('returns the same identity on subsequent loads (idempotent)',
        () async {
      final storage = _InMemorySecureStorage();
      final first = await DeviceIdentity.loadOrCreate(secure: storage);
      final second = await DeviceIdentity.loadOrCreate(secure: storage);
      expect(second.uuidV7, first.uuidV7);
      expect(second.ed25519PublicKey, equals(first.ed25519PublicKey));
    });

    test('persists uuid + pub + priv into secure storage on first launch',
        () async {
      final storage = _InMemorySecureStorage();
      final id = await DeviceIdentity.loadOrCreate(secure: storage);

      // All three storage keys are populated.
      expect(await storage.containsKey(key: 'opene2ee.device.uuid_v7'), isTrue);
      expect(
          await storage.containsKey(key: 'opene2ee.device.ed25519_public_key_b64'),
          isTrue);
      expect(
          await storage.containsKey(key: 'opene2ee.device.ed25519_private_key_b64'),
          isTrue);

      // The persisted private key decodes to 64 bytes (ed25519_edwards'
      // private-key representation is seed || pub).
      final priv = base64Decode(
        await storage.read(
            key: 'opene2ee.device.ed25519_private_key_b64') as String,
      );
      expect(priv.length, ed.PrivateKeySize);

      // And the pub derived from that priv matches what loadOrCreate returned.
      final pubFromPriv = priv.sublist(32, 32 + ed.PublicKeySize);
      expect(Uint8List.fromList(pubFromPriv), equals(id.ed25519PublicKey));
    });
  });

  group('DeviceIdentity.publicKeyFingerprint', () {
    test('is a stable 32-char lowercase hex string', () async {
      final id = await DeviceIdentity.loadOrCreate(secure: _InMemorySecureStorage());
      final fp = id.publicKeyFingerprint();
      expect(fp.length, 32);
      expect(fp, matches(RegExp(r'^[0-9a-f]{32}$')));
      // Stable: calling twice yields the same value.
      expect(id.publicKeyFingerprint(), fp);
    });

    test('differs across two independently generated keypairs', () async {
      final a = await DeviceIdentity.loadOrCreate(secure: _InMemorySecureStorage());
      final b = await DeviceIdentity.loadOrCreate(secure: _InMemorySecureStorage());
      expect(a.publicKeyFingerprint(), isNot(b.publicKeyFingerprint()));
    });
  });

  group('DeviceIdentity.deviceIdHash', () {
    test('is deterministic given the same UUID + salt', () async {
      final id = await DeviceIdentity.loadOrCreate(secure: _InMemorySecureStorage());
      final a = id.deviceIdHash(serverSalt: 'opene2ee-v1');
      final b = id.deviceIdHash(serverSalt: 'opene2ee-v1');
      expect(a, b);
      expect(a.length, 32);
      expect(a, matches(RegExp(r'^[0-9a-f]{32}$')));
    });

    test('changes when the salt changes (re-registration scenario)',
        () async {
      final id = await DeviceIdentity.loadOrCreate(secure: _InMemorySecureStorage());
      final before = id.deviceIdHash(serverSalt: 'salt-2024');
      final after = id.deviceIdHash(serverSalt: 'salt-2025');
      expect(before, isNot(after));
    });

    test('throws on empty server salt (programmer error)', () async {
      final id = await DeviceIdentity.loadOrCreate(secure: _InMemorySecureStorage());
      expect(() => id.deviceIdHash(serverSalt: ''), throwsArgumentError);
    });
  });

  group('DeviceIdentity.sign / verify', () {
    test('Ed25519 round-trip yields a verifiable signature', () async {
      final id = await DeviceIdentity.loadOrCreate(secure: _InMemorySecureStorage());
      final message = Uint8List.fromList(utf8.encode('hello opene2ee'));
      final sig = await id.sign(message);
      expect(sig.length, ed.SignatureSize);
      expect(id.verify(message, sig), isTrue);
    });

    test('verify rejects a tampered message', () async {
      final id = await DeviceIdentity.loadOrCreate(secure: _InMemorySecureStorage());
      final message = Uint8List.fromList(utf8.encode('hello opene2ee'));
      final sig = await id.sign(message);
      final tampered = Uint8List.fromList(utf8.encode('hello opene2ef'));
      expect(id.verify(tampered, sig), isFalse);
    });
  });

  group('DeviceIdentity.reset', () {
    test('clears the storage and loadOrCreate then mints fresh', () async {
      final storage = _InMemorySecureStorage();
      final first = await DeviceIdentity.loadOrCreate(secure: storage);
      await first.reset();
      final second = await DeviceIdentity.loadOrCreate(secure: storage);
      expect(second.uuidV7, isNot(first.uuidV7));
      expect(second.ed25519PublicKey, isNot(first.ed25519PublicKey));
    });
  });

  group('Privacy invariants (ADR-0006)', () {
    test('no field, getter, or method name declares a forbidden hardware id',
        () async {
      // ADR-0006 §"Veri Minimizasyonu" — IMEI / Serial / phoneNumber /
      // MSISDN / MAC are forbidden. We scan the source for Dart-level
      // declarations (field / getter / method name) of those identifiers,
      // which is the only way they could be plumbed through the type
      // system. Free-form mentions in comments / doc strings are allowed
      // (and necessary) to document the contract.
      final src = File('lib/shared/device_identity.dart').readAsStringSync();
      final pattern = RegExp(
        r'\b(?:String|int|Uint8List|List<int>)\s+(imei|serial|phoneNumber|msisdn|MAC|macAddress)\b'
        r'|\b(?:String|int)\s+get\s+(imei|serial|phoneNumber|msisdn|MAC|macAddress)\b',
        caseSensitive: false,
      );
      final matches = pattern.allMatches(src).toList();
      expect(matches, isEmpty,
          reason: 'device_identity.dart must not declare fields/getters '
              'named after hardware identifiers (ADR-0006). Matches: '
              '${matches.map((m) => m.group(0)).toList()}');
    });

    test('no Flutter device-info plugin is imported', () async {
      // belt-and-braces: device_info is the standard package that exposes
      // Imei / serialNumber on Android. We never want to depend on it.
      final src = File('lib/shared/device_identity.dart').readAsStringSync();
      expect(src.contains('device_info'), isFalse,
          reason: 'device_identity.dart must not import device_info plugin');
    });
  });
}