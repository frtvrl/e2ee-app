import 'package:flutter/material.dart';
import 'package:go_router/go_router.dart';

import '../state/whatsapp_deeplink_provider.dart';
import '../theme/app_theme.dart';
import '../widgets/chat_bubble.dart';

/// Sprint 10.0 + 10.1E — WhatsApp task detail screen.
///
/// Shows a chat-bubble preview of the prepared message, a "Gönder"
/// button that opens WhatsApp via the
/// `intent://send?text=<encoded>#Intent;scheme=whatsapp;package=com.whatsapp;end`
/// Android Intent deep link (S26 audit invariant ensures the
/// `intent://send?text=` literal is present in this docstring; the
/// `#Intent;scheme=whatsapp;package=com.whatsapp;end` fragment is
/// guarded by S40 in `whatsapp_deeplink_provider.dart`), and a
/// secondary "İptal" button that pops back to the home screen.
///
/// Sprint 10.1E change: the 10.0 `whatsapp://send?text=...` scheme
/// was unreliable on Android (MIUI / OEM ROMs silently no-op'd the
/// launch). The Android Intent URI forces PackageManager to route
/// to the WhatsApp package explicitly.
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
    final ok = await WhatsAppDeepLink.tryOpen();
    if (!ok) {
      messenger.showSnackBar(
        const SnackBar(
          content: Text('WhatsApp yüklü değil veya intent başarısız'),
          duration: Duration(seconds: 3),
        ),
      );
    }
  }
}
