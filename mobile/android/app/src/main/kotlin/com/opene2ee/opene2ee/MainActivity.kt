// mobile/android/app/src/main/kotlin/com/opene2ee/opene2ee/MainActivity.kt
//
// PR-22a (Sprint 3) — Android MainActivity.
//
// Owns the VpnService permission handshake:
//
//   1. Dart calls `requestPrepare` over the `opene2ee/vpn` MethodChannel.
//   2. MainActivity invokes `VpnService.prepare(this)`.
//   3. Android shows the system "VPN connection" consent dialog.
//   4. `onActivityResult(RESULT_OK)` confirms consent; we notify Dart.
//   5. Dart then calls `start`; the service brings up the TUN.
//
// Why the activity owns the flow:
//   `VpnService.prepare(Context)` requires an Activity context. The
//   `OpenE2eeVpnService` itself is a `Service` and only has the
//   application context, so the activity must launch the prepare intent.
//
// References:
//   - docs/ADR-0003-vpn-layer.md
//   - docs/SPRINT-3-SCOPE.md §7 PR-22

package com.opene2ee.opene2ee

import android.app.Activity
import android.content.Intent
import android.net.VpnService
import android.os.Build
import android.os.Bundle
import com.opene2ee.opene2ee.vpn.OpenE2eeVpnService
import io.flutter.embedding.android.FlutterActivity
import io.flutter.embedding.engine.FlutterEngine
import io.flutter.plugin.common.MethodCall
import io.flutter.plugin.common.MethodChannel

/**
 * Single-Activity host that bridges the Flutter side to Android system
 * services — specifically the VPN consent flow.
 */
class MainActivity : FlutterActivity() {

    companion object {
        private const val TAG = "MainActivity"
        private const val VPN_REQUEST_CODE = 0x7B_50_4E /* VPN' */
        private const val PERMISSIONS_CHANNEL = "opene2ee/vpn_permissions"
    }

    /** The MethodChannel that carries the permission-request roundtrip. */
    private var permissionsChannel: MethodChannel? = null

    /**
     * Cached Dart-side completion for the in-flight `requestVpnPermission`
     * call. We need this because `onActivityResult` runs before any Dart
     * future is even awaited.
     */
    private var pendingVpnResult: MethodChannel.Result? = null

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
    }

    /**
     * Wired once on engine attach. Sets up:
     *   1. The OPENe2eeVpnService MethodChannel (delegated to the service itself).
     *   2. A permission-request channel owned by THIS activity.
     */
    override fun configureFlutterEngine(flutterEngine: FlutterEngine) {
        super.configureFlutterEngine(flutterEngine)

        // Hand the engine to the VpnService so it can call back into Dart.
        OpenE2eeVpnService().attachFlutterEngine(flutterEngine)

        // Permission-request channel — Dart invokes `requestVpnPermission`.
        permissionsChannel = MethodChannel(
            flutterEngine.dartExecutor.binaryMessenger,
            PERMISSIONS_CHANNEL,
        ).apply {
            setMethodCallHandler(::onPermissionsCall)
        }
    }

    /**
     * Handle Dart → Activity commands.
     */
    private fun onPermissionsCall(call: MethodCall, result: MethodChannel.Result) {
        when (call.method) {
            "requestVpnPermission" -> requestVpnPermission(result)
            "isVpnPrepared" -> result.success(isVpnPrepared())
            else -> result.notImplemented()
        }
    }

    /**
     * Launch `VpnService.prepare(this)`. The Android system shows the
     * standard "VPN connection request" consent sheet. The result is
     * delivered to [onActivityResult].
     */
    private fun requestVpnPermission(result: MethodChannel.Result) {
        if (pendingVpnResult != null) {
            result.error("vpn_prepare_in_flight", "Already awaiting a permission result", null)
            return
        }
        val intent = VpnService.prepare(this)
        if (intent == null) {
            // No consent dialog needed — we're already prepared or the
            // device has no VPN framework (emulator without Google APIs).
            result.success(true)
            return
        }
        pendingVpnResult = result
        try {
            @Suppress("DEPRECATION")
            startActivityForResult(intent, VPN_REQUEST_CODE)
        } catch (e: Throwable) {
            pendingVpnResult = null
            result.error("vpn_prepare_launch_failed", e.message, null)
        }
    }

    /**
     * Snapshot: have we already obtained consent? Use this to avoid
     * showing the dialog twice in the same session.
     */
    private fun isVpnPrepared(): Boolean {
        // VpnService.prepare() returning null on a fresh call is the
        // canonical "already authorised" signal.
        return VpnService.prepare(this) == null
    }

    override fun onActivityResult(requestCode: Int, resultCode: Int, data: Intent?) {
        super.onActivityResult(requestCode, resultCode, data)
        if (requestCode != VPN_REQUEST_CODE) return
        val pending = pendingVpnResult ?: return
        pendingVpnResult = null
        when (resultCode) {
            Activity.RESULT_OK -> pending.success(true)
            Activity.RESULT_CANCELED -> pending.success(false)
            else -> pending.error("vpn_prepare_unknown_result", "resultCode=$resultCode", null)
        }
    }

    /**
     * Android 13+ runtime notification permission — the foreground-service
     * notification for the VPN session needs this. We only request it when
     * we are actually about to start a session, to minimize the prompt
     * surface area (per ADR-0006).
     */
    @Suppress("UNUSED_PARAMETER")
    fun ensureNotificationPermission(result: MethodChannel.Result) {
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.TIRAMISU) {
            result.success(true)
            return
        }
        val granted = checkSelfPermission(android.Manifest.permission.POST_NOTIFICATIONS) ==
                android.content.pm.PackageManager.PERMISSION_GRANTED
        if (granted) {
            result.success(true)
        } else {
            requestPermissions(arrayOf(android.Manifest.permission.POST_NOTIFICATIONS), 0x4E_4F_54_49)
            // The Dart side polls `status` if the user grants asynchronously.
            // A precise promise is out of scope for this method-channel surface
            // and easy to layer in a follow-up.
            result.success(false)
        }
    }

    override fun onDestroy() {
        permissionsChannel?.setMethodCallHandler(null)
        permissionsChannel = null
        // Detach the service's channel so the engine doesn't double-fire.
        OpenE2eeVpnService().detachFlutterEngine()
        super.onDestroy()
    }
}
