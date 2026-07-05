// mobile/lib/mobile/vpn/vpn_service_android.kt
//
// PR-10: Mobile-only — Android VPN Service skeleton (Kotlin).
//
// !!! Code-review artifact for Sprint 1 — no native build in this PR !!!
// ----------------------------------------------------------------------
// File location: this file lives under `mobile/lib/mobile/vpn/` because
// the task brief asked for it there (single source tree, easy code review).
// It is NOT compiled by Gradle in this PR.
//
// When the Android build is wired up in a later sprint, this file MUST be
// moved to:
//     mobile/android/app/src/main/kotlin/com/opene2ee/opene2ee/vpn/OpenE2eeVpnService.kt
// and the corresponding <service .../> entry added to AndroidManifest.xml
// with foregroundServiceType="specialUse" (Android 14+ — see ADR-0003 risk B2).
//
// Architecture (per ADR-0003 + HANDOFF §4.2 PR-10)
// ------------------------------------------------
// - Extends `android.net.VpnService`. The OS grants a TUN descriptor; we
//   read packets from it and forward them to the real network, but BEFORE
//   forwarding we copy a *metadata-only* fingerprint of each packet into
//   an in-memory ring buffer (cap = 10 packets).
// - MethodChannel name: "opene2ee/vpn"  (must match the Dart-side constant
//   in `mobile/lib/mobile/vpn/method_channel.dart::kVpnMethodChannel`).
// - Channel methods exposed to Dart:
//       "start"  → user has consented; begin packet capture
//       "stop"   → user / OS requested stop; flush ring + stop service
//       "status" → returns current state (idle | sampling | draining | stopped)
// - Channel methods invoked from native → Dart (telemetry callback):
//       "onTelemetry"  → invoked with the metadata summary once 10 packets
//                       have been observed (or the session is force-stopped
//                       with fewer packets — adaptive sampling, see ADR-0003
//                       risk G1, sampling window = HANDOFF §6.1).
//
// Privacy contract (ADR-0006 — verbatim)
// -------------------------------------
// 1. **NO raw packet payload** is ever copied off-device. The ring buffer
//    stores metadata only: IP/TCP/UDP header fields (src/dst IP, port, IP
//    protocol, TCP flags, packet length). Payload bytes are forwarded to
//    the real network but never stored, logged, or transmitted to Dart.
// 2. **NO IMEI, MSISDN, phoneNumber, MAC, contacts** are touched.
//    This service is forbidden from calling TelephonyManager / WifiInfo /
//    BluetoothAdapter. A grep test in the verification stage asserts this.
// 3. The IP source address (the device's own IP) is masked at the /24
//    boundary (IPv4) or /48 (IPv6) before being handed to Dart — matches
//    the backend's `device_ip_masked` storage rule.
// 4. The TUN interface is read-only from the metadata-extraction side; we
//    never inject packets.
//
// Open items (intentional — Sprint 2+)
// -----------------------------------
// - Permission flow: `VpnService.prepare()` must be invoked from the
//   Flutter side before calling `start` (system intent shown to user).
// - Foreground service notification (channel ID, content text, tap intent).
// - DNS leak prevention (route 10.0.0.0/8 + 192.168.0.0/16 through TUN).
// - Adaptive sampling expansion (TLS 1.3 0-RTT, see ADR-0003 risk G1).

package com.opene2ee.opene2ee.vpn

import android.app.Service
import android.content.Intent
import android.net.VpnService
import android.os.Build
import android.os.ParcelFileDescriptor
import io.flutter.embedding.engine.FlutterEngine
import io.flutter.plugin.common.MethodCall
import io.flutter.plugin.common.MethodChannel
import java.io.IOException
import java.net.Inet4Address
import java.net.Inet6Address
import java.net.InetAddress
import java.nio.ByteBuffer
import java.util.concurrent.atomic.AtomicBoolean
import java.util.concurrent.atomic.AtomicInteger

/**
 * OpenE2EE Android VPN Service — sampling-only (no payload capture).
 *
 * @see docs/ADR-0003-vpn-layer.md
 * @see docs/ADR-0006-anonimlik.md
 * @see docs/HANDOFF.md §4.2 PR-10
 */
