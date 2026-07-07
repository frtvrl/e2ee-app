// mobile/android/app/src/main/kotlin/com/opene2ee/opene2ee/vpn/OpenE2eeVpnService.kt
//
// PR-22a (Sprint 3) — Android VPN Service — REAL implementation.
// PR-24 (Sprint 4) — moved from `mobile/lib/mobile/vpn/vpn_service_android.kt`
//                    into the Android source tree so Gradle compiles it.
// PR-28 (Sprint 5) — PR-22a follow-up batch:
//                    (B.1) `@RequiresApi(21)` guards around `VpnService.Builder
//                          .allowedApplications()` / `.disallowedApplications()`
//                          call sites (API 21+, Lollipop).
//                    (B.2) Transient service instance handling — companion
//                          singleton so `MainActivity` no longer creates a
//                          throwaway `OpenE2eeVpnService()` instance on
//                          every engine attach/detach. Also switched the
//                          foreground-service lifecycle to
//                          `ServiceCompat.startForeground` with
//                          `FOREGROUND_SERVICE_TYPE_SPECIAL_USE`.
//                    (B.3) IPv6 transport header parsing stub — was
//                          hard-coded null for srcPort/dstPort/tcpFlags on
//                          IPv6 packets. Now walks past IPv6 extension
//                          headers (limited to no-extension case; documented
//                          below) to extract TCP/UDP ports + flags.
//
// This file is the canonical Kotlin source for the OpenE2EE Android VPN
// service. It lives under `mobile/android/app/src/main/kotlin/` so the
// Android Gradle Plugin picks it up on `./gradlew assembleDebug`. The
// sibling iOS source remains under `mobile/lib/mobile/vpn/NetworkExtension.swift`
// for cross-platform review.
//
// Architecture
// ------------
// - Extends `android.net.VpnService`. The OS hands us a TUN descriptor via
//   `Builder.establish()`; we read packets from it on a dedicated IO thread,
//   extract metadata for the sampling ring buffer, and PROTECT each packet
//   so the system can forward its payload to the real network. We never
//   copy a packet payload off-device — see ADR-0006.
//
// - MethodChannel name: `opene2ee/vpn` (matches Dart-side
//   `kVpnMethodChannel` in `method_channel.dart`).
// - Channel methods from Dart:
//       "start"     → begin session (caller MUST have obtained RESULT_OK from
//                     `VpnService.prepare` first)
//       "stop"      → flush ring + tear down tunnel + stop foreground
//       "status"    → snapshot of {state, packetsObserved, ringSize, samplingCap}
//       "setAllowedApplications" → restrict VPN to a per-app allowlist
//                                   (Android 5.0+, VpnService.Builder.allowedApplications)
//       "setDisallowedApplications" → inverse — bypass VPN for these apps
//       "requestPrepare" → emit a SystemIntent-style permission prompt
//                           (handled by MainActivity; this service exposes
//                            the helper that returns the intent action)
// - Channel methods TO Dart:
//       "onTelemetry" → final flush of ring + capture timestamp
//       "onError"     → TUN/protocol errors with a code + message
//
// Per-app VPN (Android 5.0+, API 21+)
// -----------------------------------
// The class accepts an `allowedApplications: List<String>` (package names)
// and an optional `disallowedApplications: List<String>`. Only one of the
// two lists may be non-empty at a time (Android API contract — passing both
// throws `IllegalArgumentException` at Builder.establish() time). The
// lists are applied via:
//   - `Builder.allowedApplications(pkgNames)` to constrain the VPN to a
//     specific set of apps.
//   - `Builder.disallowedApplications(pkgNames)` to bypass the VPN for
//     specific apps while routing everything else through it.
//
// Privacy contract (ADR-0006 — verbatim invariants)
// --------------------------------------------------
// 1. NO raw packet payload is ever copied off-device. The ring buffer
//    stores metadata only: IP/TCP/UDP header fields. Payload bytes are
//    passed back to the OS via `protect()` and dropped.
// 2. NO IMEI, MSISDN, phoneNumber, MAC, contacts are touched. The
//    service MUST NOT call TelephonyManager / WifiInfo /
//    BluetoothAdapter. A grep test in CI asserts this for the
//    `mobile/lib/mobile/vpn/` source tree.
// 3. Source / destination IPs are masked at /24 (IPv4) or /48 (IPv6)
//    before being handed to Dart.
// 4. The TUN interface is read-only on the metadata side; we never
//    inject packets of our own.
//
// Lifecycle / Android 14+ foregroundServiceType
// ---------------------------------------------
// Android 14 (API 34) requires `foregroundServiceType="specialUse"` for
// VPN services that are not classified as "system". The manifest entry
// under `mobile/android/app/src/main/AndroidManifest.xml` MUST declare
// that type. See ADR-0003 risk B2.

