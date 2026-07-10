// mobile/lib/state/whatsapp_deeplink_provider.dart
//
// Sprint 10.0 + 10.1E — WhatsApp deep link helper.
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
// Audit gaps closed (Sprint 10.1E)
// --------------------------------
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

import 'package:url_launcher/url_launcher.dart';

class WhatsAppDeepLink {
  WhatsAppDeepLink._();

  static const String _intentPrefix = 'intent://send?';
  static const String _intentSuffix = '#Intent;scheme=whatsapp;package=com.whatsapp;end';
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

  /// Try to open WhatsApp with the prepared message via the
  /// Android Intent dispatcher. Returns true if the platform
  /// handed the intent to WhatsApp (or to the system intent
  /// picker when WhatsApp is not installed). Caller is
  /// responsible for showing a fallback snackbar when this is
  /// false.
  static Future<bool> tryOpen() async {
    final uri = buildUri();
    if (!await canLaunchUrl(uri)) {
      return false;
    }
    try {
      return await launchUrl(uri, mode: LaunchMode.externalApplication);
    } on Exception {
      return false;
    }
  }
}