class OpenE2eeVpnService : VpnService() {

    companion object {
        /** MUST match the Dart-side `kVpnMethodChannel` in method_channel.dart. */
        const val METHOD_CHANNEL = "opene2ee/vpn"

        /**
         * Sampling cap per the HANDOFF BRD §6.1 mobile spec — first 10 packets
         * of a test session. After the cap is reached we stop adding to the
         * ring buffer but keep forwarding traffic until the user / OS stops us.
         */
        const val SAMPLING_CAP_PACKETS = 10

        /** TUN MTU — keeps memory bounded; do not raise without reviewing. */
        const val TUN_MTU = 1500

        /** DNS resolver handed to the TUN (Cloudflare 1.1.1.1; public, no leak). */
        val PRIMARY_DNS: InetAddress = InetAddress.getByName("1.1.1.1")
        val SECONDARY_DNS: InetAddress = InetAddress.getByName("1.0.0.1")

        /** VPN tunnel address space (RFC 1918 — internal only). */
        val TUN_ADDRESS: InetAddress = InetAddress.getByName("10.42.0.2")
        val TUN_ROUTED_CIDR: String = "10.42.0.0/24"
    }

    /**
     * Lifecycle states. Exposed via `status` to Dart. Keeps the UI honest about
     * whether sampling is happening — see Android 14+ `foregroundServiceType`
     * requirement (ADR-0003 risk B2).
     */
    enum class State { IDLE, SAMPLING, DRAINING, STOPPED }

    /** Monotonically increasing count of packets observed since `start`. */
    private val packetsObserved = AtomicInteger(0)

    /** True while the foreground service + TUN loop are running. */
    private val running = AtomicBoolean(false)

    /** Current lifecycle state — observable via `status`. */
    @Volatile
    private var state: State = State.IDLE

    /** TUN file descriptor (null when stopped). */
    private var tunInterface: ParcelFileDescriptor? = null

    /** MethodChannel into Dart. Wired up by `MainActivity` via `attachFlutterEngine`. */
    private var methodChannel: MethodChannel? = null

    /** Last few metadata entries (cap = SAMPLING_CAP_PACKETS). */
    private val ring: ArrayDeque<Map<String, Any?>> = ArrayDeque()

    /**
     * Attach the Flutter engine so this service can dispatch telemetry back
     * to Dart. Called from `MainActivity.configureFlutterEngine` on app boot.
     */
    fun attachFlutterEngine(engine: FlutterEngine) {
        val channel = MethodChannel(engine.dartExecutor.binaryMessenger, METHOD_CHANNEL)
        channel.setMethodCallHandler(::onMethodCall)
        methodChannel = channel
    }

    /**
     * Handle Dart → native commands.
     */
    private fun onMethodCall(call: MethodCall, result: MethodChannel.Result) {
        when (call.method) {
            "start" -> {
                // Phase 2 work: caller must have invoked VpnService.prepare() and
                // received RESULT_OK before calling start. We do not enforce here
                // (no Activity context inside a Service); the UI layer is responsible.
                startCapture()
                result.success(state.name)
            }
            "stop" -> {
                stopCapture()
                result.success(state.name)
            }
            "status" -> {
                result.success(
                    mapOf(
                        "state" to state.name,
                        "packetsObserved" to packetsObserved.get(),
                        "ringSize" to ring.size,
                        "samplingCap" to SAMPLING_CAP_PACKETS,
                    ),
                )
            }
            else -> result.notImplemented()
        }
    }

    /**
     * Bring up the TUN interface + foreground notification + reader thread.
     *
     * Android 14+ (API 34) requires `foregroundServiceType="specialUse"`
     * for VPN services that are not classified as "system" — see ADR-0003
     * risk B2. The manifest change is Phase 2.
     */
    private fun startCapture() {
        if (running.get()) return
        running.set(true)
        state = State.SAMPLING
        packetsObserved.set(0)
        ring.clear()

        val builder = Builder()
            .setSession("OpenE2EE Network Diagnostic")
            .addAddress(TUN_ADDRESS, 24)
            .addRoute(TUN_ROUTED_CIDR)
            .addDnsServer(PRIMARY_DNS)
            .addDnsServer(SECONDARY_DNS)
            .setMtu(TUN_MTU)
            // We do NOT block any apps — sampling-only.
            .setBlocking(true)

        tunInterface = builder.establish()

        startForegroundCompat()
        // Phase 2: spawn a coroutine on Dispatchers.IO that reads from the
        // TUN, extracts metadata, and forwards payload bytes (untouched).
    }