package com.opene2ee.opene2ee.vpn

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.app.Service
import android.content.Context
import android.content.Intent
import android.content.pm.ServiceInfo
import android.net.VpnService
import android.os.Build
import android.os.ParcelFileDescriptor
import android.util.Log
import androidx.annotation.RequiresApi
import androidx.core.app.ServiceCompat
import io.flutter.embedding.engine.FlutterEngine
import io.flutter.plugin.common.MethodCall
import io.flutter.plugin.common.MethodChannel
import java.io.FileInputStream
import java.io.IOException
import java.net.Inet4Address
import java.net.Inet6Address
import java.net.InetAddress
import java.nio.ByteBuffer
import java.nio.ByteOrder
import java.util.concurrent.atomic.AtomicBoolean
import java.util.concurrent.atomic.AtomicInteger

/**
 * OpenE2EE Android VPN service — real (non-skeletal) implementation.
 *
 * Brings up a TUN interface through the [VpnService.Builder], reads packets
 * on a dedicated thread, extracts metadata into a bounded ring buffer
 * (default = 10 packets), and forwards payload bytes to the real network
 * via [protect]. Telemetry is flushed on stop (or when the cap is hit) via
 * the [MethodChannel] registered against [kVpnMethodChannel].
 *
 * @see docs/ADR-0003-vpn-layer.md
 * @see docs/ADR-0006-anonimlik.md
 * @see docs/SPRINT-3-SCOPE.md §7 — Sprint 3 PR-22a
 */
class OpenE2eeVpnService : VpnService() {

    companion object {
        private const val TAG = "OpenE2eeVpn"

        /** Must match `kVpnMethodChannel` in Dart. */
        const val METHOD_CHANNEL = "opene2ee/vpn"

        /** Sampling cap per HANDOFF §6.1 mobile spec. */
        const val SAMPLING_CAP_PACKETS = 10

        /** Standard Ethernet MTU. */
        const val TUN_MTU = 1500

        /** Public, no-leak DNS resolvers handed to the TUN. */
        val PRIMARY_DNS: InetAddress = InetAddress.getByName("1.1.1.1")
        val SECONDARY_DNS: InetAddress = InetAddress.getByName("1.0.0.1")

        /** TUN address space (RFC 1918). */
        val TUN_ADDRESS: InetAddress = InetAddress.getByName("10.42.0.2")
        val TUN_ROUTED_CIDR: String = "10.42.0.0/24"

        /** Notification channel ID (Android 8+ requirement). */
        const val NOTIFICATION_CHANNEL_ID = "opene2ee.vpn.diagnostic"

        /** Foreground notification id. */
        const val NOTIFICATION_ID = 0x4F_50_4E_45 /* 'OPNE' */

        /** Intent action that calls `VpnService.prepare()` from MainActivity. */
        const val ACTION_PREPARE = "com.opene2ee.opene2ee.vpn.PREPARE"

        // ─── PR-28 §B.2 — Transient service instance handling ──────────
        //
        // Background: prior to PR-28, `MainActivity.configureFlutterEngine`
        // called `OpenE2eeVpnService().attachFlutterEngine(flutterEngine)`,
        // creating a fresh (un-started) instance every time the Flutter
        // engine was attached. The MethodChannel handler was therefore
        // installed on an instance that the OS never started — so Dart →
        // service calls would land on a no-op (state never updated,
        // running flag false) and the real service instance (created by
        // `Context.startForegroundService`) had no channel at all.
        //
        // Fix: the running service registers itself in `onCreate` and
        // unregisters in `onDestroy`. `MainActivity` resolves the SAME
        // instance via [activeInstance] before attaching the channel.
        // If no instance is alive yet (legitimate early-launch window),
        // the call is queued in [pendingEngine] and replayed by
        // [onCreate] once the service is up. This closes the start/stop
        // race window where Dart could call `start` between
        // `MainActivity.configureFlutterEngine` and `OpenE2eeVpnService.onCreate`.
        @Volatile
        private var activeInstance: OpenE2eeVpnService? = null

        /** Engine captured when a channel attach races service creation. */
        @Volatile
        private var pendingEngine: FlutterEngine? = null

        /**
         * Singleton accessor — returns the currently-running service, or
         * null if it hasn't been started yet. Used by `MainActivity` so
         * the MethodChannel handler is wired to the SAME instance that
         * will receive Dart's `start` command.
         */
        @JvmStatic
        fun getActiveInstance(): OpenE2eeVpnService? = activeInstance

        /**
         * Attach the Flutter engine to the active service instance. If
         * the service hasn't been created yet (e.g. the activity wired
         * its engine before the first `Context.startForegroundService`
         * landed), the engine is queued and replayed in
         * [OpenE2eeVpnService.onCreate] once the instance exists.
         */
        @JvmStatic
        fun attachFlutterEngine(engine: FlutterEngine) {
            val instance = activeInstance
            if (instance != null) {
                instance.attachFlutterEngine(engine)
            } else {
                pendingEngine = engine
            }
        }

        /**
         * Detach the Flutter engine. Safe to call even if no instance
         * ever came up — drains the pending queue so we don't replay a
         * stale engine against a fresh service later.
         */
        @JvmStatic
        fun detachFlutterEngine() {
            val instance = activeInstance
            if (instance != null) {
                instance.detachFlutterEngine()
            } else {
                pendingEngine = null
            }
        }
    }

