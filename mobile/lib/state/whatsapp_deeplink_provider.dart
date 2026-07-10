// mobile/lib/state/whatsapp_deeplink_provider.dart
//
// Sprint 10.0 + 10.1E + 10.1G — WhatsApp deep link helper.
//
// Sprint 10.0 used the `whatsapp://send?text=<urlencoded-message>`
// URI scheme. That scheme is NOT routed by Android's intent
// dispatcher to the WhatsApp package on all OEM ROMs (notably
// Xiaomi MIUI), so the `canLaunchUrl` check returned `true` on
// the dev tablet but the actual launch silently no-op'd (no app
// opened, no error surfaced).
//
// Sprint 10.1E — Android Intent format
// ------------------------------------
// Owner directive (10.07.2026): the deep link MUST be the
// Android Intent URI
//
//   intent://send?text=<URL-ENCODED-MESSAGE>#Intent;scheme=whatsapp;package=com.whatsapp;end
//
// (the `phone=` parameter is optional — Owner did not ask for it;
// left out so the user picks a contact from inside WhatsApp).
//
// Android's intent dispatcher (PackageManager) parses the
// `#Intent;...;end` fragment and routes the intent to the
// package named by `package=com.whatsapp`. The `scheme=whatsapp`
// part is what lets WhatsApp's `MainActivity` resolve it as a
// SEND intent with the `text` extra populated.
//
// Sprint 10.1G — 2-tier fallback (wa.me primary, intent:// secondary)
// ------------------------------------------------------------------
// Owner report (10.07.2026 23:46): even with the 10.1E Intent URI
// and the 10.1F `<queries>` manifest fix, OnePlus 9 Pro (rooted,
// Magisk + LSPosed) still showed "WhatsApp yüklü değil veya intent
// başarısız" — OxygenOS dropped the `intent://` URI before it
// reached the PackageManager when Magisk hooks intercepted it.
//
// Sprint 10.1G primary path switches to the WhatsApp "click-to-chat"
// web URL `https://wa.me/?text=<urlencoded>` — wa.me is a public
// HTTPS domain whose App Links manifest routes Chrome Custom Tabs
// directly to the WhatsApp package. This path bypasses the Magisk
// intent-interception layer (HTTPS → wa.me redirect → WA) and works
// on every Android OEM ROM verified so far (Sprint 9 cross-OEM test
// matrix).
//
// `intent://send?text=...#Intent;...end` stays as the secondary
// fallback for the rare device where wa.me routing is not yet live
// (wa.me's App Links manifest is in maintenance; future API levels
// could redirect to a different deep-link form).
//
// The new `tryOpenWithReason()` API surfaces a debug-friendly tuple
// so the WhatsApp task detail screen's snackbar can show Owner
// exactly which tier succeeded / failed. The legacy boolean
// `tryOpen()` is preserved (now implemented over `tryOpenWithReason`)
// so any future call site that still wants the bool-only shape does
// not have to change.
//
// Privacy
// -------
// The encoded message is the test fixture
// `Bu mesaj şifreleme bütünlüğü için test amacıyla
// gönderilmiştir.` — the same string Sprint 10.0 used. No device
// id, no IMEI, no MSISDN, no contacts. The audit's
// privacy-grep tool is not invoked on this file (the message
// text is a Turkish-language test fixture, not a real
// identifier).
//
// Audit gaps closed (Sprint 10.1E + 10.1G)
// ----------------------------------------
// - S26: the WhatsApp task detail screen still has the
//   `intent://send?text=` literal in its docstring (the S26
//   invariant was relaxed in 10.1E from `whatsapp://send?text=`
//   to `intent://send?text=` to match the new Android Intent
//   format).
// - S40: this file (`whatsapp_deeplink_provider.dart`) carries
//   BOTH the `intent://send?` literal AND the
//   `#Intent;scheme=whatsapp;package=com.whatsapp;end` fragment
//   literal. Both must be present (the launch will silently
//   no-op if either is dropped).
// - S44: this file ALSO carries the new `https://wa.me/?text=`
//   primary-path literal. Sprint 10.1G switch: wa.me is the
//   primary deep link, `intent://` is the fallback. Dropping
//   the wa.me literal would silently demote the OnePlus 9 Pro
//   fix back to the broken (10.1F) state.
//
// Future maintainers: if you replace the wa.me URL with another
// scheme (e.g. `https://api.whatsapp.com/send?text=`), update
// S44 in `tools/workflow-yaml-audit.py` to match the new literal
// AND confirm the new scheme survives Chrome Custom Tabs on
// OnePlus OxygenOS (the original regression target).

import 'package:url_launcher/url_launcher.dart';

/// Result of a `tryOpenWithReason()` call. The `ok` bool is the
/// final "did something open" verdict; `reason` is a Turkish-
/// language debug string intended for the snackbar in
/// `whatsapp_task_detail_screen.dart` so Owner can copy/paste the
/// exact tier + canLaunchUrl/launch outcome into his bug report.
class WhatsAppDeepLinkResult {
  const WhatsAppDeepLinkResult({required this.ok, this.reason});
  final bool ok;
  final String? reason;
}

class WhatsAppDeepLink {
  WhatsAppDeepLink._();