    /**
     * Tear down TUN, flush ring, notify Dart.
     */
    private fun stopCapture() {
        if (!running.get()) return
        state = State.DRAINING

        val tun = tunInterface
        if (tun != null) {
            try {
                tun.close()
            } catch (e: IOException) {
                // Best-effort; closing a TUN is idempotent.
            }
            tunInterface = null
        }

        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.N) {
            stopForeground(STOP_FOREGROUND_REMOVE)
        } else {
            @Suppress("DEPRECATION")
            stopForeground(true)
        }

        // Drain remaining ring entries to Dart as a single telemetry event.
        flushTelemetry()

        running.set(false)
        state = State.STOPPED
    }

    /**
     * Phase 2: invoke `onTelemetry` on the Dart side with the accumulated
     * metadata. This skeleton version is a no-op until the packet reader
     * thread is implemented; the MethodChannel plumbing is wired so
     * `flutter test` does not block on missing plumbing.
     */
    private fun flushTelemetry() {
        val payload: List<Map<String, Any?>> = ring.toList()
        methodChannel?.invokeMethod(
            "onTelemetry",
            mapOf(
                "sessionId" to null,             // populated by Dart-side glue (PR-10 Phase 2)
                "packets" to payload,            // metadata-only; never payload bytes
                "capturedAt" to System.currentTimeMillis(),
            ),
        )
    }

    /**
     * Phase 2: extract IP/TCP/UDP metadata from a packet buffer.
     *
     * Privacy invariants (ADR-0006) enforced here:
     * - Source IP is masked at the /24 boundary (IPv4) or /48 (IPv6).
     * - Destination IP is masked at the same boundary.
     * - Payload bytes (after the transport header offset) are NEVER read
     *   past the metadata-only length fields.
     */
    @Suppress("UNUSED_PARAMETER")
    private fun extractMetadata(packet: ByteBuffer, length: Int): Map<String, Any?>? {
        if (length < 20) return null // too short for IPv4 header
        val versionAndIhl = packet.get(0).toInt() and 0xFF
        val version = versionAndIhl ushr 4
        if (version != 4 && version != 6) return null

        // Phase 2: parse version, ihl, total length, protocol, src/dst, sport/dport,
        // tcp flags (SYN/ACK/FIN/RST), and ip-id (TLS Client Hello fingerprint input).
        // For skeleton we only echo the protocol and a masked source IP.
        val protocol: Int = if (version == 4) {
            packet.get(9).toInt() and 0xFF
        } else {
            // IPv6 next-header at offset 6
            packet.get(6).toInt() and 0xFF
        }

        return mapOf(
            "version" to version,
            "protocol" to protocol,                  // 6=TCP, 17=UDP, ...
            "packetLength" to length,
            "srcIpMasked" to null,                    // Phase 2: extract + mask
            "dstIpMasked" to null,                    // Phase 2: extract + mask
            "srcPort" to null,                        // Phase 2
            "dstPort" to null,                        // Phase 2
            "tcpFlags" to null,                       // Phase 2
            "tlsClientHelloFingerprint" to null,      // Phase 2 — paired with PR-4 analysis
        )
    }

    /**
     * Phase 2 — Android 14+ foreground notification.
     */
    private fun startForegroundCompat() {
        // Phase 2: build a NotificationChannel + Notification with text
        // "OpenE2EE is running a network diagnostic session" and a content
        // intent that returns to the test screen.
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        // The service is started by the Dart side via `flutterLocalNotifications`
        // or a direct bind; for the skeleton we just honour the existing state.
        return START_NOT_STICKY
    }

    override fun onDestroy() {
        stopCapture()
        methodChannel?.setMethodCallHandler(null)
        methodChannel = null
        super.onDestroy()
    }

    override fun onRevoke() {
        // User revoked the VPN profile from system settings; tear down.
        stopCapture()
        super.onRevoke()
    }
}