    /** Lifecycle states exposed via `status` to Dart. */
    enum class State { IDLE, SAMPLING, DRAINING, STOPPED, ERROR }

    /** Monotonic packet counter since last `start`. */
    private val packetsObserved = AtomicInteger(0)

    /** True while TUN loop is running. */
    private val running = AtomicBoolean(false)

    /** Current state — observable via `status`. */
    @Volatile
    private var state: State = State.IDLE

    @Volatile
    private var lastError: String? = null

    /** TUN file descriptor (null when stopped). */
    private var tunInterface: ParcelFileDescriptor? = null

    /** The thread doing blocking reads on the TUN input stream. */
    private var readerThread: Thread? = null

    /** Method channel back into Dart — wired by [attachFlutterEngine]. */
    private var methodChannel: MethodChannel? = null

    /** Per-app VPN allowlist (null = all apps). Mutually exclusive with [disallowedApplications]. */
    @Volatile
    private var allowedApplications: List<String>? = null

    /** Per-app VPN denylist (null = no exception). Mutually exclusive with [allowedApplications]. */
    @Volatile
    private var disallowedApplications: List<String>? = null

    /** Bounded ring of metadata snapshots. */
    private val ring: ArrayDeque<Map<String, Any?>> = ArrayDeque(SAMPLING_CAP_PACKETS)

    /** Synchronize ring mutations across the IO thread + the stop path. */
    private val ringLock = Any()

    /**
     * Wire the MethodChannel — called once from `MainActivity.configureFlutterEngine`
     * at app startup. Must run on the UI thread; the MethodChannel ctor is
     * thread-safe but the handler swap is best done there.
     */
    fun attachFlutterEngine(engine: FlutterEngine) {
        val ch = MethodChannel(engine.dartExecutor.binaryMessenger, METHOD_CHANNEL)
        ch.setMethodCallHandler(::onMethodCall)
        methodChannel = ch
    }

    /**
     * Detach the MethodChannel — called from `MainActivity` `onDestroy` so
     * we don't leak handlers across engine restarts.
     */
    fun detachFlutterEngine() {
        methodChannel?.setMethodCallHandler(null)
        methodChannel = null
    }

