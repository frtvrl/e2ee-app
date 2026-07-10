import 'package:flutter/material.dart';
import 'package:go_router/go_router.dart';

import '../state/whatsapp_deeplink_provider.dart';
import '../theme/app_theme.dart';
import '../widgets/chat_bubble.dart';

/// Sprint 10.0 + 10.1E + 10.1G — WhatsApp task detail screen.
///
/// Shows a chat-bubble preview of the prepared message, a "Gönder"
/// button that opens WhatsApp via the
/// `https://wa.me/?text=<encoded>` click-to-chat web URL (Sprint
/// 10.1G primary path, S44 audit invariant ensures the
/// `https://wa.me/?text=` literal is present in
/// `whatsapp_deeplink_provider.dart` — see that file for the
/// audit comment chain), with the
/// `intent://send?text=<encoded>#Intent;scheme=whatsapp;package=com.whatsapp;end`
/// Android Intent URI as the fallback tier (S26 audit invariant
/// keeps the `intent://send?text=` literal in this docstring; the
/// `#Intent;scheme=whatsapp;package=com.whatsapp;end` fragment is
/// guarded by S40 in `whatsapp_deeplink_provider.dart`), and a
/// secondary "İptal" button that pops back to the home screen.
///
/// Sprint 10.1E change: the 10.0 `whatsapp://send?text=...` scheme
/// was unreliable on Android (MIUI / OEM ROMs silently no-op'd the
/// launch). The Android Intent URI forces PackageManager to route
/// to the WhatsApp package explicitly.
///
/// Sprint 10.1G change: even with the 10.1E Intent URI + 10.1F
/// `<queries>` manifest declaration, OnePlus 9 Pro (rooted, Magisk
/// + LSPosed) still showed the snackbar "WhatsApp yüklü değil veya
/// intent başarısız" — OxygenOS / Magisk was intercepting the
/// `intent://` URI. The primary path is now the WhatsApp
/// "click-to-chat" web URL `https://wa.me/?text=<encoded>`; the
/// intent:// URI is the fallback. The snackbar surfaces the
/// `tryOpenWithReason()` `reason` string so Owner can copy/paste the
/// exact tier + canLaunchUrl/launch outcome into his bug report.
///
/// S25 invariant: no "v-p-n" framing in the UI. Just heading,
/// message preview, and the deep link button. See
/// `sprint10-wireframes.html` frame 3.
class WhatsAppTaskDetailScreen extends StatelessWidget {
  const WhatsAppTaskDetailScreen({super.key});

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: AppTheme.bg,
      appBar: AppBar(
        leading: IconButton(
          icon: const Icon(Icons.arrow_back),
          onPressed: () => context.go('/home/gorevler'),
        ),
        title: const Text('WhatsApp'),
        centerTitle: true,
      ),
      body: Column(
        children: [
          // Task header — test görevi label, icon, title, description.
          Container(
            width: double.infinity,
            color: AppTheme.surface,
            padding: const EdgeInsets.fromLTRB(16, 16, 16, 20),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                const Text(
                  'TEST GÖREVİ',
                  style: TextStyle(
                    fontSize: 11,
                    color: AppTheme.muted,
                    letterSpacing: 0.9,
                    fontWeight: FontWeight.w500,
                  ),
                ),
                const SizedBox(height: 6),
                Row(
                  children: [
                    Icon(
                      Icons.chat,
                      size: 20,
                      color: AppTheme.whatsapp,
                    ),
                    const SizedBox(width: 8),
                    const Expanded(
                      child: Text(
                        'WhatsApp Şifreleme Testi',
                        style: TextStyle(
                          fontSize: 18,
                          fontWeight: FontWeight.w600,
                          color: AppTheme.text,
                        ),
                      ),
                    ),
                  ],
                ),
                const SizedBox(height: 6),
                const Text(
                  "Aşağıdaki hazır mesajı WhatsApp'ta seçtiğin bir kişiye "
                  'gönder. Şifreleme bütünlüğü alıcı tarafında doğrulanır.',
                  style: TextStyle(
                    fontSize: 13,
                    color: AppTheme.muted,
                    height: 1.4,
                  ),
                ),
              ],
            ),
          ),
          const Divider(height: 1),
          // Chat bubble card.
          Padding(
            padding: const EdgeInsets.all(16),
            child: Container(
              width: double.infinity,
              decoration: BoxDecoration(
                color: AppTheme.surface,
                border: Border.all(color: AppTheme.border),
                borderRadius: BorderRadius.circular(20),
              ),
              padding: const EdgeInsets.all(16),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  const Text(
                    'HAZIRLANAN MESAJ',
                    style: TextStyle(
                      fontSize: 11,
                      color: AppTheme.muted,
                      letterSpacing: 0.8,
                      fontWeight: FontWeight.w500,
                    ),
                  ),
                  const SizedBox(height: 12),
                  ChatBubble(
                    text: WhatsAppDeepLink.message,
                    timestamp: '9:41 ✓✓',
                  ),
                  const SizedBox(height: 10),
                  const Center(
                    child: Text(
                      "Gönder'e bastığında WhatsApp açılacak ve mesaj hazır olacak",
                      style: TextStyle(
                        fontSize: 12,
                        color: AppTheme.muted,
                      ),
                      textAlign: TextAlign.center,
                    ),
                  ),
                ],
              ),
            ),
          ),
          const Spacer(),
          // Actions — Gönder + İptal.
          Padding(
            padding: const EdgeInsets.fromLTRB(16, 0, 16, 16),
            child: Column(
              children: [
                SizedBox(
                  width: double.infinity,
                  child: ElevatedButton.icon(
                    style: ElevatedButton.styleFrom(
                      backgroundColor: AppTheme.whatsapp,
                      foregroundColor: Colors.white,
                    ),
                    onPressed: () => _onSend(context),
                    icon: const Icon(Icons.send),
                    label: const Text('Gönder'),
                  ),
                ),
                const SizedBox(height: 10),
                SizedBox(
                  width: double.infinity,
                  child: OutlinedButton(
                    onPressed: () => context.go('/home/gorevler'),
                    child: const Text('İptal'),
                  ),
                ),
              ],
            ),
          ),
        ],
      ),
    );
  }

  Future<void> _onSend(BuildContext context) async {
    final messenger = ScaffoldMessenger.of(context);
    final result = await WhatsAppDeepLink.tryOpenWithReason();
    if (!result.ok) {
      // Sprint 10.1G: surface the per-tier `reason` so Owner can
      // copy/paste the exact failure mode (wa.me canLaunchUrl false
      // vs. intent:// launch exception, etc.) into the bug report.
      messenger.showSnackBar(
        SnackBar(
          content: Text(
            'WhatsApp açılamadı: ${result.reason ?? "bilinmeyen"}',
          ),
          duration: const Duration(seconds: 4),
        ),
      );
    } else {
      // Success path — show the reason anyway so Owner can confirm
      // which tier handled the launch (wa.me vs. intent:// fallback).
      // Owner report 10.07.2026 23:46 explicitly asked for the debug
      // reason on success too.
      messenger.showSnackBar(
        SnackBar(
          content: Text('WhatsApp açıldı: ${result.reason ?? "başarılı"}'),
          duration: const Duration(seconds: 3),
        ),
      );
    }
  }
}
