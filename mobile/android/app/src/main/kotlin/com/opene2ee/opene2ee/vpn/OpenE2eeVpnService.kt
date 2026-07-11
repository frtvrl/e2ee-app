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
// Sprint 11.0A — REAL packet drain → MethodChannel push bridge.
//                The 10.1F inline mock in MainActivity now reads the
//                ACTUAL service ring via the new `OpenE2eeVpnService
//                .snapshot()` static accessor (S46). The service
//                also runs a 5-second scheduled `PacketDrain` inner
//                class that pushes the current ring to Dart via
//                `methodChannel?.invokeMethod("onPacketsSampled",
//                packetsArray)` (S45). A new `companion object`
//                `methodChannel: MethodChannel?` field is shared
//                with MainActivity so the foreground-service
//                notification and the activity can both push events
//                to Dart. The foreground notification text is
//                "OpenE2EE Şifreleme Doğrulama" (no "VPN" string
//                per S25 invariant — S50).
//
// Sprint 11.0D — channel ownership moved BACK to MainActivity.
//                In 11.0A the `opene2ee/vpn` MethodChannel handler
//                was installed by `attachFlutterEngine` in this
//                service — but `attachFlutterEngine` only runs
//                AFTER the service is created (via `onCreate`),
//                and the service is only created on Dart's `start`
//                call. The Dart-side `pool_provider.dart` polling
//                loop calls `vpn.getSampledPackets()` every 5s
//                starting the moment the ActivePoolScreen is
//                first opened — BEFORE `start`. Result: OnePlus 9
//                Pro owner reported `MissingPluginException(No
//                implementation found for method getSampledPackets
//                on channel opene2ee/vpn)`. The fix: handler lives
//                at the activity level (always alive from app
//                launch), delegates to a new
//                `OpenE2eeVpnService.dispatch(context, call, result)`
//                static which routes per-method to the live
//                service OR returns safe defaults (empty ring /
//                IDLE status) when no service is alive yet. The
//                instance `attachFlutterEngine` is preserved (it
//                now sets the companion `methodChannel` for
//                outbound `onPacketsSampled` pushes only — no
//                inbound `setMethodCallHandler`). See S73
//                invariant: `MainActivity.kt` owns the
//                `opene2ee/vpn` MethodChannel handler.
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
//       "getSampledPackets" → snapshot of the bounded ring
//                              (Sprint 11.0A; replaces the 10.1F
//                              mock packet). Safe to call BEFORE
//                              the service is running — dispatch
//                              returns an empty list.
//       "setAllowedApplications" → restrict VPN to a per-app allowlist
//                                   (Android 5.0+, VpnService.Builder.allowedApplications)
//       "setDisallowedApplications" → inverse — bypass VPN for these apps
//       "requestPrepare" → emit a SystemIntent-style permission prompt
//                           (handled by MainActivity; this service exposes
//                            the helper that returns the intent action)
// - Channel methods TO Dart:
//       "onPacketsSampled" → 5-second scheduled push of the bounded
//                              ring (S45 invariant; Sprint 11.0A)
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
import android.net.ConnectivityManager
import android.net.LinkProperties
import android.net.Network
import android.net.NetworkCapabilities
import android.net.NetworkRequest
import android.net.VpnService
import android.os.Build
import android.os.Handler
import android.os.Looper
import android.os.ParcelFileDescriptor
import android.util.Log
import androidx.annotation.RequiresApi
import androidx.core.app.NotificationCompat
import androidx.core.app.ServiceCompat
import androidx.core.content.ContextCompat
import io.flutter.embedding.engine.FlutterEngine
import io.flutter.plugin.common.MethodCall
import io.flutter.plugin.common.MethodChannel
import java.io.FileInputStream
import java.io.IOException
import java.net.Inet4Address
import java.net.Inet6Address
import java.net.InetAddress
import java.net.Socket
import java.nio.ByteBuffer
import java.nio.ByteOrder
import java.util.concurrent.Executors
import java.util.concurrent.ScheduledExecutorService
import java.util.concurrent.ScheduledFuture
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicBoolean
import java.util.concurrent.atomic.AtomicInteger
import java.util.concurrent.atomic.AtomicLong

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

        /**
         * Sprint 11.0H — TOCTOU (Time-Of-Check-Time-Of-Use) guard
         * for [startCapture] and [stopCapture]. Without this lock,
         * a double-tap on the "Aktif Nöbet başlat" button (or a
         * system-side `onStartCommand` self-restart) can race:
         * the FIRST `startCapture` is mid-flight (TUN setup),
         * `running.set(true)` not yet called, when the SECOND
         * `startCapture` enters, sees `running.get() == false`,
         * and starts a SECOND TUN setup. The two TUNs collide,
         * the reader thread from the first captures EOF from
         * the second's `pfd.close()`, and the service lands in
         * a corrupt state — symptom Owner saw: `start` returns
         * `state: DRAINING, packetsObserved: 0, ringSize: 0`
         * with no `lastError` (the catch block wasn't hit, the
         * stop path WAS hit by the racing stop).
         *
         * The lock serializes `startCapture` / `stopCapture` /
         * `onRevoke` so the TOCTOU window is closed. The lock
         * is a companion-level `@JvmStatic` `Object` so it is
         * shared across all instances (companion fields are
         * shared JVM-wide). It is intentionally NOT the
         * per-instance `lock` — that would defeat the purpose.
         */
        @JvmField
        val stateLock: Any = Any()

        /**
         * Sprint 11.0K — main looper Handler. The Flutter
         * Engine requires `MethodChannel.invokeMethod` to be
         * invoked on the Android UI thread (the main
         * `Looper`). Pre-11.0K, all three call sites
         * (flushTelemetry's `onTelemetry`, notifyError's
         * `onError`, PacketDrain's `onPacketsSampled`) called
         * `methodChannel?.invokeMethod` from background threads
         * (PacketDrain = `opene2ee-vpn-drain` ScheduledExecutor
         * worker; flushTelemetry / notifyError = the TUN reader
         * thread). The Engine threw `@UiThread` violations and
         * the Owner saw "VPN active, internet OK, UI never
         * updates" (state: DRAINING, packetsObserved: 0,
         * ringSize: 0). 11.0K wraps all three call sites in
         * `mainHandler.post { ... }` via the `pushToDart` helper.
         *
         * The Handler is `@JvmField` so the post is one line at
         * the call site (no allocation per push). It is created
         * EAGERLY at class-load time so the first push doesn't
         * have to wait for `Looper.getMainLooper()` to be
         * queried from a non-main thread (subtle but documented
         * in `Handler` Javadoc — querying the main looper from a
         * worker thread is fine but the Handler itself should be
         * constructed on a thread that has the main Looper's
         * classloader available; lazy init in a worker thread
         * can hit a `NullPointerException` on some Android
         * OEM ROMs — OnePlus OxygenOS is one of them).
         */
        @JvmField
        val mainHandler: Handler = Handler(Looper.getMainLooper())

        /** Must match `kVpnMethodChannel` in Dart. */
        const val METHOD_CHANNEL = "opene2ee/vpn"

        /** Sampling cap per HANDOFF §6.1 mobile spec. */
        const val SAMPLING_CAP_PACKETS = 10

        /**
         * Sprint 11.0S-EXTRA — 15-minute countdown
         * duration. The foreground notification
         * chronometer counts down from this value to
         * zero, then the auto-stop Handler fires
         * `stopCapture(graceful = true)` to tear
         * down the VPN. S92 audit verifies
         * `setWhen(now + COUNTDOWN_TOTAL_MS)` +
         * the auto-stop Handler is present.
         */
        const val COUNTDOWN_TOTAL_MS = 15L * 60L * 1000L

        /**
         * Sprint 11.0P — `TUN_MTU` lowered from 1500 to
         * 1400. The standard Ethernet MTU (1500) is too
         * large for mobile networks: Turkcell 4G/5G uses
         * GTP-U encapsulation (8-byte header + IPsec
         * 50-70-byte trailer) which means a 1500-byte TUN
         * packet becomes 1500 + 78 = 1578 bytes on the
         * wire. The mobile network drops any frame
         * >1500 bytes (the radio link MTU), so packets
         * exit the TUN, hit the radio link, and are
         * dropped silently. The Owner sees Chrome /
         * WhatsApp "no internet" even though the TUN
         * reader is capturing packets (1247 packets/2min
         * logcat in Sprint 11.0O confirmed passthrough
         * is real; the missing 30% of large packets that
         * were dropped on the radio link is what the
         * user experiences as "DNS / load failures").
         *
         * 1400 bytes is the canonical mobile-safe MTU
         * (1400 + 78 = 1478 < 1500 radio MTU). The
         * S87 audit verifies `TUN_MTU = 1400` (NOT
         * 1500) is present in `OpenE2eeVpnService.kt`
         * as a regression guard.
         */
        const val TUN_MTU = 1400

        /** Public, no-leak DNS resolvers handed to the TUN. */
        val PRIMARY_DNS: InetAddress = InetAddress.getByName("1.1.1.1")
        val SECONDARY_DNS: InetAddress = InetAddress.getByName("1.0.0.1")

        /** TUN address space (RFC 1918). */
        val TUN_ADDRESS: InetAddress = InetAddress.getByName("10.42.0.2")
        val TUN_ROUTED_CIDR: String = "10.42.0.0/24"

        /**
         * Sprint 11.0I — TUN interface address prefix length
         * (`/24`). Used by [buildVpnBuilder] for the
         * `addAddress(TUN_ADDRESS, TUN_PREFIX_LENGTH)` call
         * (the interface address) AND logged in the
         * startCapture breadcrumb.
         */
        const val TUN_PREFIX_LENGTH = 24

        /**
         * Sprint 11.0I — captured-route destination address
         * (`"0.0.0.0"` = default route = ALL traffic). Used
         * by [buildVpnBuilder] for the
         * `addRoute(CAPTURED_ROUTE_ADDRESS, CAPTURED_ROUTE_PREFIX)`
         * call.
         *
         * Why `0.0.0.0/0` and NOT `TUN_ADDRESS/24`:
         * Pre-11.0I, the code used
         * `.addRoute(TUN_ADDRESS, 24)` which is the SAME
         * address as the interface (`addAddress(TUN_ADDRESS, 24)`).
         * The Android `VpnService.Builder.addRoute` method
         * expects a DESTINATION SUBNET (the network whose
         * traffic the VPN will capture), NOT the interface
         * address. Using the interface address is a
         * 9.7.0-era mirror bug that the OnePlus 9 Pro
         * OxygenOS strict validation rejects with
         * `IllegalArgumentException: Bad address` (Owner
         * 11:46-11:57 logcat confirmed the regression).
         * Pixel/Samsung tolerate the bug; OnePlus does not.
         *
         * `0.0.0.0/0` (default route) is the safest
         * fallback — it captures ALL traffic, works on every
         * Android version (API 21+), and avoids the
         * OnePlus strict validation.
         */
        const val CAPTURED_ROUTE_ADDRESS = "0.0.0.0"
        const val CAPTURED_ROUTE_PREFIX = 0

        /** Notification channel ID (Android 8+ requirement). */
        const val NOTIFICATION_CHANNEL_ID = "opene2ee.vpn.diagnostic"

        /** Foreground notification id. */
        const val NOTIFICATION_ID = 0x4F_50_4E_45 /* 'OPNE' */

        /** Intent action that calls `VpnService.prepare()` from MainActivity. */
        const val ACTION_PREPARE = "com.opene2ee.opene2ee.vpn.PREPARE"

        // ═══ PR-28 §B.2 — Transient service instance handling ══════════
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
        // PR-28 §B.2 — singleton dispatch. NOT marked `@JvmStatic` because
        // the companion form shares its JVM name+descriptor with the
        // instance [attachFlutterEngine] below, which Kotlin rejects as a
        // "Platform declaration clash". Kotlin callers reach the companion
        // via the standard `OpenE2eeVpnService.attachFlutterEngine(...)`
        // sugar; Java callers go through `Companion.attachFlutterEngine`.
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
        // Companion counterpart of [detachFlutterEngine] below. Same
        // @JvmStatic rationale as above — kept off to avoid the JVM
        // signature clash with the instance method of the same name.
        fun detachFlutterEngine() {
            val instance = activeInstance
            if (instance != null) {
                instance.detachFlutterEngine()
            } else {
                pendingEngine = null
            }
        }

        // ═══ Sprint 11.0A — REAL packet drain → MethodChannel push ═══
        //
        // The 10.1F inline mock in MainActivity read a hard-coded
        // synthetic packet; 10.2 was supposed to integrate the real
        // service. Sprint 11.0A (M1) closes the gap: the service
        // exposes its ring to MainActivity via the [snapshot] static
        // accessor, AND runs a 5-second scheduled `PacketDrain` inner
        // class that pushes the current ring to Dart via
        // `methodChannel?.invokeMethod("onPacketsSampled", packetsArray)`.
        //
        // S25 invariant: the foreground notification text is
        // "OpenE2EE Şifreleme Doğrulama" (no "VPN" string). See
        // [startForegroundCompat] for the title.

        /** Drain cadence (seconds) for the 5-second push loop. */
        const val DRAIN_INTERVAL_SECONDS: Long = 5

        /**
         * Shared MethodChannel reference. The companion owns this so
         * the foreground service (which never sees the Flutter engine
         * directly) can still push `onPacketsSampled` events to Dart.
         * The [attachFlutterEngine] setter wires the channel from the
         * engine; [detachFlutterEngine] clears it. Read by
         * [snapshot] when MainActivity polls and by [PacketDrain] when
         * the scheduled timer fires.
         */
        @JvmStatic
        @Volatile
        var methodChannel: MethodChannel? = null

        /**
         * Snapshot of the active service's bounded ring. Returns
         * `null` if no service is alive (MainActivity surfaces an
         * empty list to Dart in that case). The returned list is a
         * copy; the service may continue mutating the ring after the
         * call returns.
         *
         * Replaces the Sprint 10.1F inline mock packet in
         * `MainActivity.onVpnCall("getSampledPackets", ...)`. The
         * Dart side contract (`ParsedPacket.toJson()` shape) is
         * preserved verbatim.
         */
        @JvmStatic
        fun snapshot(): List<Map<String, Any?>>? {
            val instance = activeInstance ?: return null
            return instance.snapshotRing()
        }

        /**
         * Sprint 11.0D — single entry point for ALL Dart → service
         * MethodChannel calls. MainActivity's `opene2ee/vpn`
         * MethodChannel handler is a thin wrapper that calls this
         * method:
         *
         *   vpnChannel.setMethodCallHandler { call, result ->
         *       OpenE2eeVpnService.dispatch(this, call, result)
         *   }
         *
         * This is the fix for the Sprint 11.0A regression where the
         * channel handler lived inside the service: Dart polled
         * `getSampledPackets` before the service was started (i.e.
         * before the user clicked "Şifreleme Doğrulamayı Başlat"),
         * the handler was never installed, and Dart got
         * `MissingPluginException`. By centralising the dispatch in
         * a static (which never depends on a live service instance),
         * the inbound side is ALWAYS reachable from app launch.
         *
         * Behaviour matrix (see [onMethodCall] for the per-method
         * contract preserved from 11.0A):
         *   - `getSampledPackets`: returns activeInstance ring OR
         *     empty list if no service yet. SAFE to call before the
         *     service starts.
         *   - `status`: returns activeInstance status OR the
         *     canonical IDLE status map. SAFE pre-service.
         *   - `start`: uses [context] to launch the foreground
         *     service via [ContextCompat.startForegroundService],
         *     then delegates to the running instance's
         *     [onMethodCall] for the response. This is the missing
         *     piece in Sprint 11.0A — the service was never started
         *     because no one called `startForegroundService` for it.
         *   - `stop`: delegates to active instance, or returns
         *     STOPPED if no service.
         *   - `setAllowedApplications` /
         *     `setDisallowedApplications`: stores on active instance
         *     (or no-ops if no service yet).
         *   - `requestPrepare`: returns the consent Intent action
         *     (the actual consent dialog is owned by
         *     `opene2ee/vpn_permissions` — MainActivity handles it).
         *   - else: `notImplemented`.
         */
        @JvmStatic
        fun dispatch(context: Context, call: MethodCall, result: MethodChannel.Result) {
            // Sprint 11.0F — diagnostic breadcrumb at the dispatcher
            // entry. Pairs with the per-service-instance breadcrumbs
            // (`onStartCommand: entry`, `startCapture: entry`) so the
            // Owner can pinpoint where the regression is hanging on
            // the OnePlus 9 Pro Magisk Zygisk flow.
            Log.d(TAG, "dispatch: entry (method=${call.method})")
            try {
                when (call.method) {
                    "start" -> {
                        // Sprint 11.0D — actually start the foreground
                        // service. The intent has NO action so the
                        // service's `onStartCommand` falls into the
                        // `else if (running.get() == false) startCapture()`
                        // branch and brings up the TUN. We use
                        // [ContextCompat.startForegroundService] (the
                        // API 26+ path; the compat shim handles the
                        // pre-O fallback to plain `startService`).
                        val intent = Intent(context, OpenE2eeVpnService::class.java)
                        ContextCompat.startForegroundService(context, intent)
                        Log.d(TAG, "dispatch: startForegroundService() invoked, awaiting onStartCommand")
                        // Delegate to the active instance if it
                        // already exists (idempotent), else return
                        // a pending `preparing` status so Dart's
                        // state stream flips.
                        val instance = activeInstance
                        if (instance != null) {
                            Log.d(TAG, "dispatch: activeInstance present, delegating onMethodCall")
                            instance.onMethodCall(call, result)
                        } else {
                            // Service is spinning up — the system
                            // will call `onCreate` → register as
                            // activeInstance → `onStartCommand` →
                            // `startCapture` shortly. Tell Dart to
                            // poll `status` again to observe the
                            // transition.
                            Log.d(TAG, "dispatch: no activeInstance yet, returning DRAINING (service spinning up)")
                            result.success(idleStatusMap().toMutableMap().apply {
                                this["state"] = "DRAINING" // service is bringing up TUN
                            })
                        }
                    }
                    "stop" -> {
                        val instance = activeInstance
                        if (instance != null) {
                            instance.onMethodCall(call, result)
                        } else {
                            result.success(idleStatusMap().toMutableMap().apply {
                                this["state"] = "STOPPED"
                            })
                        }
                    }
                    "status" -> {
                        val instance = activeInstance
                        if (instance != null) {
                            instance.onMethodCall(call, result)
                        } else {
                            result.success(idleStatusMap())
                        }
                    }
                    "getSampledPackets" -> {
                        // SAFE to call before the service is alive:
                        // an empty list is the correct "no samples
                        // yet" answer. This is the primary call that
                        // was throwing `MissingPluginException` in
                        // Sprint 11.0A.
                        val r = snapshot() ?: emptyList()
                        // Sprint 11.0F — diagnostic breadcrumb. Logs
                        // the size of the returned ring so the Owner
                        // can see whether the polling loop is reaching
                        // the channel and what state the sampling is
                        // in (empty vs non-empty).
                        Log.d(TAG, "dispatch: getSampledPackets returned ${r.size} packets (activeInstance=${activeInstance != null})")
                        result.success(r)
                    }
                    "setAllowedApplications" -> {
                        val instance = activeInstance
                        if (instance != null) {
                            instance.onMethodCall(call, result)
                        } else {
                            // Pre-service set: stash on a
                            // companion-side pending list so the
                            // service picks it up on `onCreate`. For
                            // Sprint 11.0D we keep the simpler
                            // behaviour: drop with a warning result.
                            result.success(false)
                        }
                    }
                    "setDisallowedApplications" -> {
                        val instance = activeInstance
                        if (instance != null) {
                            instance.onMethodCall(call, result)
                        } else {
                            result.success(false)
                        }
                    }
                    "requestPrepare" -> {
                        // The actual consent flow lives on the
                        // `opene2ee/vpn_permissions` channel; this
                        // is kept for the 10.1F Dart-side contract
                        // (which calls `requestPrepare` first, then
                        // the permissions channel). The intent
                        // action is what `VpnService.prepare()`
                        // returns in the standard Android flow.
                        result.success("android.net.VpnService")
                    }
                    else -> result.notImplemented()
                }
            } catch (t: Throwable) {
                Log.e(TAG, "dispatch error: ${call.method}", t)
                result.error("vpn_method_error", t.message, null)
            }
        }

        /**
         * Canonical IDLE status payload returned by [dispatch] when
         * the service has not been started yet. Matches the shape of
         * [currentStatusMap] so Dart's `VpnService._stateFromMap`
         * receives the expected keys (state / packetsObserved /
         * ringSize / samplingCap / lastError / allowedApplications /
         * disallowedApplications).
         */
        @JvmStatic
        fun idleStatusMap(): Map<String, Any?> = mapOf(
            "state" to "IDLE",
            "packetsObserved" to 0,
            "ringSize" to 0,
            "samplingCap" to SAMPLING_CAP_PACKETS,
            "lastError" to null,
            "allowedApplications" to null,
            "disallowedApplications" to null,
        )

        /**
         * Sprint 11.0E — idempotent notification-channel creator.
         * Called from [ensureForegroundService] (and the legacy
         * [startForegroundCompat] for back-compat) before
         * `startForeground()`. Android 8+ (API 26+) REQUIRES a
         * channel to exist for `NotificationCompat.Builder.build()`
         * to succeed when the notification is tied to a foreground
         * service; missing the channel is the "silent no-op →
         * 5-second timeout" failure mode in Sprint 11.0E's Senaryo 2.
         *
         * Idempotent: re-creating an existing channel is a no-op on
         * the platform side, but we add the `getNotificationChannel
         * (CHANNEL_ID) == null` short-circuit so the call has no
         * observable cost on the hot path.
         */
        @JvmStatic
        fun ensureNotificationChannel(context: Context) {
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
                val nm = context.getSystemService(NotificationManager::class.java) ?: return
                if (nm.getNotificationChannel(NOTIFICATION_CHANNEL_ID) != null) {
                    return // already exists — no-op
                }
                val channel = NotificationChannel(
                    NOTIFICATION_CHANNEL_ID,
                    "OpenE2EE Şifreleme Doğrulama",
                    NotificationManager.IMPORTANCE_LOW,
                ).apply {
                    // Sprint 11.0A — S50 invariant: NO "VPN" string in
                    // any user-facing surface. The PRIVACY_TEXT eki is
                    // the Turkish-language disclosure appended for the
                    // Android 14+ foregroundServiceType=specialUse
                    // subtype justification.
                    description = "Ağ şifreleme bütünlüğü doğrulama oturumu (PRIVACY_TEXT eki)"
                    setShowBadge(false)
                    enableVibration(false)
                    setSound(null, null)
                }
                nm.createNotificationChannel(channel)
            }
        }
    }

    /** Lifecycle states exposed via `status` to Dart. */
    enum class State { IDLE, SAMPLING, DRAINING, STOPPED, ERROR }

    /** Monotonic packet counter since last `start`. */
    private val packetsObserved = AtomicInteger(0)

    /**
     * Sprint 11.0P — IP fragment counter. Increments when
     * the read packet's IP header has the
     * `MF` (More Fragments) flag set or the fragment
     * offset is non-zero. The fragment rate on a healthy
     * mobile network is < 0.1%; a rate > 5% indicates
     * MTU 1500 is too high (Owner 13:50: Turkcell GTP
     * encapsulation drops fragments > 1500 bytes on the
     * radio link). The 11.0P MTU = 1400 fix should
     * drive the fragment rate back to baseline. The
     * `startReaderThread` per-1000-packet breadcrumb
     * surfaces `fragmentCount` for the Owner to grep
     * `adb logcat` for after a Chrome / WhatsApp test.
     */
    private val ipFragmentCount = AtomicLong(0)

    /**
     * Sprint 11.0T — passthrough write counter. Owner
     * 18:19 reported that `curl 212.64.210.85/healthz`
     * works WITHOUT the VPN (the upstream Patroni
     * answers) but FAILS with the VPN (the user sees
     * "no route to host" / timeout). Sprint 11.0J
     * added the transparent passthrough (`output
     .write(buf, 0, n)`) but Owner 18:19 confirmed
     * it doesn't actually write the bytes. This
     * counter increments EXACTLY ONCE per
     * successful `output.write(buf, 0, n)` and is
     * the canonical diagnostic for the Owner to
     * grep `adb logcat` after a `curl 212.64.210.85/
     * healthz` test:
     *   - If `passthroughCount` is 0 → write is
     *     never called (or never succeeds). The
     *     reader thread is in an error state
     *     BEFORE the write.
     *   - If `passthroughCount` > 0 but `curl`
     *     still fails → the write IS happening
     *     but the bytes are not reaching the
     *     kernel (the OS drops them, the TUN fd
     *     is closed, Magisk Zygisk interferes, etc.).
     *   - If `passthroughCount` equals
     *     `packetsObserved` (the per-1000 log
     *     breadcrumb compares both) → every
     *     captured packet is also being
     *     passthrough-written (the healthy state).
     * S93 audit verifies this field is declared
     * AND is reset in startCapture AND is
     * incremented in the write call block.
     */
    private val passthroughCount = AtomicLong(0)

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

    /**
     * Sprint 11.0S-EXTRA — the pending 15-minute
     * auto-stop Handler. Set by
     * `scheduleCountdownAutoStop()` after the
     * foreground notification is posted, cancelled
     * in `stopCapture()` (when the user manually
     * disconnects, so the VPN tears down
     * immediately and the 00:00 wakeup doesn't
     * fire later). The chronometer + Handler pair
     * is the "alarm clock" the user sees in the
     * notification bar.
     */
    private var countdownAutoStopRunnable: Runnable? = null

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

    // Sprint 11.0A — scheduled packet drain (5s cadence). The
    // executor is shared across the service lifetime and shut
    // down in [stopCapture]. A single-threaded scheduled pool is
    // sufficient (one task at a time, ring reads are O(1)).
    private var drainExecutor: ScheduledExecutorService? = null
    private var drainTask: ScheduledFuture<*>? = null

    /**
     * Sprint 11.0D — `attachFlutterEngine` is now a NO-OP for
     * INBOUND channel registration. The `opene2ee/vpn`
     * MethodChannel handler is owned by `MainActivity`
     * (registered in its `configureFlutterEngine` override,
     * which runs at app launch — BEFORE the VpnService is ever
     * started).
     *
     * Why this changed: in Sprint 11.0A, the handler was set
     * here, BUT the Dart-side `pool_provider.dart` polling loop
     * calls `vpn.getSampledPackets()` immediately when the
     * ActivePoolScreen is first opened (the PoolNotifier is
     * constructed lazily when `ref.watch(poolProvider)` is first
     * read). At that moment the VpnService is NOT yet running
     * (the user has not clicked "Şifreleme Doğrulamayı Başlat"),
     * so the handler was never set, and Dart got
     * `MissingPluginException(No implementation found for
     * method getSampledPackets on channel opene2ee/vpn)`.
     *
     * The MainActivity-owned handler delegates to the
     * [Companion.dispatch] static dispatcher which returns
     * safe defaults when no service is active, OR calls the
     * service's instance methods when the service IS active.
     *
     * The OUTBOUND side of the channel (the `onPacketsSampled`
     * event the 5-second `PacketDrain` pushes to Dart) still
     * needs a [MethodChannel] reference, which is the
     * [Companion.methodChannel] field. We publish the channel
     * from the engine here so the drain can read it without
     * holding an engine reference.
     */
    fun attachFlutterEngine(engine: FlutterEngine) {
        val ch = MethodChannel(engine.dartExecutor.binaryMessenger, METHOD_CHANNEL)
        // Publish the channel for OUTBOUND pushes from
        // `PacketDrain` (the 5-second `onPacketsSampled` event).
        // Do NOT install an inbound handler here — MainActivity
        // owns that side (see class doc + S73 invariant).
        Companion.methodChannel = ch
    }

    /**
     * Detach the MethodChannel — called from `MainActivity` `onDestroy` so
     * we don't leak handlers across engine restarts.
     */
    fun detachFlutterEngine() {
        methodChannel?.setMethodCallHandler(null)
        methodChannel = null
        // Sprint 11.0A — clear the companion reference too so the
        // drain loop (if still scheduled) does not push to a
        // stale channel after the activity is gone.
        if (Companion.methodChannel === methodChannel) {
            Companion.methodChannel = null
        }
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
                "getSampledPackets" -> {
                    // Sprint 11.0A — read the LIVE bounded ring and
                    // return the snapshot. Replaces the 10.1F inline
                    // mock packet that lived in MainActivity. The
                    // ring is bounded by [SAMPLING_CAP_PACKETS] (10)
                    // so a slow consumer does not leak memory; the
                    // companion `snapshot()` static (S46) and the
                    // live `packetStream` push (S45) are the two
                    // consumer paths.
                    val packets: List<Map<String, Any?>> = snapshotRing()
                    result.success(packets)
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
                    // TODO(port-main-activity): MainActivity port (parallel sprint item) will own the actual startActivityForResult flow that consumes this ACTION.
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
            // Sprint 11.0I — `addAddress(TUN_ADDRESS, 24)` is the
            // TUN INTERFACE address (the IP the VPN endpoint
            // will hold on the device). The `/24` prefix length
            // is the SUBNET size of that interface.
            .addAddress(TUN_ADDRESS, TUN_PREFIX_LENGTH)
            // Sprint 11.0I — `addRoute` takes a DESTINATION
            // SUBNET, NOT the interface address. Pre-11.0I the
            // code used `.addRoute(TUN_ADDRESS, 24)` (the SAME
            // IP as the interface) which is the 9.7.0 mirror
            // bug. OnePlus 9 Pro OxygenOS strict validation
            // rejects it with `IllegalArgumentException: Bad
            // address` (Owner 11:46-11:57 logcat confirmed
            // the regression). The fix is `addRoute("0.0.0.0",
            // 0)` — default route = ALL traffic captured.
            // S79 audit invariant: this line MUST NOT regress
            // to `addRoute(TUN_ADDRESS, ...)`.
            .addRoute(CAPTURED_ROUTE_ADDRESS, CAPTURED_ROUTE_PREFIX)
            .addDnsServer(PRIMARY_DNS)
            .addDnsServer(SECONDARY_DNS)
            .setMtu(TUN_MTU)
            .setBlocking(true)
        // PR-28 §B.1: per-app VPN allow/deny via [VpnService.Builder.addAllowedApplication]
        // / [addDisallowedApplication] are API 21+ (Lollipop). The Builder API exposes
        // these as SINGULAR per-package calls — there is no list-form overload in the
        // Android SDK (the original PR-28 source used `b.allowedApplications(pkgs)`,
        // which does not resolve on any API level). We therefore loop the per-app list
        // here. The project floor minSdk = 23 (Sprint 7 MOB-5) makes the @RequiresApi(21)
        // guard above redundant; [buildVpnBuilder] carries the lint annotation.
        allowedApplications?.forEach { pkg ->
            try {
                b.addAllowedApplication(pkg)
            } catch (e: android.content.pm.PackageManager.NameNotFoundException) {
                // Unknown package — skip silently. The Dart-side package name list is
                // user-supplied; we don't want one stale entry to break Builder.establish.
                Log.w(TAG, "allowedApplications: package not found, skipping: $pkg")
            }
        }
        disallowedApplications?.forEach { pkg ->
            try {
                b.addDisallowedApplication(pkg)
            } catch (e: android.content.pm.PackageManager.NameNotFoundException) {
                Log.w(TAG, "disallowedApplications: package not found, skipping: $pkg")
            }
        }
        return b
    }

    /**
     * Bring up the TUN, start the foreground notification, spawn the reader
     * thread. Idempotent — a duplicate call while already running is a no-op.
     */
    private fun startCapture(): State {
        // Sprint 11.0H — TOCTOU guard. The `synchronized(stateLock)`
        // block serializes `startCapture` / `stopCapture` /
        // `onRevoke` so a double-tap on the "Aktif Nöbet başlat"
        // button (or a system-side `onStartCommand` self-restart)
        // cannot race. Pre-11.0H the check
        // `if (running.get()) return state` was non-atomic
        // w.r.t. the rest of the function — a second invocation
        // could enter mid-flight. The lock closes that window.
        return synchronized(stateLock) {
            val prevState = state
            if (running.get()) {
                Log.d(TAG, "startCapture: TOCTOU guard hit, already running (state=$state, returning $state)")
                return@synchronized state
            }
            // Sprint 11.0F — diagnostic breadcrumbs. Each `Log.d` line is
            // emitted BEFORE the named side-effect so the Owner (or
            // anyone running `adb logcat -d -s OpenE2eeVpn:V`) can
            // pinpoint which step regressed. The `S75` audit invariant
            // asserts at least 5 of these are present in the source.
            // Sprint 11.0H — `prevState` is logged at entry so the
            // state-transition breadcrumbs (S78) are explicit
            // about the BEFORE / AFTER delta.
            Log.d(TAG, "startCapture: entry (running=false, prevState=$prevState)")
            try {
                val builder = buildVpnBuilder()
                // Sprint 11.0I — extended breadcrumb so the
                // addAddress / addRoute / MTU / DNS parameters
                // are visible in `adb logcat -d -s OpenE2eeVpn:V`.
                // Pre-11.0I the breadcrumb only logged the
                // interface address, which made the OnePlus
                // `Bad address` regression hard to diagnose
                // (the actual culprit was the addRoute
                // destination subnet).
                Log.d(TAG, "startCapture: buildVpnBuilder returned " +
                        "(addAddress=${TUN_ADDRESS.hostAddress}/$TUN_PREFIX_LENGTH, " +
                        "addRoute=$CAPTURED_ROUTE_ADDRESS/$CAPTURED_ROUTE_PREFIX, " +
                        "mtu=$TUN_MTU, dns=${PRIMARY_DNS.hostAddress}/${SECONDARY_DNS.hostAddress})")
                val pfd = builder.establish()
                if (pfd == null) {
                    // Sprint 11.0F — make the error message actionable.
                    // On OnePlus 9 Pro (rootlu, Magisk Zygisk) the
                    // `VpnService.Builder.establish()` call returns
                    // null even though the user already granted consent
                    // via `VpnService.prepare(this)` (because the
                    // foreground-service consent was confirmed in a
                    // PRIOR process / boot — `prepare()` returns null
                    // in that case too). The most common cause on a
                    // rooted OnePlus is Magisk's Zygisk module
                    // intercepting VpnService.establish() as part of
                    // its root-hide trick. The actionable advice: open
                    // Magisk → Settings → Zygisk → Disable, then
                    // reboot. Without this hint, the user sees a
                    // generic error and the regression looks
                    // unresolvable.
                    state = State.ERROR
                    lastError = "VpnService.Builder.establish() returned null " +
                            "(user declined consent, system refused, OR " +
                            "OnePlus Magisk Zygisk is intercepting). " +
                            "Workaround: Magisk → Settings → Zygisk → Disable, " +
                            "reboot, reinstall APK. See sprint-110f-final-report.md."
                    Log.w(TAG, "startCapture: state transition $prevState -> ERROR (establish() returned null, Magisk hint emitted)")
                    notifyError(lastError!!)
                    return@synchronized state
                }
                Log.d(TAG, "startCapture: builder.establish() returned pfd=$pfd (TUN descriptor acquired)")
                tunInterface = pfd
                running.set(true)
                state = State.SAMPLING
                Log.d(TAG, "startCapture: SAMPLING started, pfd=$pfd, state transition $prevState -> $state")
                // Sprint 11.0S-DNS — check whether Android
                // Private DNS (DNS-over-TLS, since Android 9)
                // is active on the device. When Private DNS
                // is enabled, the system overrides the
                // VPN's `addDnsServer(1.1.1.1)` resolver and
                // routes all DNS queries through the user's
                // Private DNS hostname (Cloudflare DoT,
                // Google DoT, etc.) — which means the VPN
                // tunnel sees no DNS traffic even though
                // `addDnsServer` was called. Result: Chrome
                // and WhatsApp can resolve domain names
                // (via DoT) but the user gets no
                // "VPN-tunneled" DNS, and any app that
                // forces cleartext DNS over the VPN (e.g.
                // via `bindProcessToNetwork(VPN)`) fails
                // because the resolver is unreachable.
                //
                // The fix is two-fold:
                //   (1) Detect the conflict early (here, in
                //       startCapture after `establish()`)
                //       via
                //       `LinkProperties.isPrivateDnsActive`.
                //       Log a warning + push a telemetry
                //       event so the Dart side can show a
                //       snackbar.
                //   (2) Bind the process to the VPN network
                //       via
                //       `ConnectivityManager.bindProcessToNetwork`
                //       so any cleartext DNS queries from
                //       this process go through the VPN
                //       tunnel and hit the `addDnsServer`
                //       resolvers (1.1.1.1 / 1.0.0.1) — not
                //       the system Private DNS override.
                //
                // The Dart side checks `lastError` (or
                // a new field) for the Private DNS
                // warning and shows a snackbar:
                // "Ozel DNS kapali olmali - Ayarlar > Ag
                // ve internet > Ozel DNS > Kapali".
                checkPrivateDnsAndBindToVpn()
                packetsObserved.set(0)
                // Sprint 11.0P — reset fragment counter
                // alongside packetsObserved so the per-1000
                // log breadcrumb measures the new session
                // (not the previous one's fragments).
                ipFragmentCount.set(0)
                // Sprint 11.0T — reset passthrough counter
                // alongside packetsObserved so the per-1000
                // log breadcrumb compares the new session's
                // read+write counts (not the previous
                // session's).
                passthroughCount.set(0)
                synchronized(ringLock) { ring.clear() }
                startForegroundCompat()
                Log.d(TAG, "startCapture: startForegroundCompat() returned (foreground promotion OK)")
                startReaderThread(pfd)
                Log.d(TAG, "startCapture: startReaderThread(pfd) returned (TUN reader thread spawned)")
                // Sprint 11.0A — start the 5-second scheduled drain that
                // pushes the current ring to Dart via the shared
                // methodChannel. The handler is `PacketDrain::tick`.
                startDrainLoop()
                Log.d(TAG, "startCapture: startDrainLoop() returned (5-second scheduled drain armed)")
                // Sprint 11.0S-EXTRA — schedule the
                // 15-minute auto-stop. The foreground
                // notification chronometer (set in
                // buildForegroundNotification) counts
                // down from now+15min to now+0; when
                // the Handler fires, the VPN tears
                // down gracefully. We use a Handler
                // on the main looper (not a Timer) so
                // the postDelayed is lightweight
                // (~1 wakeup at 00:00, no per-second
                // polling) and the system keeps the
                // notification visible until then.
                scheduleCountdownAutoStop()
                Log.d(TAG, "startCapture: scheduleCountdownAutoStop() returned (15-min countdown armed)")
                Log.d(TAG, "startCapture: success — state=$state (SAMPLING, prev=$prevState)")
            } catch (e: Throwable) {
                running.set(false)
                state = State.ERROR
                lastError = "startCapture failed: ${e.javaClass.simpleName}: ${e.message}"
                Log.e(TAG, "startCapture: state transition $prevState -> ERROR (caught exception)", e)
                notifyError(lastError!!)
            }
            return@synchronized state
        }
    }

    /**
     * Tear down TUN, flush ring, notify Dart.
     *
     * Sprint 11.0H — wrapped in `synchronized(stateLock)` so the
     * TOCTOU guard covers the stop path too. The lock is
     * intentional — without it, a racing `startCapture` could
     * see `running.get() == false` (because stop hadn't yet
     * set it) and start a second TUN while the first stop
     * was still in flight. The log breadcrumbs at entry / DRAINING
     * / DONE are the S78 invariant.
     */
    private fun stopCapture(@Suppress("UNUSED_PARAMETER") graceful: Boolean): State {
        // Sprint 11.0S-EXTRA — cancel the pending
        // 15-minute auto-stop Handler. If the user
        // manually disconnects (toggle OFF, 11.0R
        // "Oturumu Bitir" button, or 11.0Q
        // MainActivity.disconnectVpn), we tear down
        // NOW; the 00:00 Handler wakeup should not
        // fire later on an already-stopped service.
        countdownAutoStopRunnable?.let { runnable ->
            mainHandler.removeCallbacks(runnable)
        }
        countdownAutoStopRunnable = null
        return synchronized(stateLock) {
            val prevState = state
            Log.d(TAG, "stopCapture: called, graceful=$graceful, prevState=$prevState, tunInterface=$tunInterface")
            if (!running.get() && tunInterface == null) {
                state = State.STOPPED
                // Sprint 11.0V — ALREADY-IDLE BRANCH. The
                // TUN was already torn down by a prior
                // stop, but the bounded queue (`ring`)
                // and the per-session counter
                // (`packetsObserved`) may still hold
                // the stale 10 packets from the
                // previous session. Owner 20:19
                // reported `getSampledPackets()`
                // returning 10 packets after VPN
                // stop — the Dart `poolProvider` used
                // those to bump the UI counter, making
                // it look like the VPN was still
                // capturing. Both branches MUST clear
                // the ring + reset the counter so the
                // NEXT session starts from 0 0
                // (Sprint 11.0R invariant). S95 audit
                // verifies this branch.
                synchronized(ringLock) { ring.clear() }
                packetsObserved.set(0)
                Log.d(TAG, "stopCapture: DONE (was already idle), state transition $prevState -> $state (ring cleared, packetsObserved=0)")
                return@synchronized state
            }
            state = State.DRAINING
            Log.d(TAG, "stopCapture: state transition $prevState -> $state (TUN tear-down starts)")
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

        // Sprint 11.0A — cancel the scheduled drain loop. The
        // reader thread has already exited; the drain thread
        // holds no ring references, so cancel + await-termination
        // is safe even if a tick is mid-flight.
        stopDrainLoop()

        // Sprint 11.0V — NORMAL TEARDOWN BRANCH. Clear
        // the bounded queue + reset the per-session
        // counter BEFORE the final telemetry flush so
        // the last batch doesn't include stale ring
        // data, AND so the NEXT `getSampledPackets()`
        // call (after a fresh `requestAndStart()`) returns
        // 0 packets instead of the previous session's
        // 10. Owner 20:19: pre-11.0V the ring held 10
        // packets after VPN stop; the Dart `poolProvider`
        // read those 10 and bumped the UI counter, making
        // it look like the VPN was still capturing. S95
        // audit verifies this branch.
        synchronized(ringLock) { ring.clear() }
        packetsObserved.set(0)
        // Also reset the per-session passthrough
        // and fragment counters (Sprint 11.0P/11.0T
        // invariant — these are per-session too, so
        // they should reset on stop, not just on start).
        ipFragmentCount.set(0)
        passthroughCount.set(0)

        // Send the final telemetry batch.
        flushTelemetry()
        running.set(false)
        val newState = State.STOPPED
        state = newState
        Log.d(TAG, "stopCapture: DONE, state transition $prevState -> $newState (ring cleared, packetsObserved=0, fragmentCount=0, passthroughCount=0)")
        return@synchronized newState
        }
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
     * Sprint 11.0K — push a method-call from a background thread
     * (PacketDrain ScheduledExecutorService worker, TUN reader
     * thread, etc.) to the Dart side over the MethodChannel.
     *
     * The Flutter Engine REQUIRES `MethodChannel.invokeMethod`
     * to be invoked on the Android UI thread (the main
     * `Looper`). The error is the `@UiThread` annotation
     * violation:
     *
     *   `onPacketsSampled push failed: Methods marked with
     *    @UiThread must be executed on the main thread.
     *    Current thread: opene2ee-vpn-drain.`
     *
     * This is the OnePlus 9 Pro "VPN active, internet
     * working, but UI never updates" symptom (Owner 12:31
     * logcat: 98 packets observed in 80s, deltaPerInterval
     * Log.d in logcat, but Dart side never receives the
     * `onPacketsSampled` event so `state: DRAINING` and
     * `ringSize: 0` stay frozen in the UI).
     *
     * Pre-11.0K, the three call sites (flushTelemetry's
     * `onTelemetry`, notifyError's `onError`, PacketDrain's
     * `onPacketsSampled`) called `methodChannel?.invokeMethod`
     * directly from their caller threads — which are the
     * `opene2ee-vpn-drain` ScheduledExecutorService worker
     * thread (PacketDrain) and the TUN reader thread
     * (flushTelemetry / notifyError invoked from
     * `startCapture`). 11.0K wraps ALL three in a
     * `Handler(Looper.getMainLooper()).post { ... }` so the
     * call is dispatched to the main looper.
     *
     * Audit S82 verifies:
     *   1. ALL `methodChannel?.invokeMethod` AND
     *      `ch.invokeMethod` calls in this file are wrapped
     *      in `Handler(Looper.getMainLooper()).post { ... }`
     *      (a `Handler` field declared on the companion is
     *      the canonical pattern).
     *   2. The companion declares a `@JvmField val mainHandler:
     *      Handler = Handler(Looper.getMainLooper())` so the
     *      post is one-line at the call site.
     */
    private fun pushToDart(method: String, args: Any?) {
        mainHandler.post {
            try {
                methodChannel?.invokeMethod(method, args)
            } catch (t: Throwable) {
                // Logged as warn (not error) because the
                // engine being detached / engine swap is a
                // benign race during config changes; the
                // outer `lastError` field is the canonical
                // error surface for Dart.
                Log.w(TAG, "pushToDart: $method push failed: ${t.message}")
            }
        }
    }

    /**
     * Forward a metadata batch back to Dart over the MethodChannel.
     * Called once at stop time and on ring-cap threshold.
     */
    private fun flushTelemetry() {
        val payload: List<Map<String, Any?>> = synchronized(ringLock) { ring.toList() }
        // Sprint 11.0K — push to Dart on the main looper
        // (Flutter Engine `@UiThread` requirement). Pre-11.0K
        // this called `methodChannel?.invokeMethod` from the
        // TUN reader thread; the Engine threw `@UiThread`
        // and Dart never received `onTelemetry`. 11.0K
        // delegates to `pushToDart` which `mainHandler.post`s
        // to the UI thread.
        pushToDart(
            "onTelemetry",
            mapOf(
                "sessionId" to null, // populated by Dart-side glue from PR-7's sessionId
                "packets" to payload,
                "capturedAt" to System.currentTimeMillis(),
            ),
        )
    }

    private fun notifyError(message: String) {
        // Sprint 11.0F — diagnostic breadcrumb. The `onError` push
        // to Dart is the user-visible error surface; logging it
        // here too ensures the Owner (or any logcat session) sees
        // the error even if the channel push fails (e.g. Dart
        // side not listening).
        Log.e(TAG, "notifyError: $message")
        // Sprint 11.0K — push to Dart on the main looper
        // (Flutter Engine `@UiThread` requirement).
        pushToDart(
            "onError",
            mapOf(
                "code" to "vpn_runtime_error",
                "message" to message,
            ),
        )
    }

    /**
     * Sprint 11.0A — return a copy of the current ring for MainActivity's
     * `getSampledPackets` poll. Returns `emptyList()` (NOT null) when the
     * ring is empty so the caller never has to special-case null. The
     * `ActivePoolScreen` subscribes to the [OpenE2eeVpnService.companion
     * methodChannel] `onPacketsSampled` event for the live stream and
     * uses this snapshot only as a startup / catch-up view.
     */
    internal fun snapshotRing(): List<Map<String, Any?>> {
        return synchronized(ringLock) { ring.toList() }
    }

    /**
     * Sprint 11.0J — return the current ring size under [ringLock].
     * Used by [PacketDrain.run] to log `ringSize` in the
     * `deltaPerInterval` breadcrumb. Public-internal visibility
     * so the `PacketDrain` inner class can call it.
     */
    internal fun synchronizedRingSizeForDrain(): Int =
        synchronized(ringLock) { ring.size }

    /**
     * Sprint 11.0A — start the 5-second scheduled drain loop. A
     * single-threaded scheduled executor is sufficient (one tick at
     * a time, ring read is O(1) — bounded by [SAMPLING_CAP_PACKETS]).
     * The tick handler is [PacketDrain.tick].
     */
    private fun startDrainLoop() {
        if (drainExecutor == null) {
            drainExecutor = Executors.newSingleThreadScheduledExecutor { r ->
                Thread(r, "opene2ee-vpn-drain").apply { isDaemon = true }
            }
        }
        drainTask?.cancel(false)
        drainTask = drainExecutor!!.scheduleAtFixedRate(
            PacketDrain(this),
            DRAIN_INTERVAL_SECONDS,
            DRAIN_INTERVAL_SECONDS,
            TimeUnit.SECONDS,
        )
        Log.d(TAG, "drain loop started (every ${DRAIN_INTERVAL_SECONDS}s)")
    }

    /**
     * Sprint 11.0A — cancel the drain loop. Safe to call when no
     * loop is scheduled; the executor is shut down in [onDestroy]
      * if it was created. Matches the [startReaderThread] →
      * `t.join(1_000L)` cleanup idiom in [stopCapture].
      */

    /**
     * Sprint 11.0S-DNS — Private DNS conflict detection
     * + VPN network process binding. Owner 17:14 logcat
     * showed `packetsObserved` was real (1394 packets in
     * <2 min) and `fragmentRatePct=0` (Sprint 11.0P MTU
     * fix is good) but Chrome / WhatsApp "no internet" —
     * the visible symptom is the user can browse by IP
     * but domain names don't resolve. This is the
     * Android Private DNS (DNS-over-TLS) override
     * conflict documented in celzero/rethink-app
     * issue #25: when Private DNS is enabled
     * (Android 9+ default on OnePlus OxygenOS), the
     * system ignores `addDnsServer(1.1.1.1)` and
     * routes ALL DNS through the user's Private DNS
     * hostname (Cloudflare DoT, Google DoT, etc.).
     *
     * 11.0S-DNS does two things:
     *   (1) DETECT the conflict via
     *       `LinkProperties.isPrivateDnsActive` and
     *       log a warning. Push a telemetry event
     *       `lastError = "private_dns_active"` so the
     *       Dart side can show a snackbar: "Ozel DNS
     *       kapali olmali - Ayarlar > Ag ve internet >
     *       Ozel DNS > Kapali".
     *   (2) BIND the process to the VPN network via
     *       `ConnectivityManager.bindProcessToNetwork(
     *       network)`. This forces any cleartext DNS
     *       queries from THIS process (e.g. from the
     *       `TelematicsService` HTTPS call which
     *       resolves a hostname before connecting)
     *       to use the VPN tunnel's `addDnsServer`
     *       resolvers (1.1.1.1 / 1.0.0.1) — bypassing
     *       the system Private DNS override.
     *       S91 audit verifies the
     *       `bindProcessToNetwork` call site.
     */
    private fun checkPrivateDnsAndBindToVpn() {
        // Sprint 11.0W — 5 explicit Log.d breadcrumbs
        // at every step of the DNS check + bind.
        // Owner 20:45 reported `checkPrivateDnsAndBindToVpn
        // log YOK logcatte` — the previous version only
        // logged in the error/exception path. If the
        // function SILENTLY returned early (e.g. if
        // requestNetwork never fires onAvailable/onUnavailable
        // on OnePlus OxygenOS), there was NO breadcrumb
        // at all. S96 audit verifies all 5 Log.d tokens.
        try {
            // (1) ENTRY breadcrumb.
            Log.d(TAG, "DNS: checkPrivateDnsAndBindToVpn: ENTRY")
            val cm = getSystemService(android.content.Context.CONNECTIVITY_SERVICE) as ConnectivityManager
            // Check the active network's LinkProperties for
            // Private DNS. We probe the active network
            // BEFORE our VPN comes up because the VPN
            // interface is not yet in the network
            // registry (it shows up a moment after
            // establish() returns).
            val activeNet = cm.activeNetwork
            if (activeNet != null) {
                val lp: LinkProperties? = cm.getLinkProperties(activeNet)
                if (lp != null) {
                    // (2) isPrivateDnsActive breadcrumb.
                    // OnePlus OxygenOS sometimes sets a
                    // Private DNS hostname that resolves to
                    // NXDOMAIN. Log the hostname alongside
                    // the boolean so the Owner can confirm
                    // in logcat whether the hostname is
                    // a known-bad one (e.g. an unreachable
                    // cellular carrier DNS that returns
                    // NXDOMAIN and disables Private DNS
                    // silently).
                    val serverName = try {
                        lp.privateDnsServerName ?: "automatic"
                    } catch (e: Throwable) { "unknown" }
                    Log.d(TAG, "DNS: LinkProperties.isPrivateDnsActive=${lp.isPrivateDnsActive}, privateDnsServerName=$serverName")
                    if (lp.isPrivateDnsActive) {
                        Log.w(TAG, "DNS: Android Private DNS is ACTIVE on the active network — VPN addDnsServer will be ignored by the system. User must disable Private DNS: Settings > Network & internet > Private DNS > Off.")
                        // Stash the warning in `lastError` so the
                        // Dart side can show a snackbar via
                        // the existing `lastError` state field
                        // (no new field needed — the Dart
                        // `stateToMap` already serializes it).
                        lastError = "private_dns_active: VPN DNS bypassed by Android DoT. Disable Settings > Network > Private DNS."
                    }
                }
            }
            // (2) Bind this process to the VPN network.
            // We request a network with TRANSPORT_VPN
            // and bind the process when it becomes
            // available. The bind is process-wide (any
            // socket opened by this process after the
            // bind will use the VPN network by default).
            val request = NetworkRequest.Builder()
                .addTransportType(NetworkCapabilities.TRANSPORT_VPN)
                .build()
            // (3) requestNetwork start breadcrumb.
            // OnePlus OxygenOS sometimes silently
            // drops requestNetwork(TRANSPORT_VPN) if
            // the device's VPN profile is in a
            // half-configured state — the callback
            // never fires onAvailable or onUnavailable.
            // This Log.d confirms the request was
            // actually issued.
            Log.d(TAG, "DNS: ConnectivityManager.requestNetwork(TRANSPORT_VPN) start")
            // Sprint 11.0X — 5s activeNetwork FALLBACK.
            // Owner 21:08 logcat: requestNetwork start
            // breadcrumb appeared but the callback NEVER
            // fired (for 1 minute) on OnePlus OxygenOS.
            // The pre-11.0X code only logged inside the
            // onAvailable/onUnavailable lambdas, so when
            // the callback was never invoked there was
            // no second breadcrumb. 11.0X adds:
            //   (a) An AtomicBoolean flag `callbackFired`
            //       set true in BOTH onAvailable AND
            //       onUnavailable (so we know the
            //       callback WAS invoked even if the
            //       result is "unavailable").
            //   (b) A Handler.postDelayed Runnable that
            //       fires after 5 seconds. If the flag
            //       is still false, we attempt the
            //       activeNetwork fallback: read
            //       `cm.activeNetwork`, check
            //       `getNetworkCapabilities(activeNet)
            //       .hasTransport(TRANSPORT_VPN)`,
            //       and if true call
            //       `bindProcessToNetwork(activeNet)`.
            //   (c) Log.e with Magisk DenyList
            //       troubleshooting hint if BOTH paths
            //       fail (so the Owner + Mavis can see
            //       the root cause in logcat).
            val callbackFired = java.util.concurrent.atomic.AtomicBoolean(false)
            val fallbackHandler = Handler(Looper.getMainLooper())
            val fallbackRunnable = Runnable {
                if (!callbackFired.get()) {
                    Log.d(TAG, "DNS: NetworkCallback TIMEOUT (5s) - attempting activeNetwork fallback")
                    try {
                        val activeNet = cm.activeNetwork
                        if (activeNet != null) {
                            val nc = cm.getNetworkCapabilities(activeNet)
                            if (nc != null && nc.hasTransport(NetworkCapabilities.TRANSPORT_VPN)) {
                                val bindResult = cm.bindProcessToNetwork(activeNet)
                                Log.d(TAG, "DNS: FALLBACK bindProcessToNetwork(activeNetwork) result=$bindResult")
                                if (!bindResult) {
                                    Log.e(TAG, "DNS: FALLBACK bind returned false. Check Magisk DenyList (Settings > Magisk > DenyList - ensure opene2ee is NOT in the list).")
                                }
                            } else {
                                Log.e(TAG, "DNS: FALLBACK activeNetwork has NO TRANSPORT_VPN. VPN profile not established. Check: (1) Magisk DenyList - opene2ee must NOT be in the list; (2) OnePlus OxygenOS Battery optimization - exclude opene2ee; (3) Android 14 foreground service type - confirm the foreground service is running.")
                            }
                        } else {
                            Log.e(TAG, "DNS: FALLBACK activeNetwork is NULL. VPN profile not established. Owner troubleshooting: (1) Magisk DenyList: Settings > Magisk > DenyList - remove opene2ee if listed; (2) confirm VPN toggle is ON in system Settings; (3) reboot device to reset VPN subsystem.")
                        }
                    } catch (e: Throwable) {
                        Log.e(TAG, "DNS: FALLBACK failed: ${e.message}")
                    }
                }
            }
            fallbackHandler.postDelayed(fallbackRunnable, 5_000L)
            cm.requestNetwork(request, object : ConnectivityManager.NetworkCallback() {
                override fun onAvailable(network: Network) {
                    // (4a) NetworkCallback.onAvailable
                    // success breadcrumb. Set the flag
                    // + cancel the 5s fallback (the happy
                    // path is reached, no need for the
                    // activeNetwork probe).
                    callbackFired.set(true)
                    fallbackHandler.removeCallbacks(fallbackRunnable)
                    Log.d(TAG, "DNS: NetworkCallback.onAvailable (VPN network up), attempting bindProcessToNetwork")
                    try {
                        // (5) bindProcessToNetwork result
                        // breadcrumb. Log.d the boolean
                        // return value (true=bind OK,
                        // false=bind silently failed —
                        // common on OnePlus OxygenOS if
                        // the VPN profile is not in the
                        // "connected" state).
                        val bindResult = cm.bindProcessToNetwork(network)
                        Log.d(TAG, "DNS: bindProcessToNetwork(vpn) result=$bindResult")
                    } catch (e: Throwable) {
                        Log.w(TAG, "DNS: bindProcessToNetwork(vpn) failed: ${e.message}")
                    } finally {
                        try { cm.unregisterNetworkCallback(this) } catch (_: Throwable) {}
                    }
                }
                override fun onUnavailable() {
                    // (4b) NetworkCallback.onUnavailable
                    // failure breadcrumb. Set the flag
                    // + cancel the 5s fallback (we got a
                    // definitive "no VPN network" answer,
                    // the activeNetwork probe would not
                    // help).
                    callbackFired.set(true)
                    fallbackHandler.removeCallbacks(fallbackRunnable)
                    Log.d(TAG, "DNS: NetworkCallback.onUnavailable (no VPN network for bindProcessToNetwork)")
                    try { cm.unregisterNetworkCallback(this) } catch (_: Throwable) {}
                }
            })
        } catch (e: Throwable) {
            // 11.0S-DNS is best-effort: a failure here
            // (e.g. on Android 7 where bindProcessToNetwork
            // is gated) does NOT block the VPN from
            // running. Log + continue.
            Log.w(TAG, "DNS: checkPrivateDnsAndBindToVpn failed: ${e.message}")
        }
    }

    private fun stopDrainLoop() {
        drainTask?.cancel(false)
        drainTask = null
    }

    /**
     * Sprint 11.0A — periodic drain task. Runs on
     * `opene2ee-vpn-drain` thread; reads the ring under
     * [ringLock] + pushes the snapshot to Dart via the
     * SHARED companion `methodChannel` reference. The snapshot
     * is a List<Map<String, Any?>> that the Dart side
     * deserializes into `List<SampledPacket>`.
     *
     * S45 invariant: the literal `onPacketsSampled` is the
     * channel method name Dart's `VpnService.packetStream`
     * listener subscribes to.
     */
    private class PacketDrain(private val service: OpenE2eeVpnService) : Runnable {
        /**
         * Sprint 11.0J — `prev` counter for the
         * `deltaPerInterval` breadcrumb. Persistent across `run()`
         * invocations (one `PacketDrain` instance is created in
         * `startDrainLoop` and reused for every 5s tick via
         * `scheduleAtFixedRate`). The breadcrumb makes the
         * passthrough regression visible: if `deltaPerInterval = 0`
         * while `running.get() = true` and the foreground
         * notification is visible, the reader thread is NOT
         * draining the TUN — the passthrough is broken (the user's
         * internet is dead).
         */
        private var prevPacketsObserved: Int = service.packetsObserved.get()

        override fun run() {
            // Sprint 11.0J — emit the deltaPerInterval breadcrumb
            // BEFORE the `onPacketsSampled` push so the log line is
            // visible even if the channel push throws.
            val currPacketsObserved = service.packetsObserved.get()
            val delta = currPacketsObserved - prevPacketsObserved
            prevPacketsObserved = currPacketsObserved
            val ringSize = service.synchronizedRingSizeForDrain()
            Log.d(TAG, "PacketDrain: tick, packetsObserved=$currPacketsObserved, " +
                    "deltaPerInterval=$delta, ringSize=$ringSize")
            val packets: List<Map<String, Any?>> = service.snapshotRing()
            val ch = OpenE2eeVpnService.methodChannel
            if (ch == null) {
                // No Dart subscriber yet (engine not attached). Drop
                // the snapshot; the next 5s tick will try again.
                return
            }
            // Sprint 11.0K — push to Dart on the main looper.
            // Pre-11.0K, the inline `ch.invokeMethod` was called
            // on the `opene2ee-vpn-drain` ScheduledExecutorService
            // worker thread. The Flutter Engine threw `@UiThread:
            // Methods marked with @UiThread must be executed on the
            // main thread. Current thread: opene2ee-vpn-drain` and
            // the Owner saw `state: DRAINING, packetsObserved: 0,
            // ringSize: 0` even though the TUN reader was pushing
            // 98 packets in 80s (the ring was full, the drain was
            // ticking, but Dart never received the events).
            // 11.0K delegates to `pushToDart` which `mainHandler.post`s
            // to the UI thread.
            //
            // Audit S82 verifies the event name `"onPacketsSampled"`
            // (S45 invariant) AND that the call is wrapped in
            // `mainHandler.post { ... }` (S82 invariant).
            OpenE2eeVpnService.mainHandler.post {
                try {
                    ch.invokeMethod("onPacketsSampled", packets)
                } catch (t: Throwable) {
                    Log.w(TAG, "onPacketsSampled push failed: ${t.message}")
                }
            }
        }
    }

    /**
     * Spawn the dedicated TUN reader thread. The thread reads up to MTU-sized
     * packets in a tight loop, extracts metadata via [extractMetadata], pushes
     * a metadata entry into the ring (cap = [SAMPLING_CAP_PACKETS]), and
     * WRITES the packet back to the TUN output stream so the OS can forward
     * it to the real network interface (transparent VPN passthrough).
     * Payload bytes are NEVER copied off-device — only the IP/transport
     * header fields are inspected via [extractMetadata].
     *
     * Sprint 11.0J — passthrough was MISSING pre-11.0J. The pre-11.0J
     * code opened the TUN input stream, read packets, but never
     * wrote them back. Combined with `.addRoute("0.0.0.0", 0)` (default
     * route) this caused the OS to drop ALL the user's internet
     * traffic after 5-30 seconds, triggering a system-side
     * `onRevoke()` and the `state: DRAINING` regression Owner
     * observed on PID 4244 (12:14 logcat). The fix opens BOTH the
     * TUN input stream (read packets from kernel) AND the TUN
     * output stream (write packets back to kernel — kernel then
     * routes them out the real NIC) and writes the SAME bytes
     * back to the output. S80 audit invariant: this is the
     * load-bearing pattern.
     */
    private fun startReaderThread(pfd: ParcelFileDescriptor) {
        // Sprint 11.0J — `AutoCloseInputStream` /
        // `AutoCloseOutputStream` close the underlying ParcelFileDescriptor
        // when the stream is closed (i.e., in the `finally` block).
        // The pre-11.0J code used `FileInputStream(pfd.fileDescriptor)`
        // which only closed the file descriptor reference, leaking
        // the kernel-side TUN fd across restart cycles. The
        // `AutoClose*` variants are the canonical pattern from
        // `VpnService.Builder.establish()` Javadoc.
        val input = ParcelFileDescriptor.AutoCloseInputStream(pfd)
        val output = ParcelFileDescriptor.AutoCloseOutputStream(pfd)
        val thread = Thread({
            val buf = ByteArray(TUN_MTU)
            try {
                while (running.get()) {
                    val n = try {
                        input.read(buf)
                    } catch (e: IOException) {
                        // TUN closed — normal shutdown path.
                        Log.d(TAG, "startReaderThread: TUN input EOF / IOException, exiting reader loop")
                        break
                    }
                    if (n <= 0) {
                        Log.d(TAG, "startReaderThread: read returned $n bytes, exiting reader loop")
                        break
                    }
                    // Sprint 11.0P — IP fragment detection.
                    // IPv4 header at offset 6-7 carries the
                    // flags (3 bits) + fragment offset (13
                    // bits). The MF (More Fragments) bit is
                    // the high bit of byte 6. A non-zero
                    // fragment offset (low 5 bits of byte 6 +
                    // byte 7) also indicates a fragment. A
                    // fragment rate > 5% on a mobile network
                    // is a strong indicator that TUN MTU is
                    // too high (the radio link drops fragments
                    // > 1500 bytes). 11.0P sets TUN_MTU = 1400
                    // so the radio link (1500-byte MTU minus
                    // 78-byte GTP/IPsec trailer) can carry the
                    // 1400-byte TUN frame. The fragment count
                    // is surfaced in the per-1000-packet log
                    // breadcrumb below.
                    if (n >= 20 && (buf[0].toInt() and 0xFF) ushr 4 == 4) {
                        val flagsAndOffset =
                            (buf[6].toInt() and 0xFF) shl 8 or
                                (buf[7].toInt() and 0xFF)
                        val mfSet = (flagsAndOffset and 0x2000) != 0
                        val fragOffset = flagsAndOffset and 0x1FFF
                        if (mfSet || fragOffset != 0) {
                            ipFragmentCount.incrementAndGet()
                        }
                    }
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
                        // Sprint 11.0P — per-1000-packet MTU +
                        // fragment breadcrumb. Owner 13:50: the
                        // TUN_MTU = 1500 was too high for
                        // Turkcell's GTP-U encapsulation
                        // (78-byte trailer). 11.0P lowered
                        // TUN_MTU to 1400 and the Owner can
                        // now verify by grepping `adb logcat`
                        // for `startReaderThread: MTU=$TUN_MTU,
                        // fragmentCount=...` every 1000 packets.
                        // A fragment rate > 5% is a strong
                        // signal the MTU is still too high; a
                        // rate < 0.1% confirms the mobile-safe
                        // 1400-byte MTU is working.
                        if (packetsObserved.get() % 1000 == 0) {
                            val total = packetsObserved.get().toLong()
                            val fragments = ipFragmentCount.get()
                            val fragRatePct = if (total > 0) {
                                (fragments.toDouble() * 100.0 / total.toDouble())
                            } else {
                                0.0
                            }
                            Log.d(TAG, "startReaderThread: MTU=$TUN_MTU, " +
                                    "packetsObserved=$total, " +
                                    "ipFragmentCount=$fragments, " +
                                    "fragmentRatePct=${"%.2f".format(fragRatePct)}, " +
                                    // Sprint 11.0T — passthrough
                                    // diagnostic. Owner 18:19
                                    // reported passthrough is NOT
                                    // writing. The Owner greps
                                    // `adb logcat` for this line
                                    // and confirms
                                    // `passthroughCount == packetsObserved`
                                    // (every captured packet is
                                    // also being passthrough-written).
                                    // If `passthroughCount` is 0
                                    // after a `curl 212.64.210.85/healthz`
                                    // test, the write is failing
                                    // (the `try { output.write ...`
                                    // catch block is the source —
                                    // grep the Log.e line for
                                    // the exception class).
                                    "passthroughCount=${passthroughCount.get()}, " +
                                    "passthroughGap=${
                                        if (total > 0)
                                            total - passthroughCount.get()
                                        else 0L
                                    }")
                        }
                        if (packetsObserved.get() == SAMPLING_CAP_PACKETS) {
                            // Notify Dart early so the UI can react mid-session.
                            flushTelemetry()
                        }
                    }
                    // Sprint 11.0J — TRANSPARENT PASSTHROUGH. Write the
                    // SAME bytes back to the TUN output stream. The
                    // kernel then routes the packet out the device's
                    // actual NIC (real network interface). WITHOUT this
                    // write, the kernel drops the packet (since the
                    // TUN consumed it from the input side) and the
                    // user's internet is dead. Pre-11.0J, the code
                    // called `protect(Socket)` and immediately closed
                    // it — that protects a SOCKET from the VPN, but
                    // there's no socket to protect here. The actual
                    // pattern for a transparent capture VPN is to
                    // WRITE the packet back to the TUN output. The
                    // `protect()` call was a 11.0A-era misconception
                    // and has been REMOVED in 11.0J.
                    //
                    // Sprint 11.0K — `flushTelemetry()` is called
                    // from this TUN reader thread; its underlying
                    // `methodChannel?.invokeMethod` is dispatched
                    // to the Android UI thread via `pushToDart` →
                    // `mainHandler.post { ... }` to satisfy the
                    // Flutter Engine `@UiThread` requirement.
                    // Pre-11.0K, this push happened on the TUN
                    // reader thread directly and the engine threw
                    // `@UiThread` violations for `onTelemetry`
                    // (and `onPacketsSampled` from the PacketDrain
                    // worker thread). The visible symptom was
                    // Owner-12:31's "VPN active, internet OK, UI
                    // never updates" — 98 packets in 80s, drain
                    // tick visible, but Dart never got the events.
                    // Sprint 11.0T — 5-LIMBED DEBUG per
                    // Owner 18:19. The brief: passthrough
                    // is NOT actually writing (curl
                    // 212.64.210.85/healthz fails with VPN,
                    // works without). 5 limbs:
                    //   1. tun.write() called per
                    //      read+write — Log.d + passthroughCount
                    //      increment.
                    //   2. output stream valid? —
                    //      pfd.fileDescriptor.valid() check.
                    //   3. output.flush() immediate?
                    //      Yes (per-packet) — see
                    //      `try { output.flush() }` below.
                    //   4. DNS UDP 53 capture? — detect
                    //      the IP protocol + UDP dst port 53
                    //      and log so the Owner can grep.
                    //   5. passthrough count for any IP
                    //      (e.g. 212.64.210.85) > 0? —
                    //      surfaced in the per-1000-packet
                    //      breadcrumb below.
                    // (2) pfd validity check.
                    if (!pfd.fileDescriptor.valid()) {
                        Log.e(TAG, "startReaderThread: TUN pfd.fileDescriptor.valid() = false (fd revoked?); exiting reader loop")
                        break
                    }
                    val writeOk = try {
                        // (1) write + flush + increment.
                        output.write(buf, 0, n)
                        output.flush()
                        passthroughCount.incrementAndGet()
                        true
                    } catch (e: IOException) {
                        // TUN output closed mid-flight — common during
                        // the Magisk Zygisk revoke path (Sprint 11.0H).
                        // Log and exit the reader loop; the service
                        // will tear down via `onRevoke` /
                        // `stopCapture`.
                        Log.e(TAG, "startReaderThread: TUN output write FAILED (IOException, n=$n, packetsObserved=${packetsObserved.get()}, passthroughCount=${passthroughCount.get()}): ${e.message}; exiting reader loop", e)
                        false
                    } catch (t: Throwable) {
                        // (5) broader Throwable catch — the
                        // Owner 18:19 root cause may be a
                        // non-IOException (e.g. an
                        // IllegalStateException on a closed
                        // AutoCloseOutputStream). Log + exit
                        // so `adb logcat` shows the actual
                        // exception class + message.
                        Log.e(TAG, "startReaderThread: TUN output write FAILED (UNEXPECTED Throwable, n=$n, packetsObserved=${packetsObserved.get()}, passthroughCount=${passthroughCount.get()}): ${t.javaClass.simpleName}: ${t.message}", t)
                        false
                    }
                    if (!writeOk) break
                    // (4) DNS UDP 53 detection. IPv4
                    // protocol = 17, UDP header at offset
                    // 9 of IP packet, dst port at offset
                    // 2 of UDP header (in big-endian).
                    // Log every 50th DNS packet so the
                    // Owner can grep `adb logcat` to
                    // confirm DNS queries are reaching
                    // the TUN.
                    if (n >= 28 && (buf[0].toInt() and 0xFF) ushr 4 == 4) {
                        val proto = buf[9].toInt() and 0xFF
                        if (proto == 17 && n >= 28) {
                            val udpDst = (buf[22].toInt() and 0xFF) shl 8 or
                                (buf[23].toInt() and 0xFF)
                            if (udpDst == 53 || udpDst == 853) {
                                val dnsCount = passthroughCount.get()
                                if (dnsCount % 50 == 0L) {
                                    Log.d(TAG, "startReaderThread: DNS packet captured (UDP dst port $udpDst, n=$n, passthroughCount=$dnsCount)")
                                }
                            }
                        }
                    }
                }
            } catch (t: Throwable) {
                Log.e(TAG, "TUN reader crashed", t)
                lastError = "reader: ${t.javaClass.simpleName}: ${t.message}"
            } finally {
                try { input.close() } catch (_: IOException) {}
                try { output.close() } catch (_: IOException) {}
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
        val maskedSrc = maskIpv4(src as Inet4Address)
        val maskedDst = maskIpv4(dst as Inet4Address)

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
            "srcIpMasked" to maskIpv6(src as Inet6Address),
            "dstIpMasked" to maskIpv6(dst as Inet6Address),
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
    /**
     * Sprint 11.0E — public idempotent entry point for the foreground
     * promotion. Called from [onStartCommand] as the FIRST statement
     * (so the 5-second `startForeground()` rule is satisfied even if
     * `Builder.establish()` later throws or returns null) and from
     * [startForegroundCompat] (for back-compat with the 11.0A path
     * inside `startCapture()`).
     *
     * Idempotent: a `@Volatile` boolean guards against a second
     * `startForeground` call from leaking the foreground state
     * (Android allows re-calling `startForeground` with a different
     * notification id, but doing so unnecessarily wakes the
     * notification shade). The guard is best-effort: the
     * system-supplied `startForeground` itself is idempotent on the
     * notification side, so a missed guard just means the user sees
     * a one-frame notification refresh.
     */
    private val foregroundStarted = java.util.concurrent.atomic.AtomicBoolean(false)

    /**
     * Sprint 11.0E — promote the service to foreground state
     * synchronously. MUST be called from [onStartCommand] (or
     * equivalent service lifecycle hook) BEFORE any IO that could
     * exceed Android's 5-second `startForeground()` deadline
     * (TUN setup, DNS resolution, etc.).
     *
     * Steps:
     *   1. Ensure the notification channel exists (idempotent,
     *      Android 8+).
     *   2. Build the foreground notification.
     *   3. Call `startForeground()` with the typed 3-arg overload on
     *      Android 14+ (API 34) so the foregroundServiceType matches
     *      the manifest `foregroundServiceType="specialUse"`. On
     *      older API levels, falls back to the 2-arg overload via
     *      [ServiceCompat.startForeground].
     */
    fun ensureForegroundService() {
        Companion.ensureNotificationChannel(this)
        val notification: Notification = buildForegroundNotification()
        // Android 14+ (UPSIDE_DOWN_CAKE = API 34) requires the typed
        // 3-arg `startForeground(id, notification, foregroundServiceType)`
        // overload so the foregroundServiceType matches the manifest
        // declaration. On older API levels, `ServiceCompat.startForeground`
        // is the canonical shim — it routes to the typed 4-arg form on
        // API 29+ (Q) and the legacy 2-arg form on API < 29.
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.UPSIDE_DOWN_CAKE) {
            startForeground(
                NOTIFICATION_ID,
                notification,
                ServiceInfo.FOREGROUND_SERVICE_TYPE_SPECIAL_USE,
            )
        } else {
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
        foregroundStarted.set(true)
    }

    /**
     * Build the foreground-service notification. Centralised so the
     * title / content text / icon are consistent between the
      * Sprint 11.0E `onStartCommand` path and the legacy 11.0A
      * `startCapture()` path. S50 invariant: NO "VPN" string in any
      * user-facing surface.
      */

    /**
     * Sprint 11.0S-EXTRA — schedule the
     * 15-minute auto-stop. Posts a Runnable on
     * `mainHandler` (the Android main looper, set
     * up in 11.0K for the MethodChannel dispatch)
     * that calls `stopCapture(graceful = true)`
     * at the exact moment the chronometer hits
     * 00:00. The chronometer counts down in the
     * notification bar WITHOUT per-second Kotlin
     * polling — the system handles the display.
     * On 00:00 the Runnable fires once and tears
     * down the VPN. S92 audit verifies the call
     * site + the Handler.postDelayed pattern.
     */
    private fun scheduleCountdownAutoStop() {
        // Cancel any prior auto-stop (idempotent).
        countdownAutoStopRunnable?.let { runnable ->
            mainHandler.removeCallbacks(runnable)
        }
        val runnable = Runnable {
            Log.d(TAG, "scheduleCountdownAutoStop: 15-minute countdown reached 00:00 — auto-stopping VPN")
            try {
                stopCapture(graceful = true)
            } catch (e: Throwable) {
                Log.w(TAG, "scheduleCountdownAutoStop: stopCapture threw: ${e.message}")
            }
            countdownAutoStopRunnable = null
        }
        countdownAutoStopRunnable = runnable
        mainHandler.postDelayed(runnable, COUNTDOWN_TOTAL_MS)
    }

    private fun buildForegroundNotification(): Notification {
        // Sprint 11.0S-EXTRA — native Android
        // chronometer. `setUsesChronometer(true)`
        // tells the system to render a countdown
        // timer at the right edge of the
        // notification, using `setWhen(endTimeMs)`
        // as the target. The system polls the
        // display internally (no Kotlin Timer
        // needed — saves CPU + battery). When
        // `now == setWhen`, the chronometer reads
        // "00:00" and the auto-stop Handler
        // (scheduled in `startCapture`) fires
        // `stopCapture(graceful = true)`.
        val endTimeMs = System.currentTimeMillis() + COUNTDOWN_TOTAL_MS
        return NotificationCompat.Builder(this, NOTIFICATION_CHANNEL_ID)
            .setContentTitle("OpenE2EE Şifreleme Doğrulama")
            .setContentText("Ağınızda ilk $SAMPLING_CAP_PACKETS paket analiz ediliyor (PRIVACY_TEXT eki) — 15 dk sonra otomatik kapanır")
            .setSmallIcon(android.R.drawable.ic_lock_lock)
            .setOngoing(true)
            .setUsesChronometer(true)
            .setWhen(endTimeMs)
            .setShowWhen(true)
            .build()
    }

    private fun startForegroundCompat() {
        // Sprint 11.0E — route through the centralised helper so the
        // notification-channel creation + startForeground overload
        // selection stay in lockstep with `ensureForegroundService`.
        // Kept as a private back-compat alias for the 11.0A call
        // site inside `startCapture()` (which is no longer the
        // primary path — `onStartCommand` is — but still reachable
        // if the service is restarted via the OS).
        ensureForegroundService()
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        // Sprint 11.0F — diagnostic breadcrumb. Pairs with the
        // `startCapture: entry` line further down so the Owner can
        // confirm the service was actually created by the system
        // (vs. the dispatcher's `startForegroundService` call having
        // been intercepted somewhere).
        Log.d(TAG, "onStartCommand: entry (intent.action=${intent?.action}, startId=$startId)")
        // Sprint 11.0E — CRITICAL: call `startForeground()` BEFORE any
        // other work. Android 8+ (API 26+) imposes a 5-second rule:
        // when a service is started via `Context.startForegroundService(...)`,
        // the service MUST call `Service.startForeground(id, notification)`
        // within 5 seconds. If it does not, the system kills the service
        // and throws `android.app.RemoteServiceException: Context
        // .startForegroundService() did not then call Service
        // .startForeground()`, taking the whole app down with it
        // (this is the OnePlus 9 Pro crash Owner reported at
        // 10:29 on 11.07.2026).
        //
        // Pre-Sprint-11.0E, `startForegroundCompat()` was only called
        // from inside `startCapture()` — AFTER `Builder.establish()`
        // (TUN setup, can be slow on some OEM ROMs) and AFTER the
        // `Builder.establish() == null` early-return path. If TUN
        // setup returned null (user declined consent, system refused)
        // or threw, `startForeground()` was NEVER called → the
        // 5-second rule was violated → RemoteServiceException.
        //
        // The fix: hoist the foreground-promotion to the FIRST
        // statement in `onStartCommand`, BEFORE any IO. The
        // notification is the same one `startCapture()` would have
        // shown (S50 invariant: "OpenE2EE Şifreleme Doğrulama",
        // no "VPN" string). The Android 14+ (API 34) typed overload
        // is used on UPSIDE_DOWN_CAKE+ so the foregroundServiceType
        // matches the manifest `foregroundServiceType="specialUse"`.
        ensureForegroundService()
        Log.d(TAG, "onStartCommand: ensureForegroundService() returned (foreground promotion OK)")

        // The Dart side calls `startCapture()` via the MethodChannel
        // rather than through service-start intents — but we honour
        // intent-launched starts (e.g. Android's autostart on reboot)
        // as a fallback.
        if (intent?.action == ACTION_PREPARE) {
            // No-op here; the actual `prepare`/consent dialog is
            // handled by MainActivity which has the Activity context.
            Log.d(TAG, "onStartCommand: intent.action=ACTION_PREPARE — no-op (consent dialog owned by MainActivity)")
        } else if (running.get() == false) {
            Log.d(TAG, "onStartCommand: about to call startCapture() (running=false)")
            startCapture()
            Log.d(TAG, "onStartCommand: startCapture() returned (state=$state)")
        } else {
            Log.d(TAG, "onStartCommand: already running, skipping startCapture (idempotent)")
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
        // Sprint 11.0A — shut down the drain executor. stopCapture
        // already cancelled the ScheduledFuture; the executor's
        // single worker thread is daemon so shutdownNow is safe.
        drainExecutor?.shutdownNow()
        drainExecutor = null
        drainTask = null
        super.onDestroy()
    }

    override fun onRevoke() {
        // Sprint 11.0H — diagnostic breadcrumb at the system-side
        // revoke path. The Owner saw `start` return `state: DRAINING`
        // (not `state: SAMPLING`) which suggested `stopCapture` was
        // called from somewhere external — `onRevoke` (system
        // settings / Magisk Zygisk revoke) is one candidate. The
        // log line identifies the path so the next regression can
        // be diagnosed via `adb logcat -d -s OpenE2eeVpn:V`.
        Log.w(TAG, "onRevoke: VPN profile revoked by system (Magisk Zygisk or settings or user); tearing down")
        stopCapture(graceful = true)
        super.onRevoke()
    }
}