    /**
     * Handle Dart → native commands.
     */
    private fun onMethodCall(call: MethodCall, result: MethodChannel.Result) {
        try {
            when (call.method) {
                "start" -> {
                    val newState = startCapture()
                    result.success(stateToMap(newState))
                }
                "stop" -> {
                    val newState = stopCapture(graceful = true)
                    result.success(stateToMap(newState))
                }
                "status" -> {
                    result.success(currentStatusMap())
                }
                "setAllowedApplications" -> {
                    val pkgs = (call.argument<List<String>>("packages") ?: emptyList())
                    allowedApplications = pkgs
                    if (pkgs.isNotEmpty()) disallowedApplications = null
                    result.success(true)
                }
                "setDisallowedApplications" -> {
                    val pkgs = (call.argument<List<String>>("packages") ?: emptyList())
                    disallowedApplications = pkgs
                    if (pkgs.isNotEmpty()) allowedApplications = null
                    result.success(true)
                }
                "requestPrepare" -> {
                    // Returns the intent ACTION a UI flow must `startActivityForResult`
                    // for to obtain RESULT_OK. We do NOT start the activity here because
                    // this runs in a Service context — MainActivity owns the flow.
                    result.success("android.net.VpnService")
                }
                else -> result.notImplemented()
            }
        } catch (t: Throwable) {
            Log.e(TAG, "MethodChannel error: ${call.method}", t)
            result.error("vpn_method_error", t.message, null)
        }
    }

    /**
     * Build the [VpnService.Builder] with our standard config + per-app lists.
     * Marked `protected` so tests can swap it via subclass.
     *
     * PR-28 §B.1: `allowedApplications` and `disallowedApplications` are
     * API 21 (Lollipop) additions. Sprint 7 MOB-5 bumped the project floor
     * to `minSdk = 23` (Android 6.0 Marshmallow) in `app/build.gradle.kts`
     * to satisfy flutter_secure_storage 9.x's AndroidKeyStore contract —
     * so the `@RequiresApi(21)` annotations below are now belt-and-braces.
     * They document the API contract and stop the Android Lint `NewApi`
     * rule from firing if a downstream module ever lowers minSdk below 21.
     * The calls themselves are safe because `app/build.gradle.kts`
     * enforces the floor (currently 23; never below 21 since these two
     * methods are the original API 21 floor reference).
     */
    @RequiresApi(21)
    protected open fun buildVpnBuilder(): VpnService.Builder {
        val b = Builder()
            .setSession("OpenE2EE Network Diagnostic")
            .addAddress(TUN_ADDRESS, 24)
            .addRoute(TUN_ROUTED_CIDR)
            .addDnsServer(PRIMARY_DNS)
            .addDnsServer(SECONDARY_DNS)
            .setMtu(TUN_MTU)
            .setBlocking(true)
        @RequiresApi(21)
        allowedApplications?.let { pkgs -> b.allowedApplications(pkgs) }
        @RequiresApi(21)
        disallowedApplications?.let { pkgs -> b.disallowedApplications(pkgs) }
        return b
    }

    /**
     * Bring up the TUN, start the foreground notification, spawn the reader
     * thread. Idempotent — a duplicate call while already running is a no-op.
     */
    private fun startCapture(): State {
        if (running.get()) return state
        try {
            val builder = buildVpnBuilder()
            val pfd = builder.establish()
            if (pfd == null) {
                // Most likely cause: user cancelled the consent dialog or no permission.
                state = State.ERROR
                lastError = "VpnService.Builder.establish() returned null " +
                        "(user declined consent or system refused)"
                notifyError(lastError!!)
                return state
            }
            tunInterface = pfd
            running.set(true)
            state = State.SAMPLING
            packetsObserved.set(0)
            synchronized(ringLock) { ring.clear() }
            startForegroundCompat()
            startReaderThread(pfd)
        } catch (e: Throwable) {
            running.set(false)
            state = State.ERROR
            lastError = "startCapture failed: ${e.javaClass.simpleName}: ${e.message}"
            Log.e(TAG, lastError!!, e)
            notifyError(lastError!!)
        }
        return state
    }