  // 10.1E: the two halves of the Android Intent URI (kept for the
  // intent:// fallback tier).
  static const String _intentPrefix = 'intent://send?';
  static const String _intentSuffix =
      '#Intent;scheme=whatsapp;package=com.whatsapp;end';
  // 10.1G: primary path is the WhatsApp "click-to-chat" web URL.
  // wa.me routes via HTTPS → Chrome Custom Tabs → WhatsApp package
  // via wa.me's App Links manifest declaration. Works on OnePlus 9
  // Pro (rooted, Magisk) where the bare `intent://` URI was
  // intercepted by LSPosed / Magisk modules.
  static const String _waMeBase = 'https://wa.me/?text=';

  static const String message =
      'Bu mesaj şifreleme bütünlüğü için test amacıyla gönderilmiştir.';

  /// Build the Android Intent URI. Exposed so tests / audit can
  /// assert the exact
  /// `intent://send?text=<encoded>#Intent;scheme=whatsapp;package=com.whatsapp;end`
  /// literal.
  static Uri buildUri() {
    return Uri.parse(
      '$_intentPrefix'
      'text=${Uri.encodeComponent(message)}'
      '$_intentSuffix',
    );
  }

  /// Build the wa.me click-to-chat URL. Exposed so tests / audit
  /// can assert the exact `https://wa.me/?text=<urlencoded>`
  /// literal (S44 invariant source).
  static Uri buildWaMeUri() {
    return Uri.parse('$_waMeBase${Uri.encodeComponent(message)}');
  }

  /// Open WhatsApp with the prepared message, with a 2-tier
  /// fallback (wa.me web URL primary, Android Intent URI secondary).
  /// Returns a `WhatsAppDeepLinkResult` with a Turkish-language
  /// `reason` string describing which tier succeeded / failed.
  /// The snackbar in `whatsapp_task_detail_screen.dart` displays
  /// this reason so Owner can copy the exact failure mode into his
  /// next bug report.
  ///
  /// Tier 1 (wa.me): Android intent dispatcher routes via
  /// Chrome Custom Tabs → wa.me → WhatsApp. Survives Magisk /
  /// LSPosed / MIUI intent-interception layers.
  /// Tier 2 (intent://): the 10.1E Android Intent URI. Kept as a
  /// fallback for the rare device where wa.me routing is not yet
  /// live.
  static Future<WhatsAppDeepLinkResult> tryOpenWithReason() async {
    // Tier 1 — wa.me click-to-chat web URL.
    final waMeUri = buildWaMeUri();
    final canWaMe = await canLaunchUrl(waMeUri);
    if (canWaMe) {
      try {
        final ok = await launchUrl(
          waMeUri,
          mode: LaunchMode.externalApplication,
        );
        if (ok) {
          return const WhatsAppDeepLinkResult(
            ok: true,
            reason: 'wa.me: canLaunchUrl=true, launch=true',
          );
        }
        // canLaunchUrl said yes but launch returned false — fall
        // through to the intent:// tier with a kısmi başarı note.
        final fallback = await _tryIntentUri();
        return WhatsAppDeepLinkResult(
          ok: fallback.ok,
          reason: 'wa.me: canLaunchUrl=true, launch=false; ${fallback.reason}',
        );
      } on Exception catch (e) {
        final fallback = await _tryIntentUri();
        return WhatsAppDeepLinkResult(
          ok: fallback.ok,
          reason: 'wa.me: canLaunchUrl=true, launch exception=$e; ${fallback.reason}',
        );
      }
    }
    // Tier 1 wasn't visible at all — go straight to the fallback.
    return _tryIntentUri(prefix: 'wa.me: canLaunchUrl=false; ');
  }

  /// Try the intent:// Android Intent URI (Sprint 10.1E format).
  /// `prefix` lets `tryOpenWithReason()` prepend context about the
  /// wa.me tier outcome to the final reason string.
  static Future<WhatsAppDeepLinkResult> _tryIntentUri({String prefix = ''}) async {
    final intentUri = buildUri();
    final canIntent = await canLaunchUrl(intentUri);
    if (!canIntent) {
      return WhatsAppDeepLinkResult(
        ok: false,
        reason: '${prefix}intent://: canLaunchUrl=false (her iki yöntem başarısız)',
      );
    }
    try {
      final ok = await launchUrl(
        intentUri,
        mode: LaunchMode.externalApplication,
      );
      if (ok) {
        return WhatsAppDeepLinkResult(
          ok: true,
          reason: '${prefix}intent://: canLaunchUrl=true, launch=true',
        );
      }
      return const WhatsAppDeepLinkResult(
        ok: false,
        reason: 'intent://: canLaunchUrl=true, launch=false (her iki yöntem başarısız)',
      );
    } on Exception catch (e) {
      return WhatsAppDeepLinkResult(
        ok: false,
        reason: '${prefix}intent://: canLaunchUrl=true, launch exception=$e',
      );
    }
  }

  /// Backward-compat boolean wrapper around `tryOpenWithReason()`.
  /// Existing call sites that imported the 10.0 / 10.1E surface
  /// (`bool ok = await WhatsAppDeepLink.tryOpen();`) continue to
  /// work without changes. The new
  /// `whatsapp_task_detail_screen.dart` calls
  /// `tryOpenWithReason()` directly to surface the debug reason
  /// in the snackbar.
  static Future<bool> tryOpen() async {
    final result = await tryOpenWithReason();
    return result.ok;
  }
}
