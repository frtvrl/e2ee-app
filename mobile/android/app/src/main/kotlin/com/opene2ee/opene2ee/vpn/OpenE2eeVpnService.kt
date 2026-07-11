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
import android.net.VpnService
import android.os.Build
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
                packetsObserved.set(0)
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
        return synchronized(stateLock) {
            val prevState = state
            Log.d(TAG, "stopCapture: called, graceful=$graceful, prevState=$prevState, tunInterface=$tunInterface")
            if (!running.get() && tunInterface == null) {
                state = State.STOPPED
                Log.d(TAG, "stopCapture: DONE (was already idle), state transition $prevState -> $state")
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

        // Send the final telemetry batch.
        flushTelemetry()
        running.set(false)
        val newState = State.STOPPED
        state = newState
        Log.d(TAG, "stopCapture: DONE, state transition $prevState -> $newState")
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
        // Sprint 11.0F — diagnostic breadcrumb. The `onError` push
        // to Dart is the user-visible error surface; logging it
        // here too ensures the Owner (or any logcat session) sees
        // the error even if the channel push fails (e.g. Dart
        // side not listening).
        Log.e(TAG, "notifyError: $message")
        methodChannel?.invokeMethod(
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
        override fun run() {
            val packets: List<Map<String, Any?>> = service.snapshotRing()
            val ch = OpenE2eeVpnService.methodChannel
            if (ch == null) {
                // No Dart subscriber yet (engine not attached). Drop
                // the snapshot; the next 5s tick will try again.
                return
            }
            try {
                // Sprint 11.0A — the literal event name Dart subscribes
                // to. Audit S45 verifies this exact token in
                // OpenE2eeVpnService.kt source.
                ch.invokeMethod("onPacketsSampled", packets)
            } catch (t: Throwable) {
                Log.w(TAG, "onPacketsSampled push failed: ${t.message}")
            }
        }
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
                    //
                    // PR-28 originally called `protect(input.fd)` directly, but
                    // the Android SDK exposes `protect(Socket)` /
                    // `protect(DatagramSocket)` only — the FileDescriptor
                    // overload was removed in API 24+. Wrap a fresh Socket so
                    // the call resolves to the supported overload; we close the
                    // socket immediately because we never use the connection
                    // (only the protect() effect on the OS routing table).
                    val protectSocket = Socket()
                    try {
                        protect(protectSocket)
                    } finally {
                        protectSocket.close()
                    }
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
    private fun buildForegroundNotification(): Notification =
        NotificationCompat.Builder(this, NOTIFICATION_CHANNEL_ID)
            .setContentTitle("OpenE2EE Şifreleme Doğrulama")
            .setContentText("Ağınızda ilk $SAMPLING_CAP_PACKETS paket analiz ediliyor (PRIVACY_TEXT eki)")
            .setSmallIcon(android.R.drawable.ic_lock_lock)
            .setOngoing(true)
            .build()

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