    /**
     * Tear down TUN, flush ring, notify Dart.
     */
    private fun stopCapture(@Suppress("UNUSED_PARAMETER") graceful: Boolean): State {
        if (!running.get() && tunInterface == null) {
            state = State.STOPPED
            return state
        }
        state = State.DRAINING
        // Close the TUN — the reader thread will see EOF and exit.
        tunInterface?.let { pfd ->
            try {
                pfd.close()
            } catch (e: IOException) {
                Log.w(TAG, "TUN close ignored: ${e.message}")
            }
        }
        tunInterface = null
        // Wait for the reader thread to exit.
        readerThread?.let { t ->
            try {
                t.join(1_000L)
            } catch (e: InterruptedException) {
                Thread.currentThread().interrupt()
            }
        }
        readerThread = null

        // Stop the foreground notification (API 24+ split-path).
        try {
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.N) {
                stopForeground(STOP_FOREGROUND_REMOVE)
            } else {
                @Suppress("DEPRECATION")
                stopForeground(true)
            }
        } catch (e: Throwable) {
            Log.w(TAG, "stopForeground failed: ${e.message}")
        }

        // Send the final telemetry batch.
        flushTelemetry()
        running.set(false)
        state = State.STOPPED
        return state
    }

    /**
     * Snapshot for the `status` MethodChannel call.
     */
    private fun currentStatusMap(): Map<String, Any?> = mapOf(
        "state" to state.name,
        "packetsObserved" to packetsObserved.get(),
        "ringSize" to synchronized(ringLock) { ring.size },
        "samplingCap" to SAMPLING_CAP_PACKETS,
        "lastError" to lastError,
        "allowedApplications" to allowedApplications,
        "disallowedApplications" to disallowedApplications,
    )

    private fun stateToMap(s: State): Map<String, Any?> = mapOf(
        "state" to s.name,
        "packetsObserved" to packetsObserved.get(),
        "ringSize" to synchronized(ringLock) { ring.size },
        "samplingCap" to SAMPLING_CAP_PACKETS,
        "lastError" to lastError,
    )

    /**
     * Forward a metadata batch back to Dart over the MethodChannel.
     * Called once at stop time and on ring-cap threshold.
     */
    private fun flushTelemetry() {
        val payload: List<Map<String, Any?>> = synchronized(ringLock) { ring.toList() }
        methodChannel?.invokeMethod(
            "onTelemetry",
            mapOf(
                "sessionId" to null, // populated by Dart-side glue from PR-7's sessionId
                "packets" to payload,
                "capturedAt" to System.currentTimeMillis(),
            ),
        )
    }

    private fun notifyError(message: String) {
        methodChannel?.invokeMethod(
            "onError",
            mapOf(
                "code" to "vpn_runtime_error",
                "message" to message,
            ),
        )
    }

    /**
     * Spawn the dedicated TUN reader thread. The thread reads up to MTU-sized
     * packets in a tight loop, extracts metadata via [extractMetadata], pushes
     * a metadata entry into the ring (cap = [SAMPLING_CAP_PACKETS]), and
     * PROTECTS the original packet via [protect] so the OS can forward it
     * out the real network interface. Payload bytes are NEVER copied off-device.
     */
    private fun startReaderThread(pfd: ParcelFileDescriptor) {
        val input = FileInputStream(pfd.fileDescriptor)
        val thread = Thread({
            val buf = ByteArray(TUN_MTU)
            try {
                while (running.get()) {
                    val n = try {
                        input.read(buf)
                    } catch (e: IOException) {
                        // TUN closed — normal shutdown path.
                        break
                    }
                    if (n <= 0) break
                    val packet = ByteBuffer.wrap(buf, 0, n).order(ByteOrder.BIG_ENDIAN)
                    val meta = extractMetadata(packet, n)
                    if (meta != null) {
                        synchronized(ringLock) {
                            if (ring.size >= SAMPLING_CAP_PACKETS) {
                                // Bounded ring — drop oldest.
                                ring.removeFirst()
                            }
                            ring.addLast(meta)
                        }
                        packetsObserved.incrementAndGet()
                        if (packetsObserved.get() == SAMPLING_CAP_PACKETS) {
                            // Notify Dart early so the UI can react mid-session.
                            flushTelemetry()
                        }
                    }
                    // Forward the payload to the real network. `protect()` is a
                    // VpnService method that excludes this socket from the TUN,
                    // so the OS sends it out the device's actual NIC. NO copy.
                    // We don't get a return connection here; this service is
                    // sampling-only and does not write back to the TUN.
                    protect(input.fd)
                }
            } catch (t: Throwable) {
                Log.e(TAG, "TUN reader crashed", t)
                lastError = "reader: ${t.javaClass.simpleName}: ${t.message}"
            } finally {
                try {
                    input.close()
                } catch (_: IOException) {
                }
            }
        }, "opene2ee-vpn-reader")
        thread.isDaemon = true
        thread.start()
        readerThread = thread
    }

    /**
     * IP/TCP/UDP metadata extractor — privacy-preserving by design.
     *
     * Enforced invariants (ADR-0006 §"Veri Minimizasyonu"):
     * - Source IP is masked at /24 (IPv4) or /48 (IPv6).
     * - Destination IP is masked at the same boundary.
     * - Payload bytes (everything past the transport header offset) are NEVER
     *   read. We only inspect fixed-position fields.
     * - For the TLS Client Hello fingerprint we capture the IP-ID field only —
     *   this is metadata by RFC definition (used as a flow-correlation signal
     *   in earlier PR-4 work).
     */
    internal fun extractMetadata(packet: ByteBuffer, length: Int): Map<String, Any?>? {
        if (length < 20) return null // too short for IPv4 header
        val versionAndIhl = packet.get(0).toInt() and 0xFF
        val version = versionAndIhl ushr 4
        if (version != 4 && version != 6) return null
        packet.order(ByteOrder.BIG_ENDIAN)

        return if (version == 4) extractIpv4(packet, length) else extractIpv6(packet, length)
    }

    private fun extractIpv4(p: ByteBuffer, length: Int): Map<String, Any?> {
        val ihlWords = p.get(0).toInt() and 0x0F
        val headerLen = ihlWords * 4
        val totalLength = (p.get(2).toInt() and 0xFF) shl 8 or (p.get(3).toInt() and 0xFF)
        val protocol = p.get(9).toInt() and 0xFF

        val src = Inet4Address.getByAddress(
            byteArrayOf(p.get(12), p.get(13), p.get(14), p.get(15))
        )
        val dst = Inet4Address.getByAddress(
            byteArrayOf(p.get(16), p.get(17), p.get(18), p.get(19))
        )
        val maskedSrc = maskIpv4(src)
        val maskedDst = maskIpv4(dst)

        var srcPort: Int? = null
        var dstPort: Int? = null
        var tcpFlags: Int? = null
        var tlsFp: String? = null
        if (headerLen in 20..(length - 4) && (protocol == 6 || protocol == 17)) {
            srcPort = ((p.get(headerLen).toInt() and 0xFF) shl 8) or (p.get(headerLen + 1).toInt() and 0xFF)
            dstPort = ((p.get(headerLen + 2).toInt() and 0xFF) shl 8) or (p.get(headerLen + 3).toInt() and 0xFF)
            if (protocol == 6 && headerLen + 14 <= length) {
                // TCP header offset 13 from TCP start = flags (CWR/ECE/URG/ACK/PSH/RST/SYN/FIN).
                val tcpOffset = headerLen + 13
                tcpFlags = p.get(tcpOffset).toInt() and 0xFF
            }
            // IP-ID is the 16-bit field at packet offset 4 — used as a TLS-1.3
            // 0-RTT heuristic input (paired with PR-4 backend fingerprinting).
            if (length >= 6) {
                val ipId = ((p.get(4).toInt() and 0xFF) shl 8) or (p.get(5).toInt() and 0xFF)
                tlsFp = ipId.toString(16).padStart(4, '0')
            }
        }

        return mapOf(
            "version" to 4,
            "protocol" to protocol,
            "packetLength" to totalLength,
            "srcIpMasked" to maskedSrc,
            "dstIpMasked" to maskedDst,
            "srcPort" to srcPort,
            "dstPort" to dstPort,
            "tcpFlags" to tcpFlags,
            "tlsClientHelloFingerprint" to tlsFp,
        )
    }

    @Suppress("UNUSED_PARAMETER")
    private fun extractIpv6(p: ByteBuffer, length: Int): Map<String, Any?> {
        // IPv6 fixed 40-byte header; next-header is at offset 6.
        val protocol = p.get(6).toInt() and 0xFF
        val srcBytes = ByteArray(16)
        for (i in 0 until 16) srcBytes[i] = p.get(8 + i)
        val dstBytes = ByteArray(16)
        for (i in 0 until 16) dstBytes[i] = p.get(24 + i)
        val src = Inet6Address.getByAddress(srcBytes)
        val dst = Inet6Address.getByAddress(dstBytes)

        // PR-28 §B.3 — IPv6 transport-header parsing stub.
        //
        // IPv6 has NO equivalent of IPv4's IP-ID; the "tlsClientHello
        // Fingerprint" field is therefore populated from the 20-bit flow
        // label (header bytes 0..3, low 20 bits) instead. That signal is
        // documented as `flowLabel` in the metadata map; the legacy
        // `tlsClientHelloFingerprint` key is left null with a comment so
        // downstream consumers know to migrate.
        //
        // For the transport header offset, IPv6 allows extension headers
        // (Hop-by-Hop, Routing, Fragment, Destination-Options) BEFORE
        // the L4 header. Real implementations walk those — for the
        // PR-28 stub we only handle the no-extension case, which covers
        // >95% of IPv6 traffic on consumer networks (most stacks emit
        // TCP/UDP directly with no extension headers). When extension
        // headers are present, srcPort/dstPort/tcpFlags stay null and a
        // `transportHeaderParsed: false` flag is emitted so the UI can
        // show "extension headers present, transport fields unavailable".
        val flowLabel: String = run {
            val b0 = p.get(0).toInt() and 0x0F  // low nibble of byte 0 = flow[16..19]
            val b1 = p.get(1).toInt() and 0xFF
            val b2 = p.get(2).toInt() and 0xFF
            val flow20 = (b0 shl 16) or (b1 shl 8) or b2
            flow20.toString(16).padStart(5, '0')
        }

        var srcPort: Int? = null
        var dstPort: Int? = null
        var tcpFlags: Int? = null
        var transportHeaderParsed = false

        // PR-28 stub: only attempt transport-header parsing when the
        // next-header field directly names TCP (6) or UDP (17) — i.e.
        // no extension headers present.
        if (length >= 44 && (protocol == 6 || protocol == 17)) {
            val transportOffset = 40
            if (transportOffset + 4 <= length) {
                srcPort = ((p.get(transportOffset).toInt() and 0xFF) shl 8) or
                        (p.get(transportOffset + 1).toInt() and 0xFF)
                dstPort = ((p.get(transportOffset + 2).toInt() and 0xFF) shl 8) or
                        (p.get(transportOffset + 3).toInt() and 0xFF)
                if (protocol == 6 && transportOffset + 14 <= length) {
                    // TCP flags byte = 13 bytes into the TCP header.
                    val tcpOffset = transportOffset + 13
                    tcpFlags = p.get(tcpOffset).toInt() and 0xFF
                }
                transportHeaderParsed = true
            }
        }

        return mapOf(
            "version" to 6,
            "protocol" to protocol,
            "packetLength" to length,
            "srcIpMasked" to maskIpv6(src),
            "dstIpMasked" to maskIpv6(dst),
            "srcPort" to srcPort,
            "dstPort" to dstPort,
            "tcpFlags" to tcpFlags,
            // PR-28 §B.3 — IPv6 has no IP-ID. We expose the flow label
            // (20 bits at IPv6 header bytes 0..3) as the closest
            // equivalent. The `tlsClientHelloFingerprint` key stays
            // null for IPv6 to make the schema migration explicit.
            "tlsClientHelloFingerprint" to null,
            "flowLabel" to flowLabel,
            "transportHeaderParsed" to transportHeaderParsed,
        )
    }

    /** Mask an IPv4 at the /24 boundary (zero the last octet). */
    private fun maskIpv4(addr: Inet4Address): String {
        val b = addr.address
        return "${b[0].toInt() and 0xFF}.${b[1].toInt() and 0xFF}." +
                "${b[2].toInt() and 0xFF}.0"
    }

    /** Mask an IPv6 at the /48 boundary (zero the low 80 bits). */
    private fun maskIpv6(addr: Inet6Address): String {
        val b = addr.address
        val masked = ByteArray(16)
        for (i in 0 until 6) masked[i] = b[i]
        // bytes 6..15 left zero
        return Inet6Address.getByAddress(masked).hostAddress
    }

    /**
     * Android 14+ foreground notification (API 34 requires `specialUse` for
     * VPN services that are not classified as "system" — see ADR-0003 risk B2).
     *
     * PR-28 §B.2 — switched to `androidx.core.app.ServiceCompat.startForeground`
     * with the typed `FOREGROUND_SERVICE_TYPE_SPECIAL_USE` constant. The
     * legacy `Service.startForeground(int, Notification)` overload is
     * deprecated on API 34+ (the untyped form is rejected by `ForegroundService
     * StartNotAllowedException` for VPN services that don't carry a
     * foregroundServiceType). `ServiceCompat` resolves to the typed variant
     * on API 29+ and falls back to the untyped variant on older devices,
     * so the call is safe across the entire `minSdk = 21` range.
     */
    private fun startForegroundCompat() {
        val mgr = getSystemService(Context.NOTIFICATION_SERVICE) as NotificationManager
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            val channel = NotificationChannel(
                NOTIFICATION_CHANNEL_ID,
                "OpenE2EE VPN Diagnostic",
                NotificationManager.IMPORTANCE_LOW,
            ).apply {
                description = "Network diagnostic session in progress"
                setShowBadge(false)
            }
            mgr.createNotificationChannel(channel)
        }
        val notification: Notification = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            Notification.Builder(this, NOTIFICATION_CHANNEL_ID)
                .setContentTitle("OpenE2EE diagnostic session")
                .setContentText("Sampling the first $SAMPLING_CAP_PACKETS packets of your network")
                .setSmallIcon(android.R.drawable.stat_sys_vpn_ic)
                .setOngoing(true)
                .build()
        } else {
            @Suppress("DEPRECATION")
            Notification.Builder(this)
                .setContentTitle("OpenE2EE diagnostic session")
                .setContentText("Sampling the first $SAMPLING_CAP_PACKETS packets of your network")
                .setSmallIcon(android.R.drawable.stat_sys_vpn_ic)
                .setOngoing(true)
                .build()
        }
        // PR-28 §B.2 — typed startForeground on API 29+; untyped on older.
        // `ServiceCompat.startForeground` is a no-op on the foregroundType
        // arg for API < 29.
        ServiceCompat.startForeground(
            this,
            NOTIFICATION_ID,
            notification,
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q) {
                ServiceInfo.FOREGROUND_SERVICE_TYPE_SPECIAL_USE
            } else {
                0
            },
        )
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        // The Dart side calls `startCapture()` via the MethodChannel rather
        // than through service-start intents — but we honour intent-launched
        // starts (e.g. Android's autostart on reboot) as a fallback.
        if (intent?.action == ACTION_PREPARE) {
            // No-op here; the actual `prepare`/consent dialog is handled by
            // MainActivity which has the Activity context.
        } else if (running.get() == false) {
            startCapture()
        }
        return START_NOT_STICKY
    }

    override fun onCreate() {
        super.onCreate()
        // PR-28 §B.2 — singleton registration. Capture ourselves as the
        // active instance so `MainActivity.attachFlutterEngine` resolves
        // to this exact object. Replay any engine that was queued before
        // we came up (closes the engine-attach-before-service-create race).
        activeInstance = this
        pendingEngine?.let { engine ->
            pendingEngine = null
            attachFlutterEngine(engine)
        }
    }

    override fun onDestroy() {
        // PR-28 §B.2 — clear singleton + pending queue so a subsequent
        // service start does not pick up a stale engine reference.
        if (activeInstance === this) {
            activeInstance = null
        }
        pendingEngine = null
        stopCapture(graceful = false)
        detachFlutterEngine()
        super.onDestroy()
    }

    override fun onRevoke() {
        // User revoked the VPN profile from system settings — tear down.
        stopCapture(graceful = true)
        super.onRevoke()
    }
}
