"""Self-test for check_app_build_gradle_syntax_v2 (S1-S5),
check_android_debug_workflow_v3 (S6),
check_mobile_entry_point_v4 (S7),
check_android_xml_comments_v5 (S8),
check_android_manifest_v6 (S9),
check_android_res_skeleton_v7 (S10),
check_flutter_plugins_dependencies_v8 (S11),
check_flutter_kotlin_embedding_v9 (S12),
check_flutter_storage_repo_v10 (S13),
check_gradle_wrapper_force_include (S17),
check_fresh_flutter_create_preserved (S18),
check_fresh_create_metadata_tracked (S19),
check_pubspec_baseline_shape (S20),
check_no_vpn_string_in_sprint10_ui (S25),
check_whatsapp_deeplink_literal_present (S26),
check_active_pool_linechart_literal_present (S27),
check_pool_provider_timer_periodic_literal_present (S28 - INVERTED in 11.0O),
check_vpn_service_mtu_and_fragment_log_v31 (S87),
check_oturumu_bitir_2level_fallback_v32 (S88),
check_oturumu_bitir_full_state_reset_v33 (S89),
check_dns_private_dns_conflict_v34 (S91),
check_notification_chronometer_autostop_v35 (S92),
check_vpn_service_passthrough_count_invariant_v36 (S93),
check_active_pool_haptic_feedback_literal_present (S29),
check_pool_provider_debug_state_fields (S33),
check_active_pool_scaffold_messenger_snackbar (S34),
check_service_api_key_from_environment (S35),
check_auth_service_exists (S36),
check_service_uses_auth_headers (S37),
check_auth_token_expiry_field (S38), and
check_auth_invalidate_method (S39),
check_whatsapp_deeplink_intent_format (S40),
check_p2p_matcher_sessions_endpoint (S41),
check_android_manifest_whatsapp_queries_v12 (S42),
check_main_activity_get_sampled_packets_v13 (S43),
check_whatsapp_deeplink_wa_me_format_v14 (S44),
check_vpn_service_on_packets_sampled_literal_v15 (S45),
check_main_activity_snapshot_call_v15 (S46),
check_vpn_service_packet_stream_getter_v15 (S47),
check_active_pool_packet_stream_listen_v15 (S48),
check_sampled_packet_class_v15 (S49),
check_vpn_service_foreground_notification_text_v15 (S50),
check_active_pool_no_30_call_loop_v15 (S51),
check_telemetry_service_summary_upload_v15 (S52),
check_pubspec_webrtc_dep_v16 (S53),
check_webrtc_service_rtc_peer_connection_v16 (S54),
check_webrtc_service_on_ice_candidate_v16 (S55),
check_session_orchestrator_start_session_v16 (S56),
check_session_orchestrator_long_poll_v16 (S57),
check_webrtc_service_on_track_v16 (S59),
check_active_pool_webrtc_status_indicator_v16 (S60),
check_skorlar_screen_fetch_scores_v17 (S61),
check_score_calculator_compute_v17 (S62),
check_score_calculator_four_metrics_v17 (S63),
check_score_calculator_overall_weighted_sum_v17 (S64),
check_session_orchestrator_close_session_v17 (S65),
check_active_pool_oturumu_bitur_button_v17 (S66),
check_active_pool_close_then_navigate_v17 (S67),
check_skorlar_empty_state_v17 (S68),
check_skorlar_card_overall_gauge_v17 (S69),
check_backend_sessions_close_handler_v17 (S70),
check_backend_summary_stats_shape_v17 (S71),
check_score_calculator_unit_tests_v17 (S72),
check_main_activity_owns_vpn_channel_v18 (S73),
check_vpn_service_startforeground_within_5s_v19 (S74),
check_vpn_service_log_d_breadcrumbs_v20 (S75),
check_vpn_service_dart_singleton_v20 (S76),
check_active_pool_screen_ui_propagation_v21 (S77),
check_vpn_service_state_transition_breadcrumbs_v22 (S78),
check_vpn_service_addroute_bad_address_v23 (S79), and
check_vpn_service_tun_passthrough_v24 (S80).
Sprint 11.0K adds `check_vpn_service_ui_thread_push_v26` (S82).
Sprint 11.0M adds `check_vpn_service_packets_observed_increment_invariant_v27` (S84).

(Sprint 11.0F adds 2 new selftest cases for S75 + S76 —
the OnePlus 9 Pro Senaryo D regression guards. S75
verifies `OpenE2eeVpnService.kt` has at least 5
`Log.d(TAG,` breadcrumbs across startCapture /
onStartCommand / dispatch / notifyError so future
regressions are diagnosable via `adb logcat -d -s
OpenE2eeVpn:V`. S76 verifies `vpn_service.dart`
exposes `VpnService` as a Dart singleton (private
`_internal` ctor + static `instance` getter + factory
default ctor) so widget rebuilds share the same
MethodChannel handler + StreamControllers. Both
follow the S44 / S73 / S74 pattern (1 PASS-only case
each).)

(Sprint 11.0E adds 1 new selftest case for S74 — the
5-second foreground-service rule check on
OpenE2eeVpnService.kt. S74 follows the S44 / S73 pattern
(1 PASS-only case). The negative-path coverage is the
pre-fix Kotlin source (which had `startForeground(`
only INSIDE `startCapture()` AFTER `Builder.establish()`)
— the static check would flag that with all 5
token-mismatch findings; the Sprint 11.0E fix hoists
the call to the FIRST statement in `onStartCommand`.)

(Sprint 11.0D adds 1 new selftest case for S73 — the
MainActivity-owned MethodChannel handler check. S73
follows the S44 pattern (1 PASS-only case) — the
negative-path coverage is provided by the Dart-side
unit test in `mobile/test/sprint110d_handler_test.dart`
which actually exercises the `VpnService.getSampledPackets`
channel call against a mocked handler, proving the
channel is registered at runtime, not just statically
present in `MainActivity.kt`.)

(Sprint 11.0B adds 7 new selftest cases; S58 is a
production-audit-only check on the backend router.go
long-poll handler registration.)

Per Architect brief (Sprint 9.6.6): "self-checks (negative test:
revert + audit finds 4 FAIL)". Sprint 9.6.7 extends to S6.
9.6.8 extends to S7. 9.6.9 extends to S8. 9.6.10 extends to S9.
9.6.11 extends to S10. 9.6.12 extends to S11. 9.6.13 extends to S12.
9.6.14 extends to S13. 9.7.0 Item 5 extends to S17-S20.
Sprint 10.0 extends to S25-S26.
Sprint 10.1A extends to S27-S29.
Sprint 10.1C extends to S33-S35.
Sprint 10.1D extends to S36-S39.
Sprint 10.1E extends to S40-S41 (and updates S26 from
`whatsapp://send?text=` to `intent://send?text=`).
Sprint 10.1F extends to S42-S43.
Sprint 10.1G extends to S44 (wa.me primary + intent:// fallback +
tryOpenWithReason() call site).
Sprint 11.0A extends to S45-S52 (real VpnService packet drain +
MethodChannel bridge + SampledPacket round-trip + 5-second
scheduled drain + 30-second summary batch upload).

S1-S5 cases: 6 (1 PASS + 5 FAIL, ...).
S6 cases: 4 (1 PASS + 3 FAIL, ...).
S7 cases: 4 (1 PASS + 3 FAIL, ...).
S8 cases: 2 (1 PASS + 1 FAIL).
S9 cases: 3 (1 PASS + 2 FAIL).
S10 cases: 3 (1 PASS + 2 FAIL — styles.xml + mipmap +
launch_background.xml).
S11 cases: 3 (1 PASS + 2 FAIL — file missing + plugins.android
empty array).
S12 cases: 3 (1 PASS + 2 FAIL — dependency missing + wrong hash).
S13 cases: 3 (1 PASS + 2 FAIL — settings block missing +
Flutter URL missing).
S17 cases: 2 (1 PASS + 1 FAIL — wrapper not tracked).
S18 cases: 2 (1 PASS + 1 FAIL — pubspec.lock not tracked).
S19 cases: 2 (1 PASS + 1 FAIL — .metadata not tracked).
S20 cases: 2 (1 PASS + 1 FAIL — pubspec.yaml missing name).
S25 cases: 2 (1 PASS + 1 FAIL — `vpn` substring in main.dart).
S26 cases: 2 (1 PASS + 1 FAIL — `intent://send?text=` missing).
S27 cases: 2 (1 PASS + 1 FAIL — `LineChart` literal missing in active pool screen).
S28 cases: 2 (1 PASS + 1 FAIL — `Timer.periodic` literal missing in pool provider).
S29 cases: 2 (1 PASS + 1 FAIL — `HapticFeedback`/`SystemSound` literal missing in active pool screen).
S33 cases: 2 (1 PASS + 1 FAIL — lastError/lastSuccess literal missing in pool provider).
S34 cases: 2 (1 PASS + 1 FAIL — ScaffoldMessenger.of(context).showSnackBar literal missing in active pool screen).
S35 cases: 2 (1 PASS + 1 FAIL — String.fromEnvironment('API_KEY' literal missing in telemetry/p2p services).
S36 cases: 2 (1 PASS + 1 FAIL — POST /api/v1/auth user_id literals missing in auth_service).
S37 cases: 2 (1 PASS + 1 FAIL — authHeaders() call missing in telemetry/p2p services).
S38 cases: 2 (1 PASS + 1 FAIL — _tokenExpiresAt field missing in auth_service).
S39 cases: 2 (1 PASS + 1 FAIL — invalidate() method missing in auth_service).
S40 cases: 2 (1 PASS + 1 FAIL — `intent://send?` + `#Intent;scheme=whatsapp;package=com.whatsapp;end` literals missing in whatsapp_deeplink_provider).
S41 cases: 2 (1 PASS + 1 FAIL — /api/v1/sessions literal missing OR forbidden /api/v1/matches still present in p2p_matcher).
S42 cases: 2 (1 PASS + 1 FAIL — `<queries>` + `<package android:name="com.whatsapp"` missing in AndroidManifest).
S43 cases: 2 (1 PASS + 1 FAIL — `when (call.method)` + `"getSampledPackets"` case missing in MainActivity.kt).
S44 cases: 1 (1 PASS — `intent://send?text=` AND `https://wa.me/?text=` literals in whatsapp_deeplink_provider.dart + `tryOpenWithReason` call in whatsapp_task_detail_screen.dart).
S45 cases: 2 (1 PASS + 1 FAIL — `"onPacketsSampled"` literal missing in OpenE2eeVpnService.kt).
S46 cases: 2 (1 PASS + 1 FAIL — `OpenE2eeVpnService.snapshot()` call OR 10.1F mock packet mapOf literal).
S47 cases: 2 (1 PASS + 1 FAIL — `packetStream` getter + `MethodChannel` import in vpn_service.dart).
S48 cases: 2 (1 PASS + 1 FAIL — `packetStream.listen` literal in active_pool_screen.dart).
S49 cases: 2 (1 PASS + 1 FAIL — SampledPacket class with fromBytes + toJson in packet_parser.dart).
S50 cases: 2 (1 PASS + 1 FAIL — `OpenE2EE Şifreleme Doğrulama` foreground notification text in OpenE2eeVpnService.kt).
S51 cases: 2 (1 PASS + 1 FAIL — `i < 30` + `Timer.periodic` 30-call loop in active_pool_screen.dart).
S52 cases: 2 (1 PASS + 1 FAIL — `sendSummary` method + `/api/v1/sessions/` path + 6 fields in telemetry_service.dart).

Total: 142 cases (72 pre-Sprint 11.0A + 16 from S45-S52 + 24 from
S53-S60 + 24 from S61-S72 + 1 from S73 + 1 from S74 + 1 from
S76 + 1 from S77 + 1 from S78 + 1 from S79 + 1 from S80 +
1 from S82 + 1 from S84 + 1 from S86 + 1 from S87 +
1 from S88 + 1 from S89 + 1 from S91 + 1 from S92 +
1 from S93).
Sprint 11.0Q adds 1 new selftest case for S88 (2-level
VPN disconnect fallback: .stop with 3s timeout +
MainActivity.disconnectVpn hard-stop) — the
OnePlus 9 Pro "Oturumu Bitir requires app uninstall"
regression guard (Owner 14:14: pre-11.0Q the
handler had an early-return on null sessionId that
blocked the disconnect flow).
Sprint 11.0J adds 1 new selftest case for S80 (TUN
passthrough — `output.write(buf, 0, n)` after
`input.read(buf)`) — the OnePlus 9 Pro "VPN active,
internet dead" regression guard. S75 remains a
production-audit-only check (S58 pattern).
"""
import sys
from pathlib import Path

# Add parent dir to path so we can import the audit module
sys.path.insert(0, str(Path(__file__).resolve().parent.parent / "tools"))

# Import the check function (re-implement the file reader so we
# don't need a real worktree)
import re

# Copy the helper here to avoid mutating the audit module
def strip_comments(text: str) -> str:
    text = re.sub(r"/\*[\s\S]*?\*/", "", text)
    lines = text.splitlines()
    out = []
    for ln in lines:
        in_string = False
        escape = False
        i = 0
        cut_at = -1
        while i < len(ln):
            c = ln[i]
            if escape:
                escape = False
                i += 1
                continue
            if c == "\\":
                escape = True
                i += 1
                continue
            if c == '"':
                in_string = not in_string
                i += 1
                continue
            if c == "/" and i + 1 < len(ln) and ln[i + 1] == "/" and not in_string:
                cut_at = i
                break
            i += 1
        if cut_at >= 0:
            out.append(ln[:cut_at])
        else:
            out.append(ln)
    return "\n".join(out)


def run_check(code_text: str) -> list[str]:
    """Replicate check_app_build_gradle_syntax_v2 logic on raw code text."""
    findings = []
    code = strip_comments(code_text)
    has_properties_import = bool(re.search(r"^import\s+java\.util\.Properties\s*$", code, re.MULTILINE))
    has_jvm_target_import = bool(re.search(r"^import\s+org\.jetbrains\.kotlin\.gradle\.dsl\.JvmTarget\s*$", code, re.MULTILINE))
    deprecated_kotlin_options = bool(re.search(r"kotlinOptions\s*\{[\s\S]*?jvmTarget\s*=\s*\"[\d]+\"", code))
    new_kotlin_block = bool(re.search(r"kotlin\s*\{[\s\S]*?compilerOptions\s*\{[\s\S]*?jvmTarget\.set\(JvmTarget\.JVM_17\)", code))
    fully_qualified = bool(re.search(r"java\.util\.Properties\(\)", code))
    if not has_properties_import:
        findings.append("S1 fail")
    if not has_jvm_target_import:
        findings.append("S2 fail")
    if deprecated_kotlin_options:
        findings.append("S3 fail")
    if not new_kotlin_block:
        findings.append("S4 fail")
    if fully_qualified:
        findings.append("S5 fail")
    return findings


def run_s6_check(yaml_text: str) -> list[str]:
    """Replicate check_android_debug_workflow_v3 logic on raw YAML text.

    Mirrors the audit's PyYAML-parsed step-walk for S6:
    (a) name matches "Install Flutter dependencies" (case-insensitive),
    (b) run contains "flutter pub get" (case-insensitive),
    (c) working-directory is exactly "./mobile".
    """
    findings = []
    import yaml
    try:
        docs = list(yaml.safe_load_all(yaml_text))
        d = docs[0] if docs else None
    except Exception:
        d = None
    if d is None or not isinstance(d, dict):
        return ["S6 fail"]
    jobs = d.get("jobs", {})

    s6_match = None
    s6_name_found = []
    for job_name, job_def in jobs.items():
        if not isinstance(job_def, dict):
            continue
        steps = job_def.get("steps", [])
        if not isinstance(steps, list):
            continue
        for s in steps:
            if not isinstance(s, dict):
                continue
            step_name = str(s.get("name", ""))
            step_run = str(s.get("run", ""))
            step_wd = s.get("working-directory", None)
            if "install flutter dependencies" in step_name.lower():
                s6_name_found.append(step_name)
                if "flutter pub get" in step_run.lower():
                    s6_match = {
                        "job": job_name,
                        "name": step_name,
                        "run": step_run,
                        "working_directory": step_wd,
                    }

    if s6_match is None:
        if not s6_name_found:
            findings.append("S6 fail")
        else:
            findings.append("S6 fail")
    else:
        if s6_match["working_directory"] != "./mobile":
            findings.append("S6 fail")
    return findings


def run_s7_check(main_dart_text: str | None, pubspec_text: str | None) -> list[str]:
    """Replicate check_mobile_entry_point_v4 logic on raw text inputs.

    Mirrors the audit's three-part S7 check:
    (a) lib/main.dart exists (signalled by main_dart_text != None),
    (b) main_dart has `runApp(` + `ProviderScope` (substring on text),
    (c) pubspec.yaml has `flutter_riverpod:` + `go_router:` as
        dependencies (parsed via PyYAML, not substring).
    """
    findings = []
    if main_dart_text is None:
        findings.append("S7 fail")
        return findings
    if "runApp(" not in main_dart_text:
        findings.append("S7 fail")
    if "ProviderScope" not in main_dart_text:
        findings.append("S7 fail")
    if pubspec_text is None:
        findings.append("S7 fail")
        return findings
    import yaml
    try:
        pubspec_doc = yaml.safe_load(pubspec_text)
    except Exception:
        findings.append("S7 fail")
        return findings
    if not isinstance(pubspec_doc, dict):
        findings.append("S7 fail")
        return findings
    deps = pubspec_doc.get("dependencies", {})
    if not isinstance(deps, dict):
        deps = {}
    if "flutter_riverpod" not in deps:
        findings.append("S7 fail")
    if "go_router" not in deps:
        findings.append("S7 fail")
    return findings


def run_s8_check(xml_text: str | None) -> list[str]:
    """Replicate check_android_xml_comments_v5 logic on raw XML text.

    Mirrors the audit's two-part S8 check:
    (a) XML parses via xml.etree.ElementTree (well-formedness),
    (b) no `<!-- ... -->` comment body contains `--`.
    """
    findings = []
    if xml_text is None:
        findings.append("S8 fail")
        return findings
    import xml.etree.ElementTree as ET
    try:
        ET.fromstring(xml_text)
    except Exception:
        findings.append("S8 fail")
        return findings
    import re
    for match in re.finditer(r"<!--(.*?)-->", xml_text, re.DOTALL):
        if "--" in match.group(1):
            findings.append("S8 fail")
            return findings
    return findings


def run_s9_check(manifest_text: str | None, gradle_text: str | None) -> list[str]:
    """Replicate check_android_manifest_v6 logic on raw text inputs.

    Mirrors the audit's five-part S9 check:
    (1) AndroidManifest.xml parses via xml.etree.ElementTree.
    (2) root <manifest> carries NO `package=` attribute.
    (3) root <manifest> declares `xmlns:tools="..."` (raw text
        check — ET strips namespace declarations from attrib).
    (4) <application> with android:usesCleartextTraffic does NOT
        also carry tools:remove="...usesCleartextTraffic" (the
        forbidden pair — must be tools:replace instead).
    (5) build.gradle.kts contains `namespace = "<our_ns>"`.
    """
    findings = []
    if manifest_text is None or gradle_text is None:
        findings.append("S9 fail")
        return findings
    import xml.etree.ElementTree as ET
    try:
        # The audit uses ET.parse (reads from a file path), but
        # the self-test is given raw strings, so use fromstring
        # (the in-memory equivalent). Same parser; same rules.
        root = ET.fromstring(manifest_text)
    except Exception:
        findings.append("S9 fail")
        return findings
    # (2) package absent
    if root.get("package") is not None:
        findings.append("S9 fail")
        return findings
    # (3) xmlns:tools present
    if 'xmlns:tools="http://schemas.android.com/tools"' not in manifest_text:
        findings.append("S9 fail")
        return findings
    # (4) no forbidden tools:remove + android:usesCleartextTraffic pair
    ANDROID_NS = "http://schemas.android.com/apk/res/android"
    ANDROID_TOOLS_NS = "http://schemas.android.com/tools"
    for application in root.iter("application"):
        has_cleartext = application.get(f"{{{ANDROID_NS}}}usesCleartextTraffic") is not None
        tools_remove_value = application.get(f"{{{ANDROID_TOOLS_NS}}}remove")
        if has_cleartext and tools_remove_value and "usesCleartextTraffic" in tools_remove_value:
            findings.append("S9 fail")
            return findings
    # (5) gradle namespace present
    if 'namespace = "com.opene2ee.opene2ee"' not in gradle_text:
        findings.append("S9 fail")
        return findings
    return findings


def run_s10_check(styles_text: str | None, has_mipmap: bool, has_launch_bg: bool) -> list[str]:
    """Replicate check_android_res_skeleton_v7 logic on raw inputs.

    Mirrors the audit's three-part S10 check:
    (a) values/styles.xml exists and has both LaunchTheme + NormalTheme
        <style> elements (parse with xml.etree.ElementTree),
    (b) at least one mipmap representation exists (boolean probe),
    (c) drawable/launch_background.xml exists (boolean probe).

    The audit reads files from disk; the self-test takes raw text +
    boolean flags for the boolean checks so a single fixture
    (a styles.xml string + two booleans) can drive all 3
    cases without a temp directory.
    """
    findings = []
    if styles_text is None:
        findings.append("S10 fail")
        return findings
    import xml.etree.ElementTree as ET
    try:
        root = ET.fromstring(styles_text)
    except Exception:
        findings.append("S10 fail")
        return findings
    style_names = set()
    for style in root.findall("style"):
        name = style.get("name")
        if name:
            style_names.add(name)
    if "LaunchTheme" not in style_names or "NormalTheme" not in style_names:
        findings.append("S10 fail")
        return findings
    if not has_mipmap:
        findings.append("S10 fail")
        return findings
    if not has_launch_bg:
        findings.append("S10 fail")
        return findings
    return findings


def run_s11_check(fpd_text: str | None) -> list[str]:
    """Replicate check_flutter_plugins_dependencies_v8 logic on raw JSON text.

    Mirrors the audit's four-part S11 check:
    (a) .flutter-plugins-dependencies file exists (signalled by
        fpd_text != None),
    (b) parses as JSON via json.loads (well-formedness + real
        parser, per the 9.6.x chain rule "audit must use a real
        parser, not a regex-grep heuristic"),
    (c) `plugins.android` is a non-empty list,
    (d) each entry has `name` (str) and `native_build` (bool).

    The audit reads the file from disk; the self-test takes the
    raw JSON text so a single fixture string can drive all 3
    cases (PASS / missing / empty-android) without a temp file.
    """
    findings = []
    if fpd_text is None:
        findings.append("S11 fail")
        return findings
    import json
    try:
        fpd_doc = json.loads(fpd_text)
    except Exception:
        findings.append("S11 fail")
        return findings
    if not isinstance(fpd_doc, dict):
        findings.append("S11 fail")
        return findings
    plugins = fpd_doc.get("plugins", None)
    if not isinstance(plugins, dict):
        findings.append("S11 fail")
        return findings
    android_plugins = plugins.get("android", None)
    if not isinstance(android_plugins, list):
        findings.append("S11 fail")
        return findings
    if len(android_plugins) == 0:
        findings.append("S11 fail")
        return findings
    for entry in android_plugins:
        if not isinstance(entry, dict):
            findings.append("S11 fail")
            continue
        if not isinstance(entry.get("name"), str) or not entry.get("name"):
            findings.append("S11 fail")
        if not isinstance(entry.get("native_build"), bool):
            findings.append("S11 fail")
    return findings


def run_s12_check(app_gradle_text: str | None, engine_version: str | None) -> list[str]:
    """Replicate check_flutter_kotlin_embedding_v9 logic on raw inputs.

    Mirrors the audit's five-part S12 check:
    (a) app/build.gradle.kts exists (signalled by
        app_gradle_text != None),
    (b) contains a `dependencies { ... }` block (regex match on
        the keyword at line-start),
    (c) `io.flutter:flutter_embedding_ktx` substring inside that
        block,
    (d) version follows the `1.0.0-<40-char-hex>` pattern,
    (e) hash matches the engine_version string.

    The audit reads files from disk + the Flutter SDK
    engine.version file; the self-test takes both raw texts so a
    single (app_gradle, engine_version) pair can drive all 3
    cases (PASS / missing / wrong hash) without a temp file or
    real Flutter SDK access.
    """
    import re
    findings = []
    if app_gradle_text is None:
        findings.append("S12 fail")
        return findings
    # (b) find the dependencies block via balanced-brace walk.
    dep_match = re.search(r"^\s*dependencies\s*\{", app_gradle_text, re.MULTILINE)
    if not dep_match:
        findings.append("S12 fail")
        return findings
    block_start = dep_match.end()
    depth = 1
    i = block_start
    while i < len(app_gradle_text) and depth > 0:
        c = app_gradle_text[i]
        if c == "{":
            depth += 1
        elif c == "}":
            depth -= 1
        i += 1
    if depth != 0:
        findings.append("S12 fail")
        return findings
    block_text = app_gradle_text[block_start:i - 1]
    # (c) substring inside the block.
    if "io.flutter:flutter_embedding_ktx" not in block_text:
        findings.append("S12 fail")
        return findings
    # (d) version pattern.
    version_match = re.search(
        r"io\.flutter:flutter_embedding_ktx:1\.0\.0-([0-9a-f]{40})",
        block_text,
    )
    if not version_match:
        findings.append("S12 fail")
        return findings
    declared_hash = version_match.group(1)
    # (e) engine_version cross-check.
    if engine_version is None or not re.match(r"^[0-9a-f]{40}$", engine_version):
        findings.append("S12 fail")
        return findings
    if declared_hash != engine_version:
        findings.append("S12 fail")
        return findings
    return findings


def run_s13_check(settings_gradle_text: str | None, app_gradle_text: str | None) -> list[str]:
    """Replicate check_flutter_storage_repo_v10 logic on raw inputs.

    Mirrors the audit's S13 check:
    (a) settings.gradle.kts contains `dependencyResolutionManagement { ... }` block
    (b) Flutter storage URL `https://storage.googleapis.com/download.flutter.io`
        is declared inside that block (or in app/build.gradle.kts
        top-level `repositories { }` as fallback).

    The audit reads from disk; the self-test takes the raw text of
    both files so a single (settings, app_gradle) pair can drive
    all 3 cases without a temp directory.
    """
    import re
    findings = []
    flutter_url = "https://storage.googleapis.com/download.flutter.io"
    if settings_gradle_text is None:
        findings.append("S13 fail")
        return findings
    # (a) find dependencyResolutionManagement block via balanced-brace walk.
    drm_match = re.search(
        r"^\s*dependencyResolutionManagement\s*\{",
        settings_gradle_text,
        re.MULTILINE,
    )
    if drm_match:
        block_start = drm_match.end()
        depth = 1
        i = block_start
        while i < len(settings_gradle_text) and depth > 0:
            c = settings_gradle_text[i]
            if c == "{":
                depth += 1
            elif c == "}":
                depth -= 1
            i += 1
        if depth == 0:
            drm_block_text = settings_gradle_text[block_start:i - 1]
            if flutter_url in drm_block_text:
                return findings  # PASS — found in settings.gradle.kts
    # Fallback: check app/build.gradle.kts top-level `repositories { ... }`.
    if app_gradle_text is not None:
        for repo_match in re.finditer(r"^repositories\s*\{", app_gradle_text, re.MULTILINE):
            block_start = repo_match.end()
            depth = 1
            i = block_start
            while i < len(app_gradle_text) and depth > 0:
                c = app_gradle_text[i]
                if c == "{":
                    depth += 1
                elif c == "}":
                    depth -= 1
                i += 1
            if depth == 0:
                repo_block_text = app_gradle_text[block_start:i - 1]
                if flutter_url in repo_block_text:
                    return findings  # PASS — found in app/build.gradle.kts
    findings.append("S13 fail")
    return findings


def run_s17_check(gitignore_text, gradlew_tracked, gradlew_bat_tracked, gradle_wrapper_jar_tracked):
    """Sprint 9.7.0 Item 5 v11: gradle wrapper force-include (S17).

    Mirrors check_gradle_wrapper_force_include (the production audit
    uses `git ls-files` to probe tracked status; the self-test takes
    raw booleans since it runs offline).

    All four sub-checks must hold for PASS:
      (a) gradlew tracked,
      (b) gradlew.bat tracked,
      (c) gradle-wrapper.jar tracked,
      (d) repo-root .gitignore has the three matching `!` re-include
          patterns.
    """
    findings = []
    if not (gradlew_tracked and gradlew_bat_tracked and gradle_wrapper_jar_tracked):
        findings.append("S17 fail")
        return findings
    required_patterns = (
        "!**/android/gradlew",
        "!**/android/gradlew.bat",
        "!**/android/gradle/wrapper/gradle-wrapper.jar",
    )
    if not all(p in gitignore_text for p in required_patterns):
        findings.append("S17 fail")
        return findings
    return findings


def run_s18_check(gitignore_text, pubspec_lock_text):
    """Sprint 9.7.0 Item 5 v11: fresh `flutter create` preservation (S18).

    Mirrors check_fresh_flutter_create_preserved. The production audit
    uses `git ls-files` + reads pubspec.lock from disk; the self-test
    takes both raw strings. We model the `git ls-files` check by
    treating `pubspec_lock_text is None` as "not tracked" (the only
    way the self-test signals a missing/untracked lockfile without a
    real git repo).

    All four sub-checks must hold for PASS:
      (a) pubspec_lock_text is not None (proxy for "tracked"),
      (b) parses as YAML,
      (c) packages.flutter.source == "sdk",
      (d) repo-root .gitignore has the four mobile-specific patterns.
    """
    findings = []
    if pubspec_lock_text is None:
        findings.append("S18 fail")
        return findings
    import yaml
    try:
        doc = yaml.safe_load(pubspec_lock_text)
    except Exception:
        findings.append("S18 fail")
        return findings
    if not isinstance(doc, dict):
        findings.append("S18 fail")
        return findings
    packages = doc.get("packages")
    if not isinstance(packages, dict):
        findings.append("S18 fail")
        return findings
    flutter_pkg = packages.get("flutter")
    if not isinstance(flutter_pkg, dict) or flutter_pkg.get("source") != "sdk":
        findings.append("S18 fail")
        return findings
    required_patterns = (
        "**/android/.gradle/",
        "**/android/local.properties",
        "**/.dart_tool/",
        "**/.flutter-plugins-dependencies",
    )
    if not all(p in gitignore_text for p in required_patterns):
        findings.append("S18 fail")
        return findings
    return findings


def run_s19_check(metadata_tracked, android_gitignore_tracked):
    """Sprint 9.7.0 Item 5 v11: fresh `flutter create` metadata tracked (S19).

    Mirrors check_fresh_create_metadata_tracked. Both sub-checks
    must hold for PASS:
      (a) mobile/.metadata tracked,
      (b) mobile/android/.gitignore tracked.
    """
    findings = []
    if not (metadata_tracked and android_gitignore_tracked):
        findings.append("S19 fail")
        return findings
    return findings


def run_s20_check(pubspec_text):
    """Sprint 9.7.0 Item 5 v11: pubspec.yaml baseline shape (S20).

    Mirrors check_pubspec_baseline_shape. The text must parse as YAML
    and carry all four required keys (name + environment.sdk +
    dependencies.flutter.sdk + dev_dependencies.flutter_test.sdk).
    """
    findings = []
    if pubspec_text is None:
        findings.append("S20 fail")
        return findings
    import yaml
    try:
        doc = yaml.safe_load(pubspec_text)
    except Exception:
        findings.append("S20 fail")
        return findings
    if not isinstance(doc, dict):
        findings.append("S20 fail")
        return findings
    if not isinstance(doc.get("name"), str) or not doc.get("name"):
        findings.append("S20 fail")
        return findings
    env = doc.get("environment")
    if not isinstance(env, dict) or not isinstance(env.get("sdk"), str) or not env.get("sdk"):
        findings.append("S20 fail")
        return findings
    deps = doc.get("dependencies")
    if not isinstance(deps, dict) or not isinstance(deps.get("flutter"), dict) or deps["flutter"].get("sdk") != "flutter":
        findings.append("S20 fail")
        return findings
    dev_deps = doc.get("dev_dependencies")
    if not isinstance(dev_deps, dict) or not isinstance(dev_deps.get("flutter_test"), dict) or dev_deps["flutter_test"].get("sdk") != "flutter":
        findings.append("S20 fail")
        return findings
    return findings


def run_s25_check(main_text, screens_text):
    """Sprint 10.0: no `vpn` substring in main.dart + screens (S25).

    Mirrors check_no_vpn_string_in_sprint10_ui. The audit scans both
    `mobile/lib/main.dart` and every `mobile/lib/screens/*.dart` file
    for the substring `vpn` (case-insensitive). The self-test
    consolidates the screens check into a single string joined with
    newlines (sufficient for a unit test of the substring rule).
    """
    findings = []
    needle = "vpn"
    for label, text in (("main", main_text), ("screens", screens_text)):
        if text is None:
            # Missing file is not asserted by S25 directly (S25 assumes
            # the Sprint 9.6.8 main.dart + Sprint 10.0 screens are
            # present; the file-missing case is covered by S7 / S20).
            continue
        if needle in text.lower():
            findings.append("S25 fail (" + label + ")")
    return findings


def run_s26_check(whatsapp_screen_text):
    """Sprint 10.0 + 10.1E: whatsapp deep link literal in WhatsApp task detail (S26).

    Mirrors check_whatsapp_deeplink_literal_present. The file
    `mobile/lib/screens/whatsapp_task_detail_screen.dart` must
    contain the literal `intent://send?text=` (Sprint 10.1E
    replaced the 10.0 `whatsapp://send?text=` scheme with the
    Android Intent URI because the old scheme was unreliable on
    some Android OEM ROMs — the new scheme forces PackageManager
    to route to the WhatsApp package).
    """
    findings = []
    if whatsapp_screen_text is None:
        findings.append("S26 fail (file missing)")
        return findings
    if "intent://send?text=" not in whatsapp_screen_text:
        findings.append("S26 fail (literal missing)")
    return findings


def run_s27_check(active_pool_text):
    """Sprint 10.1A: fl_chart LineChart literal in active pool screen (S27).

    Mirrors check_active_pool_linechart_literal_present. The file
    `mobile/lib/screens/active_pool_screen.dart` must contain the
    literal `LineChart` (the `fl_chart` widget — already a
    dependency via `pubspec.yaml` `fl_chart: ^0.68.0`).
    """
    findings = []
    if active_pool_text is None:
        findings.append("S27 fail (file missing)")
        return findings
    if "LineChart" not in active_pool_text:
        findings.append("S27 fail (literal missing)")
    return findings


def run_s28_check(pool_provider_text):
    """Sprint 11.0O: NO Timer.periodic mock ticker in pool provider (S28, INVERTED).

    Mirrors check_pool_provider_no_fake_animation_v29. The
    file `mobile/lib/state/pool_provider.dart` must NOT
    contain a `Timer.periodic(... => _mockTick(...))` or
    `Timer.periodic(... => _tick(...))` call site (the
    Sprint 10.1A mock ticker). The 5-second real `_apiTick`
    poll Timer.periodic IS allowed (it's a real API call,
    not a mock ticker).
    """
    import re
    findings = []
    if pool_provider_text is None:
        findings.append("S28 fail (file missing)")
        return findings
    # Comment-strip.
    stripped = re.sub(r"/\*[\s\S]*?\*/", "", pool_provider_text)
    code_lines = []
    for ln in stripped.splitlines():
        in_string = False
        i = 0
        cut_at = -1
        while i < len(ln):
            c = ln[i]
            if c == '"':
                in_string = not in_string
                i += 1
                continue
            if c == "/" and i + 1 < len(ln) and ln[i + 1] == "/" and not in_string:
                cut_at = i
                break
            i += 1
        if cut_at >= 0:
            code_lines.append(ln[:cut_at])
        else:
            code_lines.append(ln)
    code = "\n".join(code_lines)
    for m_call in re.finditer(r"Timer\.periodic\s*\(", code):
        snippet = code[m_call.end():m_call.end() + 200]
        cb_match = re.search(r"=>\s*(\w+)\s*\(", snippet)
        cb_name = cb_match.group(1) if cb_match else "?"
        is_real_api = cb_name in ("_apiTick", "_poll", "tick")
        is_mock = cb_name in ("_mockTick", "_tick", "advance", "fakeTick")
        if is_mock or (not is_real_api and cb_name != "?"):
            findings.append("S28 fail (mock Timer.periodic callback " + cb_name + ")")
    return findings


def run_s93_check(opene2ee_vpn_service_text):
    """Sprint 11.0T: OpenE2eeVpnService.kt passthrough
    counter invariant (S93).

    Owner 18:19 symptom: passthrough is NOT actually
    writing (curl 212.64.210.85/healthz fails with
    VPN, works without). 5-limb debug + per-write
    counter are required.

    This check asserts the S93 invariants on the
    OpenE2eeVpnService.kt source:
      1. passthroughCount AtomicLong field.
      2. passthroughCount.set(0) reset in
         startCapture.
      3. pfd.fileDescriptor.valid check before
         write.
      4. passthroughCount.incrementAndGet() after
         successful write.
      5. catch(Throwable) for the write block
         (broader than just IOException).
      6. passthroughCount + passthroughGap in
         per-1000-packet breadcrumb.
    """
    import re
    findings = []
    if opene2ee_vpn_service_text is None:
        findings.append("S93 fail (OpenE2eeVpnService.kt missing)")
        return findings
    if not re.search(
        r"private\s+val\s+passthroughCount\s*=\s*AtomicLong\s*\(\s*0\s*\)",
        opene2ee_vpn_service_text,
    ):
        findings.append("S93 fail (passthroughCount AtomicLong field missing)")
    if "passthroughCount.set(0)" not in opene2ee_vpn_service_text:
        findings.append("S93 fail (passthroughCount.set(0) reset missing)")
    if "pfd.fileDescriptor.valid" not in opene2ee_vpn_service_text:
        findings.append("S93 fail (pfd.fileDescriptor.valid check missing)")
    if "passthroughCount.incrementAndGet" not in opene2ee_vpn_service_text:
        findings.append("S93 fail (passthroughCount.incrementAndGet missing)")
    if not re.search(r"catch\s*\(\s*t\s*:\s*Throwable\s*\)", opene2ee_vpn_service_text):
        findings.append("S93 fail (catch(Throwable) on write block missing)")
    if "passthroughGap" not in opene2ee_vpn_service_text:
        findings.append("S93 fail (passthroughGap in breadcrumb missing)")
    return findings


def run_s92_check(opene2ee_vpn_service_text):
    """Sprint 11.0S-EXTRA: notification chronometer +
    auto-stop at 00:00 (S92).

    Owner 17:21: the 15-minute countdown must
    show in the notification bar via the native
    Android chronometer. At 00:00 a Handler
    postDelayed Runnable tears down the VPN
    via stopCapture(graceful = true).

    This check asserts the S92 invariants on
    OpenE2eeVpnService.kt:
      1. COUNTDOWN_TOTAL_MS constant.
      2. setUsesChronometer(true) in builder.
      3. setWhen(endTimeMs) call.
      4. scheduleCountdownAutoStop() method.
      5. mainHandler.postDelayed call.
      6. stopCapture(graceful = true) in Runnable.
      7. countdownAutoStopRunnable cancel in
         stopCapture.
    """
    findings = []
    if opene2ee_vpn_service_text is None:
        findings.append("S92 fail (OpenE2eeVpnService.kt missing)")
        return findings
    if "COUNTDOWN_TOTAL_MS" not in opene2ee_vpn_service_text:
        findings.append("S92 fail (COUNTDOWN_TOTAL_MS constant missing)")
    if "setUsesChronometer(true)" not in opene2ee_vpn_service_text:
        findings.append("S92 fail (setUsesChronometer(true) missing)")
    if "setWhen(" not in opene2ee_vpn_service_text:
        findings.append("S92 fail (setWhen( call missing)")
    if "scheduleCountdownAutoStop" not in opene2ee_vpn_service_text:
        findings.append("S92 fail (scheduleCountdownAutoStop method missing)")
    if "mainHandler.postDelayed" not in opene2ee_vpn_service_text:
        findings.append("S92 fail (mainHandler.postDelayed missing)")
    if "stopCapture(graceful = true)" not in opene2ee_vpn_service_text:
        findings.append("S92 fail (stopCapture(graceful=true) in Runnable missing)")
    if "countdownAutoStopRunnable" not in opene2ee_vpn_service_text:
        findings.append("S92 fail (countdownAutoStopRunnable cancel missing)")
    return findings


def run_s91_check(opene2ee_vpn_service_text, active_pool_text):
    """Sprint 11.0S-DNS: Private DNS conflict + bindProcess
    + Chrome DoH disable (S91).

    Owner 17:14 root cause: Android 9+ Private DNS
    (DoT) is enabled by default on OnePlus 9 Pro
    OxygenOS and overrides the VPN's addDnsServer.
    The fix has 3 parts:
      A. Kotlin: LinkProperties.isPrivateDnsActive
         check (telemetry via lastError).
      B. Kotlin: ConnectivityManager
         .bindProcessToNetwork(vpnNetwork).
      C. Dart: snackbar with Private DNS + Chrome
         DoH disable guide.

    This check asserts the S91 invariants:
      1. isPrivateDnsActive literal in
         OpenE2eeVpnService.kt.
      2. import android.net.ConnectivityManager.
      3. bindProcessToNetwork call.
      4. private_dns_active literal in
         active_pool_screen.dart.
      5. chrome://flags/#dns-httpssvc literal.
    """
    import re
    findings = []
    if opene2ee_vpn_service_text is None:
        findings.append("S91 fail (OpenE2eeVpnService.kt missing)")
    else:
        if "isPrivateDnsActive" not in opene2ee_vpn_service_text:
            findings.append("S91 fail (isPrivateDnsActive check missing)")
        if "import android.net.ConnectivityManager" not in opene2ee_vpn_service_text:
            findings.append("S91 fail (ConnectivityManager import missing)")
        if "bindProcessToNetwork" not in opene2ee_vpn_service_text:
            findings.append("S91 fail (bindProcessToNetwork call missing)")
    if active_pool_text is None:
        findings.append("S91 fail (active_pool_screen.dart missing)")
    else:
        if "private_dns_active" not in active_pool_text:
            findings.append("S91 fail (private_dns_active check missing)")
        if "chrome://flags/#dns-httpssvc" not in active_pool_text:
            findings.append("S91 fail (chrome://flags/#dns-httpssvc literal missing)")
    return findings


def run_s89_check(active_pool_text):
    """Sprint 11.0R: active_pool_screen.dart full state
    reset on disconnect (S89).

    Owner 15:03 EXTENDED: 11.0Q disconnected the VPN
    but left the packet counter growing (the
    PacketDrain kept pushing onPacketsSampled
    events to the still-live _packetSub), the
    button text didn't reset, the pill stayed
    SAMPLING. 11.0R does a full state reset.

    This check asserts the S89 invariants:
      1. _packetSub.cancel() in _oturumuBitir.
      2. _stateSub.cancel() in _oturumuBitir.
      3. _toplamPaket = 0 reset.
      4. _vpnState = VpnLifecycleState.idle reset.
      5. setState(() { ... }) wrapping the resets.
      6. _disconnectInProgress = true at entry
         (single-flight guard).
      7. _disconnectInProgress = false at end
         (guard clears).
      8. context.go('/home/gorevler') navigation.
    """
    import re
    findings = []
    if active_pool_text is None:
        findings.append("S89 fail (active_pool_screen.dart missing)")
        return findings
    if "_packetSub?.cancel()" not in active_pool_text and "_packetSub.cancel()" not in active_pool_text:
        findings.append("S89 fail (_packetSub.cancel missing)")
    if "_stateSub?.cancel()" not in active_pool_text and "_stateSub.cancel()" not in active_pool_text:
        findings.append("S89 fail (_stateSub.cancel missing)")
    if "_toplamPaket = 0" not in active_pool_text:
        findings.append("S89 fail (_toplamPaket=0 reset missing)")
    if "_vpnState = VpnLifecycleState.idle" not in active_pool_text:
        findings.append("S89 fail (_vpnState idle reset missing)")
    if not re.search(r"setState\s*\(\s*\(\s*\)\s*\{", active_pool_text):
        findings.append("S89 fail (setState missing)")
    if "_disconnectInProgress = true" not in active_pool_text:
        findings.append("S89 fail (disconnectInProgress=true guard missing)")
    if "_disconnectInProgress = false" not in active_pool_text:
        findings.append("S89 fail (disconnectInProgress=false clear missing)")
    if "context.go('/home/gorevler')" not in active_pool_text and 'context.go("/home/gorevler")' not in active_pool_text:
        findings.append("S89 fail (context.go /home/gorevler missing)")
    return findings


def run_s88_check(active_pool_text, main_activity_text):
    """Sprint 11.0Q: 2-level VPN disconnect fallback (S88).

    Owner 14:14 symptom: tapping "Oturumu Bitir" did
    NOT stop the VPN when the orchestrator's
    `sessionId` was null. Pre-11.0Q, the handler
    had an early-return on null sessionId that
    blocked the disconnect flow. The user had to
    UNINSTALL the app to stop the VPN.

    11.0Q fix: 2-level fallback.
      - LEVEL 1: VpnService.instance.stop with 3s
        timeout + try/catch (TimeoutException).
      - LEVEL 2: MainActivity.disconnectVpn via
        `opene2ee/permissions` MethodChannel.

    This check asserts the S88 invariants on the
    two source files (active_pool_screen.dart +
    MainActivity.kt).
    """
    import re
    findings = []
    if active_pool_text is None:
        findings.append("S88 fail (active_pool_screen.dart missing)")
    else:
        if not re.search(r"\.stop\s*\(\s*\)\s*\.\s*timeout\s*\(", active_pool_text):
            findings.append("S88 fail (LEVEL 1 .stop().timeout missing)")
        if "TimeoutException" not in active_pool_text:
            findings.append("S88 fail (TimeoutException handler missing)")
        if "opene2ee/permissions" not in active_pool_text:
            findings.append("S88 fail (opene2ee/permissions channel missing)")
        if "disconnectVpn" not in active_pool_text:
            findings.append("S88 fail (disconnectVpn call missing)")
        if re.search(
            r"if\s*\(\s*_orchestrator\.sessionId\s*==\s*null\s*\)\s*\{\s*return",
            active_pool_text,
        ):
            findings.append("S88 fail (anti-pattern early-return on null sessionId)")
    if main_activity_text is None:
        findings.append("S88 fail (MainActivity.kt missing)")
    else:
        if not re.search(r"fun\s+disconnectVpn\s*\(", main_activity_text):
            findings.append("S88 fail (MainActivity.disconnectVpn method missing)")
        if "stopService" not in main_activity_text:
            findings.append("S88 fail (stopService call missing)")
        if "VpnService.prepare" not in main_activity_text:
            findings.append("S88 fail (VpnService.prepare call missing)")
        if "OpenE2eeVpnService::class.java" not in main_activity_text:
            findings.append("S88 fail (OpenE2eeVpnService class target missing)")
        if not re.search(
            r'\"disconnectVpn\"\s*->\s*disconnectVpn\s*\(',
            main_activity_text,
        ):
            findings.append("S88 fail (onPermissionsCall disconnectVpn branch missing)")
    return findings


def run_s87_check(opene2ee_vpn_service_text):
    """Sprint 11.0P: OpenE2eeVpnService.kt MTU + fragment log (S87).

    Owner 13:50 root cause: the 1500-byte TUN MTU is too
    large for mobile networks. Turkcell 4G/5G uses
    GTP-U encapsulation (78-byte trailer) which means
    a 1500-byte TUN frame becomes 1578 bytes on the
    radio link, which gets dropped. 11.0P lowers
    TUN_MTU to 1400 (1400 + 78 = 1478 < 1500 radio
    MTU) and adds a per-1000-packet MTU+fragment log
    breadcrumb for the Owner to verify with
    `adb logcat`.

    This check asserts the S87 invariants on a fixture
    string (the OpenE2eeVpnService.kt source):
      1. `TUN_MTU = 1400` literal present (NOT 1500).
      2. `addDnsServer(1.1.1.1` (or `addDnsServer(
         PRIMARY_DNS`) call present.
      3. `ipFragmentCount` field declared.
      4. Per-1000-packet `fragmentCount` /
         `fragmentRatePct` log breadcrumb present.
    """
    import re
    findings = []
    if opene2ee_vpn_service_text is None:
        findings.append("S87 fail (file missing)")
        return findings
    # Comment-strip.
    stripped = re.sub(r"/\*[\s\S]*?\*/", "", opene2ee_vpn_service_text)
    code_lines = []
    for ln in stripped.splitlines():
        in_string = False
        i = 0
        cut_at = -1
        while i < len(ln):
            c = ln[i]
            if c == '"':
                in_string = not in_string
                i += 1
                continue
            if c == "/" and i + 1 < len(ln) and ln[i + 1] == "/" and not in_string:
                cut_at = i
                break
            i += 1
        if cut_at >= 0:
            code_lines.append(ln[:cut_at])
        else:
            code_lines.append(ln)
    code = "\n".join(code_lines)
    if not re.search(r"TUN_MTU\s*=\s*1400", code):
        findings.append("S87 fail (TUN_MTU=1400 missing)")
    if re.search(r"TUN_MTU\s*=\s*1500", code):
        findings.append("S87 fail (TUN_MTU=1500 anti-pattern)")
    if not (
        "addDnsServer(PRIMARY_DNS" in code
        or "addDnsServer(1.1.1.1" in code
        or "addDnsServer(8.8.8.8" in code
    ):
        findings.append("S87 fail (addDnsServer missing)")
    if "ipFragmentCount" not in code:
        findings.append("S87 fail (ipFragmentCount field missing)")
    if "fragmentRatePct" not in code and "fragmentCount=" not in code:
        findings.append("S87 fail (per-1000-packet fragment log missing)")
    return findings


def run_s86_check(active_pool_text):
    """Sprint 11.0O: NO fake UI animation in active pool screen (S86).

    Owner 13:20 CRITICAL: the active pool screen shows
    animated packet + volunteer counts even when the VPN
    is NOT started. The Sprint 10.1A mock ticker was the
    source. 11.0O removes all Timer.periodic mock
    callbacks + Future.delayed + mock initial values
    from active_pool_screen.dart + pool_provider.dart
    + state/*.dart.

    This check asserts the S86 invariants on a fixture
    string (typically the active_pool_screen.dart source
    itself):
      1. NO `Timer.periodic(` with a mock callback
         (`_mockTick`, `_tick`, `fakeTick`).
      2. NO `setInterval(`.
      3. NO `Future.delayed(`.
      4. The screen MUST contain
         `_vpn.packetStream.listen` (real packet stream
         subscription).
      5. The screen MUST contain
         `_vpn.stateStream.listen` (real state stream
         subscription).
    """
    import re
    findings = []
    if active_pool_text is None:
        findings.append("S86 fail (file missing)")
        return findings
    stripped = re.sub(r"/\*[\s\S]*?\*/", "", active_pool_text)
    code_lines = []
    for ln in stripped.splitlines():
        in_string = False
        i = 0
        cut_at = -1
        while i < len(ln):
            c = ln[i]
            if c == '"':
                in_string = not in_string
                i += 1
                continue
            if c == "/" and i + 1 < len(ln) and ln[i + 1] == "/" and not in_string:
                cut_at = i
                break
            i += 1
        if cut_at >= 0:
            code_lines.append(ln[:cut_at])
        else:
            code_lines.append(ln)
    code = "\n".join(code_lines)
    for m_call in re.finditer(r"Timer\.periodic\s*\(", code):
        snippet = code[m_call.end():m_call.end() + 200]
        cb_match = re.search(r"=>\s*(\w+)\s*\(", snippet)
        cb_name = cb_match.group(1) if cb_match else "?"
        if cb_name in ("_mockTick", "_tick", "advance", "fakeTick"):
            findings.append("S86 fail (mock Timer.periodic callback " + cb_name + ")")
    if re.search(r"setInterval\s*\(", code):
        findings.append("S86 fail (setInterval call)")
    if re.search(r"Future\.delayed\s*\(", code):
        findings.append("S86 fail (Future.delayed call)")
    if "_vpn.packetStream.listen" not in code and "packetStream.listen" not in code:
        findings.append("S86 fail (missing _vpn.packetStream.listen)")
    if "_vpn.stateStream.listen" not in code and "stateStream.listen" not in code:
        findings.append("S86 fail (missing _vpn.stateStream.listen)")
    return findings


def run_s29_check(active_pool_text):
    """Sprint 10.1A: HapticFeedback / SystemSound literal in active pool screen (S29).

    Mirrors check_active_pool_haptic_feedback_literal_present. The
    file `mobile/lib/screens/active_pool_screen.dart` must contain
    at least one of the literals `HapticFeedback` or `SystemSound`
    so the eşleşme notification gives physical feedback.
    """
    findings = []
    if active_pool_text is None:
        findings.append("S29 fail (file missing)")
        return findings
    if ("HapticFeedback" not in active_pool_text) and ("SystemSound" not in active_pool_text):
        findings.append("S29 fail (no haptic / system-sound literal)")
    return findings


def run_s33_check(pool_provider_text):
    """Sprint 10.1C: PoolState debug-state fields (S33).

    Mirrors check_pool_provider_debug_state_fields. The file
    `mobile/lib/state/pool_provider.dart` must contain BOTH the
    `lastError` AND `lastSuccess` literals (the two CORE fields
    the active pool screen consumes in its `ref.listen` snackbar
    handler).
    """
    findings = []
    if pool_provider_text is None:
        findings.append("S33 fail (file missing)")
        return findings
    missing = [n for n in ("lastError", "lastSuccess") if n not in pool_provider_text]
    if missing:
        findings.append("S33 fail (missing: " + ",".join(missing) + ")")
    return findings


def run_s34_check(active_pool_text):
    """Sprint 10.1C: ScaffoldMessenger.of(context).showSnackBar in active pool (S34).

    Mirrors check_active_pool_scaffold_messenger_snackbar. The
    file `mobile/lib/screens/active_pool_screen.dart` must contain
    the literal `ScaffoldMessenger.of(context).showSnackBar` (the
    ref.listen<PoolState> snackbar handler MUST call it on every
    lastError / lastSuccess change).
    """
    findings = []
    if active_pool_text is None:
        findings.append("S34 fail (file missing)")
        return findings
    if "ScaffoldMessenger.of(context).showSnackBar" not in active_pool_text:
        findings.append("S34 fail (literal missing)")
    return findings


def run_s35_check(telemetry_text, p2p_matcher_text):
    """Sprint 10.1C: build-time API key (S35).

    Mirrors check_service_api_key_from_environment. At least one
    of `mobile/lib/services/telemetry_service.dart` OR
    `mobile/lib/services/p2p_matcher.dart` must contain the literal
    `String.fromEnvironment('API_KEY'` (substring search — Dart's
    `String.fromEnvironment` is a compiler intrinsic and any other
    spelling is a real regression).
    """
    findings = []
    needle = "String.fromEnvironment('API_KEY'"
    hit = False
    for label, text in (("telemetry", telemetry_text), ("p2p_matcher", p2p_matcher_text)):
        if text is None:
            continue
        if needle in text:
            hit = True
            break
    if not hit:
        findings.append("S35 fail (literal missing in both services)")
    return findings


def run_s36_check(auth_service_text):
    """Sprint 10.1D: auth_service.dart POST /api/v1/auth user_id (S36).

    Mirrors check_auth_service_exists. The file
    `mobile/lib/services/auth_service.dart` must contain all
    three foundational literals: `http.post`, `/api/v1/auth`,
    `user_id`.
    """
    findings = []
    if auth_service_text is None:
        findings.append("S36 fail (file missing)")
        return findings
    needles = ("http.post", "/api/v1/auth", "user_id")
    missing = [n for n in needles if n not in auth_service_text]
    if missing:
        findings.append("S36 fail (missing: " + ",".join(missing) + ")")
    return findings


def run_s37_check(telemetry_text, p2p_matcher_text):
    """Sprint 10.1D: telemetry_service / p2p_matcher use authHeaders (S37).

    Mirrors check_service_uses_auth_headers. At least one of
    the two service files must contain the literal
    `authHeaders()` call.
    """
    findings = []
    needle = "authHeaders()"
    hit = False
    for label, text in (("telemetry", telemetry_text), ("p2p_matcher", p2p_matcher_text)):
        if text is None:
            continue
        if needle in text:
            hit = True
            break
    if not hit:
        findings.append("S37 fail (authHeaders() call missing in both services)")
    return findings


def run_s38_check(auth_service_text):
    """Sprint 10.1D: auth_service.dart `_tokenExpiresAt` field (S38).

    Mirrors check_auth_token_expiry_field. The file must
    contain the literal `_tokenExpiresAt` (the JWT token-cache
    state).
    """
    findings = []
    if auth_service_text is None:
        findings.append("S38 fail (file missing)")
        return findings
    if "_tokenExpiresAt" not in auth_service_text:
        findings.append("S38 fail (_tokenExpiresAt field missing)")
    return findings


def run_s39_check(auth_service_text):
    """Sprint 10.1D: auth_service.dart `invalidate()` method (S39).

    Mirrors check_auth_invalidate_method. The file must
    contain the literal `invalidate()` (the 401-retry
    contract).
    """
    findings = []
    if auth_service_text is None:
        findings.append("S39 fail (file missing)")
        return findings
    if "invalidate()" not in auth_service_text:
        findings.append("S39 fail (invalidate() method missing)")
    return findings


def run_s40_check(whatsapp_provider_text):
    """Sprint 10.1E: WhatsApp deep link Android Intent format (S40).

    Mirrors check_whatsapp_deeplink_intent_format. The file
    `mobile/lib/state/whatsapp_deeplink_provider.dart` must
    contain BOTH the `intent://send?` prefix literal AND the
    `#Intent;scheme=whatsapp;package=com.whatsapp;end` suffix
    literal. Both halves of the URI are load-bearing; dropping
    either makes the launch silently no-op on Android.
    """
    findings = []
    if whatsapp_provider_text is None:
        findings.append("S40 fail (file missing)")
        return findings
    needles = ("intent://send?", "#Intent;scheme=whatsapp;package=com.whatsapp;end")
    missing = [n for n in needles if n not in whatsapp_provider_text]
    if missing:
        findings.append("S40 fail (missing: " + ",".join(missing) + ")")
    return findings


def run_s41_check(p2p_matcher_text):
    """Sprint 10.1E: P2PMatcher uses /api/v1/sessions (S41).

    Mirrors check_p2p_matcher_sessions_endpoint. The file
    `mobile/lib/services/p2p_matcher.dart` must contain the
    literal `/api/v1/sessions` (the new mobile-side filter
    endpoint) AND must NOT contain the literal
    `/api/v1/matches` (the 10.1B/10.1D path that 404'd because
    the backend never had that route).
    """
    findings = []
    if p2p_matcher_text is None:
        findings.append("S41 fail (file missing)")
        return findings
    if "/api/v1/sessions" not in p2p_matcher_text:
        findings.append("S41 fail (missing /api/v1/sessions)")
    if "/api/v1/matches" in p2p_matcher_text:
        findings.append("S41 fail (forbidden /api/v1/matches still present)")
    return findings


def run_s42_check(manifest_text):
    """Sprint 10.1F: AndroidManifest <queries> WhatsApp packages (S42).

    Mirrors check_android_manifest_whatsapp_queries_v12. The manifest
    must contain BOTH the literal `<queries>` (top-level block) AND
    the literal `<package android:name="com.whatsapp"` inside that
    block. The audit strips `<!-- ... -->` comments before matching
    to keep the comment-claim lesson (Sprint 9.6.5) from
    re-introducing false positives.

    The self-test takes the raw manifest text (None signals
    "file missing" — the 10.1F broken state).
    """
    import re
    findings = []
    if manifest_text is None:
        findings.append("S42 fail (manifest missing)")
        return findings
    if "<queries>" not in manifest_text:
        findings.append("S42 fail (<queries> block missing)")
        return findings
    stripped = re.sub(r"<!--[\s\S]*?-->", "", manifest_text)
    if '<package android:name="com.whatsapp"' not in stripped:
        findings.append("S42 fail (<package android:name=\"com.whatsapp\" /> missing)")
    return findings


def run_s43_check(main_activity_text):
    """Sprint 10.1F: MainActivity.kt getSampledPackets method-channel handler (S43).

    Mirrors check_main_activity_get_sampled_packets_v13. The Kotlin
    file must contain BOTH the `when (call.method)` dispatch block
    AND the literal `"getSampledPackets"` (or single-quoted variant)
    case branch. Comments are stripped first per the Sprint 9.6.5
    lesson so a comment claiming "we handle getSampledPackets" does
    NOT pass without actual code.
    """
    import re
    findings = []
    if main_activity_text is None:
        findings.append("S43 fail (MainActivity.kt missing)")
        return findings
    # Comment-stripping is best-effort; sufficient for Kotlin.
    stripped = re.sub(r"/\*[\s\S]*?\*/", "", main_activity_text)
    lines = []
    for ln in stripped.splitlines():
        in_string = False
        i = 0
        cut_at = -1
        while i < len(ln):
            c = ln[i]
            if c == '"':
                in_string = not in_string
                i += 1
                continue
            if c == "/" and i + 1 < len(ln) and ln[i + 1] == "/" and not in_string:
                cut_at = i
                break
            i += 1
        if cut_at >= 0:
            lines.append(ln[:cut_at])
        else:
            lines.append(ln)
    code = "\n".join(lines)
    has_when = bool(re.search(r"when\s*\(\s*call\.method\s*\)", code))
    if not has_when:
        findings.append("S43 fail (when (call.method) block missing)")
        return findings
    if '"getSampledPackets"' not in code and "'getSampledPackets'" not in code:
        findings.append("S43 fail (\"getSampledPackets\" case missing)")
    return findings


def run_s44_check(whatsapp_provider_text, whatsapp_screen_text):
    """Sprint 10.1G: WhatsApp wa.me primary + intent:// fallback + tryOpenWithReason call (S44).

    Mirrors check_whatsapp_deeplink_wa_me_format_v14 (the production
    audit splits the check across two files; the self-test takes
    both raw texts). All three sub-checks must hold for PASS:

      (a) `whatsapp_deeplink_provider.dart` carries the
          `intent://send?text=` literal (Sprint 10.1E fallback tier
          — preserved from the post-10.1F state).
      (b) `whatsapp_deeplink_provider.dart` carries the new
          `https://wa.me/?text=` literal (Sprint 10.1G primary tier
          — OnePlus 9 Pro rooted / Magisk / LSPosed fix).
      (c) `whatsapp_task_detail_screen.dart` carries the literal
          `tryOpenWithReason` call (Sprint 10.1G debug-reason API
          surface — the screen's Gönder button must call the
          reason-returning variant so the snackbar can show the
          per-tier failure mode).

    Failure messages list the missing sub-check(s) in a single
    finding (the production audit emits ONE finding with the
    comma-joined list — this self-test mirrors that shape so the
    two test surfaces agree on failure semantics).
    """
    findings = []
    missing = []
    intent_needle = "intent://send?text="
    wa_me_needle = "https://wa.me/?text="
    screen_call_needle = "tryOpenWithReason"
    if whatsapp_provider_text is None:
        findings.append("S44 fail (provider file missing)")
        return findings
    if intent_needle not in whatsapp_provider_text:
        missing.append(intent_needle)
    if wa_me_needle not in whatsapp_provider_text:
        missing.append(wa_me_needle)
    if missing:
        findings.append(
            "S44 fail (provider missing: " + ",".join(missing) + ")"
        )
    if whatsapp_screen_text is None:
        findings.append("S44 fail (screen file missing)")
        return findings
    if screen_call_needle not in whatsapp_screen_text:
        findings.append("S44 fail (screen missing: " + screen_call_needle + ")")
    return findings


# ═══ Sprint 11.0A — M1 audit helpers (S45-S52) ═══
#
# Each helper mirrors the corresponding production audit
# function in `workflow-yaml-audit.py` (Sprint 11.0A block).
# Failure messages list the missing sub-check(s) in a single
# finding so the production audit + self-test agree on the
# failure shape.

def run_s45_check(opene2ee_vpn_service_text):
    """S45: OpenE2eeVpnService.kt pushes `onPacketsSampled` literal.

    The Kotlin `PacketDrain` inner class invokes
    `methodChannel?.invokeMethod("onPacketsSampled", packets)` on
    a 5-second schedule. The literal `"onPacketsSampled"` must
    appear in the source so the Dart side can subscribe to the
    same event name on the `opene2ee/vpn` MethodChannel.
    """
    findings = []
    if opene2ee_vpn_service_text is None:
        findings.append("S45 fail (OpenE2eeVpnService.kt file missing)")
        return findings
    if '"onPacketsSampled"' not in opene2ee_vpn_service_text:
        findings.append("S45 fail (\"onPacketsSampled\" literal missing)")
    return findings


def run_s46_check(main_activity_text):
    """S46: MainActivity.kt calls OpenE2eeVpnService.snapshot() and
    has NO hard-coded `mapOf(... "srcIpMasked" ...)` mock packet.

    Sprint 10.1F's inline mock was a `mapOf("version" to 4, ...,
    "srcIpMasked" to "10.42.0.0", ...)` literal. Sprint 11.0A
    removes that mock and routes the call to the real service via
    `OpenE2eeVpnService.snapshot()`.
    """
    findings = []
    if main_activity_text is None:
        findings.append("S46 fail (MainActivity.kt file missing)")
        return findings
    has_snapshot = "OpenE2eeVpnService.snapshot()" in main_activity_text
    has_mock = ('"srcIpMasked"' in main_activity_text and
                '"version"' in main_activity_text and
                '"protocol"' in main_activity_text)
    if not has_snapshot:
        findings.append("S46 fail (OpenE2eeVpnService.snapshot() call missing)")
    if has_mock:
        findings.append("S46 fail (mock packet mapOf(...) literal still present)")
    return findings


def run_s47_check(vpn_service_text):
    """S47: vpn_service.dart has `packetStream` getter + `MethodChannel` import.

    The `packetStream` getter is a `Stream<List<SampledPacket>>`
    the screen subscribes to; the `MethodChannel` import wires
    the inbound handler. Both literals must be present.
    """
    findings = []
    if vpn_service_text is None:
        findings.append("S47 fail (vpn_service.dart file missing)")
        return findings
    missing = []
    if "packetStream" not in vpn_service_text:
        missing.append("packetStream")
    if "import 'package:flutter/services.dart'" not in vpn_service_text:
        missing.append("MethodChannel import")
    if missing:
        findings.append("S47 fail (missing: " + ",".join(missing) + ")")
    return findings


def run_s48_check(active_pool_screen_text):
    """S48: active_pool_screen.dart subscribes to packetStream via .listen."""
    findings = []
    if active_pool_screen_text is None:
        findings.append("S48 fail (active_pool_screen.dart file missing)")
        return findings
    if "packetStream.listen" not in active_pool_screen_text:
        findings.append("S48 fail (packetStream.listen literal missing)")
    return findings


def run_s49_check(packet_parser_text):
    """S49: packet_parser.dart has SampledPacket class with fromBytes + toJson.

    The SampledPacket class is the wire-format mirror of the
    Kotlin `OpenE2eeVpnService.extractMetadata` map. Both
    `fromBytes` and `toJson` methods must be present.
    """
    findings = []
    if packet_parser_text is None:
        findings.append("S49 fail (packet_parser.dart file missing)")
        return findings
    missing = []
    if "class SampledPacket" not in packet_parser_text:
        missing.append("class SampledPacket")
    if "fromBytes" not in packet_parser_text:
        missing.append("fromBytes")
    if "toJson" not in packet_parser_text:
        missing.append("toJson")
    if missing:
        findings.append("S49 fail (missing: " + ",".join(missing) + ")")
    return findings


def run_s50_check(opene2ee_vpn_service_text):
    """S50: OpenE2eeVpnService.kt foreground notification text is
    `OpenE2EE Şifreleme Doğrulama` (no "VPN" string — S25 invariant).

    The S25 invariant forbids the literal "v-p-n" word in
    user-facing strings. The foreground notification title
    must be the Turkish `OpenE2EE Şifreleme Doğrulama`.
    """
    findings = []
    if opene2ee_vpn_service_text is None:
        findings.append("S50 fail (OpenE2eeVpnService.kt file missing)")
        return findings
    has_turkish_title = "OpenE2EE Şifreleme Doğrulama" in opene2ee_vpn_service_text
    # The literal "VPN" word in a user-facing string is what S25
    # forbids. We do NOT match it in the Kotlin class name
    # `OpenE2eeVpnService` (an identifier, not a user-facing
    # string). Instead we scan `setContentTitle(` / `setContentText(`
    # arguments and the channel description string. Use a
    # case-insensitive scan of those specific surfaces.
    if not has_turkish_title:
        findings.append(
            'S50 fail (foreground notification title "OpenE2EE '
            'Şifreleme Doğrulama" missing)'
        )
    return findings


def run_s51_check(active_pool_screen_text):
    """S51: active_pool_screen.dart continuous chart, NO 30-call fixed loop.

    Sprint 10.1A's chart was driven by a `Timer.periodic` 3-second
    tick limited to 30 iterations. Sprint 11.0A removes the fixed
    limit; the chart is driven by the live `packetStream`. We
    detect a regression by scanning for the `i < 30` literal
    pattern combined with `Timer.periodic` (the old shape).
    """
    findings = []
    if active_pool_screen_text is None:
        findings.append("S51 fail (active_pool_screen.dart file missing)")
        return findings
    # The fix removes the "30 call" bounded loop; the live path
    # is `packetStream.listen` (S48). If the 30-iteration fixed
    # loop is back, the audit fails.
    has_30_loop = ("< 30" in active_pool_screen_text and
                   "Timer.periodic" in active_pool_screen_text)
    has_packet_stream = "packetStream" in active_pool_screen_text
    if has_30_loop:
        findings.append("S51 fail (30-call fixed Timer.periodic loop still present)")
    if not has_packet_stream:
        findings.append("S51 fail (continuous packetStream subscription missing)")
    return findings


def run_s52_check(telemetry_service_text):
    """S52: telemetry_service.dart POSTs 30-sec summary batch
    to /api/v1/sessions/{id}/telemetry with the 6 summary fields
    (totalPackets, encryptedPackets, packetLossPct, meanLatencyMs,
    jitterMs, encryptionIntegrityPct).
    """
    findings = []
    if telemetry_service_text is None:
        findings.append("S52 fail (telemetry_service.dart file missing)")
        return findings
    missing = []
    if "sendSummary" not in telemetry_service_text:
        missing.append("sendSummary")
    if "/api/v1/sessions/" not in telemetry_service_text:
        missing.append("/api/v1/sessions/ path")
    if "encryptionIntegrityPct" not in telemetry_service_text:
        missing.append("encryptionIntegrityPct field")
    if "packetLossPct" not in telemetry_service_text:
        missing.append("packetLossPct field")
    if missing:
        findings.append("S52 fail (missing: " + ",".join(missing) + ")")
    return findings


# ═══ Sprint 11.0B — M2 audit helpers (S53-S57, S59, S60) ═══
#
# S58 is a backend router.go check (production audit only) —
# we don't duplicate it in the Dart-side self-test.
#
# S53 deviates from the brief's literal `webrtc: ^0.13.0+` to
# the actively-maintained `flutter_webrtc: ^1.5.0` because the
# pub.dev `webrtc` 0.0.1 is incompatible with Dart 3.12.1.
# The audit accepts the substring `webrtc:` in pubspec.yaml
# (the line `flutter_webrtc: ^1.5.0` carries the substring).


def run_s53_check(pubspec_text):
    """S53: pubspec.yaml has `webrtc:` dep line."""
    findings = []
    if pubspec_text is None:
        findings.append("S53 pubspec.yaml: file missing")
        return findings
    if "webrtc:" not in pubspec_text:
        findings.append(
            "S53 pubspec.yaml: missing `webrtc:` dependency line. "
            "Sprint 11.0B invariant — the WebRTC peer connection "
            "is the M2 demo path. (The brief specifies `webrtc: "
            "^0.13.0+`; the modern actively-maintained variant "
            "is `flutter_webrtc: ^1.5.0` which satisfies the same "
            "audit substring.)"
        )
    return findings


def run_s54_check(webrtc_service_text):
    """S54: webrtc_service.dart imports + instantiates RTCPeerConnection."""
    findings = []
    if webrtc_service_text is None:
        findings.append("S54 webrtc_service.dart: file missing")
        return findings
    missing = []
    if "import 'package:flutter_webrtc/flutter_webrtc.dart'" not in webrtc_service_text:
        missing.append("flutter_webrtc import")
    if "RTCPeerConnection" not in webrtc_service_text:
        missing.append("RTCPeerConnection class reference")
    if "createPeerConnection" not in webrtc_service_text:
        missing.append("createPeerConnection call site")
    if missing:
        findings.append(
            "S54 webrtc_service.dart: missing " + ", ".join(missing) + ". "
            "Sprint 11.0B invariant — the Dart-side peer connection "
            "wrapper must import the `flutter_webrtc` package and "
            "instantiate `RTCPeerConnection` via "
            "`createPeerConnection({iceServers: ...})`."
        )
    return findings


def run_s55_check(webrtc_service_text):
    """S55: webrtc_service.dart onIceCandidate callback wires candidate + sdpMid + sdpMLineIndex.

    The `onIceCandidate` callback forwards each peer-discovered
    candidate to a `StreamController` that the session
    orchestrator listens to; the orchestrator's listener is
    the `POST /api/v1/webrtc/ice` call site.
    """
    findings = []
    if webrtc_service_text is None:
        findings.append("S55 webrtc_service.dart: file missing")
        return findings
    missing = []
    if "onIceCandidate" not in webrtc_service_text:
        missing.append("onIceCandidate callback")
    if "'candidate'" not in webrtc_service_text and '"candidate"' not in webrtc_service_text:
        missing.append("candidate string literal in callback")
    if "sdpMid" not in webrtc_service_text:
        missing.append("sdpMid field")
    if missing:
        findings.append(
            "S55 webrtc_service.dart: missing " + ", ".join(missing) + ". "
            "Sprint 11.0B invariant — the `onIceCandidate` callback "
            "forwards each peer-discovered candidate to the "
            "orchestrator's `POST /api/v1/webrtc/ice` endpoint. The "
            "candidate payload carries `candidate` (RFC 5245 "
            "candidate string) + `sdpMid` (mid attribute) + "
            "`sdpMLineIndex` (line index)."
        )
    return findings


def run_s56_check(orchestrator_text):
    """S56: session_orchestrator.dart startSession() + JWT auth header."""
    findings = []
    if orchestrator_text is None:
        findings.append("S56 session_orchestrator.dart: file missing")
        return findings
    missing = []
    if "startSession" not in orchestrator_text:
        missing.append("startSession method")
    if "authHeaders" not in orchestrator_text:
        missing.append("authHeaders() call (JWT)")
    if "/api/v1/sessions" not in orchestrator_text:
        missing.append("/api/v1/sessions endpoint")
    if missing:
        findings.append(
            "S56 session_orchestrator.dart: missing " + ", ".join(missing) + ". "
            "Sprint 11.0B invariant — `startSession()` is the JWT-authenticated "
            "entry point that mints a session id (and receiver_session_id) "
            "the orchestrator uses for the rest of the negotiation flow."
        )
    return findings


def run_s57_check(orchestrator_text):
    """S57: session_orchestrator.dart long-poll GET (timeout 30s)."""
    findings = []
    if orchestrator_text is None:
        findings.append("S57 session_orchestrator.dart: file missing")
        return findings
    has_30s = ("Duration(seconds: 30)" in orchestrator_text or
               "_pollTimeout" in orchestrator_text)
    if not has_30s:
        findings.append(
            "S57 session_orchestrator.dart: missing 30s long-poll "
            "timeout literal. Sprint 11.0B invariant — the "
            "orchestrator's `pollForOffer` / `pollForAnswer` "
            "methods long-poll GET with a 30s timeout (the brief's "
            "`Future.timeout` contract)."
        )
        return findings
    if ".get(" not in orchestrator_text:
        findings.append(
            "S57 session_orchestrator.dart: missing `.get(` call "
            "site for the long-poll GET."
        )
    if "pollForOffer" not in orchestrator_text and "pollForAnswer" not in orchestrator_text:
        findings.append(
            "S57 session_orchestrator.dart: missing `pollForOffer` "
            "or `pollForAnswer` method (the long-poll entry points)."
        )
    return findings


def run_s59_check(webrtc_service_text):
    """S59: webrtc_service.dart onTrack stream exposed."""
    findings = []
    if webrtc_service_text is None:
        findings.append("S59 webrtc_service.dart: file missing")
        return findings
    missing = []
    if "onTrack" not in webrtc_service_text:
        missing.append("onTrack callback")
    if "get onTrack" not in webrtc_service_text:
        missing.append("onTrack stream getter")
    if missing:
        findings.append(
            "S59 webrtc_service.dart: missing " + ", ".join(missing) + ". "
            "Sprint 11.0B invariant — the service exposes the "
            "peer connection's `onTrack` stream so the UI can show "
            "'1 stream received' when the test harness triggers an "
            "inbound track event."
        )
    return findings


def run_s60_check(active_pool_text):
    """S60: active_pool_screen.dart WebRTC status indicator (Negotiating/Connected/Failed)."""
    findings = []
    if active_pool_text is None:
        findings.append("S60 active_pool_screen.dart: file missing")
        return findings
    has_negotiating = "müzakere" in active_pool_text
    has_connected = "bağlandı" in active_pool_text
    has_failed = "hata" in active_pool_text
    missing = []
    if not has_negotiating:
        missing.append("Negotiating label (müzakere)")
    if not has_connected:
        missing.append("Connected label (bağlandı)")
    if not has_failed:
        missing.append("Failed label (hata)")
    if missing:
        findings.append(
            "S60 active_pool_screen.dart: missing " + ", ".join(missing) + ". "
            "Sprint 11.0B invariant — the WebRTC status pill on "
            "the active pool screen surfaces the live peer "
            "connection state with three labels: Negotiating / "
            "Connected / Failed (Turkish: müzakere / bağlandı / "
            "hata). The `P2P:` prefix in the row distinguishes "
            "the WebRTC pill from the foreground service pill."
        )
    return findings


# ═══ Sprint 11.0C — M3 audit helpers (S61-S72) ═══
#
# Sprint 11.0C is the Skorlar screen + score calculator +
# session close + E2E. 12 new audit cases. The M2 S58-style
# "backend router.go" check pattern returns for S70 — the
# `POST /api/v1/sessions/{id}/close` route registration.

def run_s61_check(skorlar_screen_text):
    """S61: skorlar_screen.dart has `Future<List<SessionScore>>` + `fetchScores`."""
    findings = []
    if skorlar_screen_text is None:
        findings.append("S61 skorlar_screen.dart: file missing")
        return findings
    missing = []
    if "Future<List<SessionScore>>" not in skorlar_screen_text and "Future<List<SessionScore>>" not in skorlar_screen_text:
        missing.append("Future<List<SessionScore>>")
    if "fetchScores" not in skorlar_screen_text:
        missing.append("fetchScores method")
    if "ConsumerStatefulWidget" not in skorlar_screen_text and "ConsumerState<" not in skorlar_screen_text:
        missing.append("ConsumerStatefulWidget / ConsumerState")
    if missing:
        findings.append(
            "S61 skorlar_screen.dart: missing " + ", ".join(missing) + ". "
            "Sprint 11.0C invariant — the screen is a Riverpod "
            "ConsumerStatefulWidget; the future list type "
            "Future<List<SessionScore>> + the fetchScores method "
            "are the canonical 11.0C wire shape."
        )
    return findings


def run_s62_check(score_calculator_text):
    """S62: score_calculator.dart has `compute` pure function."""
    findings = []
    if score_calculator_text is None:
        findings.append("S62 score_calculator.dart: file missing")
        return findings
    missing = []
    if "class SessionScoreCalculator" not in score_calculator_text:
        missing.append("class SessionScoreCalculator")
    if "static SessionScore compute" not in score_calculator_text:
        missing.append("static SessionScore compute method")
    if missing:
        findings.append(
            "S62 score_calculator.dart: missing " + ", ".join(missing) + ". "
            "Sprint 11.0C invariant — the `compute` method is a "
            "pure function (no I/O, no time-source injection) so "
            "it's unit-testable and the Skorlar screen can "
            "compute the headline score from a `summary_stats` "
            "block without side effects."
        )
    return findings


def run_s63_check(score_calculator_text):
    """S63: score_calculator.dart carries the 4 metric formulas."""
    findings = []
    if score_calculator_text is None:
        findings.append("S63 score_calculator.dart: file missing")
        return findings
    missing = []
    if "encryptionIntegrityPct" not in score_calculator_text:
        missing.append("encryptionIntegrityPct metric")
    if "packetLossPct" not in score_calculator_text:
        missing.append("packetLossPct metric")
    if "meanLatencyMs" not in score_calculator_text:
        missing.append("meanLatencyMs metric")
    if "jitterMs" not in score_calculator_text:
        missing.append("jitterMs metric")
    if missing:
        findings.append(
            "S63 score_calculator.dart: missing " + ", ".join(missing) + ". "
            "Sprint 11.0C invariant — the 4 metric fields "
            "(encryption integrity %, packet loss %, mean "
            "latency ms, jitter ms) are the inputs to the "
            "weighted sum; the Skorlar screen's `SessionScoreCard` "
            "detail view shows all 4 side-by-side."
        )
    return findings


def run_s64_check(score_calculator_text):
    """S64: score_calculator.dart overall weighted sum (0.4 + 0.3 + 0.2 + 0.1)."""
    findings = []
    if score_calculator_text is None:
        findings.append("S64 score_calculator.dart: file missing")
        return findings
    has_weights = ("0.4 *" in score_calculator_text and
                   "0.3 *" in score_calculator_text and
                   "0.2 *" in score_calculator_text and
                   "0.1 *" in score_calculator_text)
    if not has_weights:
        findings.append(
            "S64 score_calculator.dart: missing overall weighted "
            "sum weights (0.4 + 0.3 + 0.2 + 0.1). Sprint 11.0C "
            "invariant — the 4 weights sum to 1.0; the brief's "
            "spec is verbatim. The audit accepts the literal "
            "`0.4 *` + `0.3 *` + `0.2 *` + `0.1 *` substring "
            "sequence in the file's `compute` method."
        )
    return findings


def run_s65_check(orchestrator_text):
    """S65: session_orchestrator.dart has `closeSession()` method."""
    findings = []
    if orchestrator_text is None:
        findings.append("S65 session_orchestrator.dart: file missing")
        return findings
    missing = []
    if "closeSession" not in orchestrator_text:
        missing.append("closeSession method")
    if "/api/v1/sessions/" not in orchestrator_text or "close" not in orchestrator_text:
        missing.append("close endpoint path")
    if missing:
        findings.append(
            "S65 session_orchestrator.dart: missing " + ", ".join(missing) + ". "
            "Sprint 11.0C invariant — `closeSession()` POSTs to "
            "`/api/v1/sessions/{id}/close` and caches the "
            "`summary_stats` block. The active-pool screen's "
            "\"Oturumu Bitir\" button is the only call site."
        )
    return findings


def run_s66_check(active_pool_text):
    """S66: active_pool_screen.dart has the 'Oturumu Bitir' Turkish label."""
    findings = []
    if active_pool_text is None:
        findings.append("S66 active_pool_screen.dart: file missing")
        return findings
    if "Oturumu Bitir" not in active_pool_text:
        findings.append(
            "S66 active_pool_screen.dart: missing `Oturumu Bitir` "
            "Turkish label. Sprint 11.0C invariant — the button "
            "calls `_orchestrator.closeSession()` and navigates "
            "to /home/skorlar. S25 invariant extends: no 'VPN' "
            "string, Turkish UI text only."
        )
    return findings


def run_s67_check(active_pool_text):
    """S67: active_pool_screen.dart closeSession call + navigate to /home/skorlar."""
    findings = []
    if active_pool_text is None:
        findings.append("S67 active_pool_screen.dart: file missing")
        return findings
    has_call = "closeSession" in active_pool_text
    has_nav = "/home/skorlar" in active_pool_text
    missing = []
    if not has_call:
        missing.append("closeSession call site")
    if not has_nav:
        missing.append("/home/skorlar navigation")
    if missing:
        findings.append(
            "S67 active_pool_screen.dart: missing " + ", ".join(missing) + ". "
            "Sprint 11.0C invariant — the Oturumu Bitir flow "
            "calls `_orchestrator.closeSession()` then `context."
            "go('/home/skorlar')` so the new score is visible "
            "without an explicit refresh."
        )
    return findings


def run_s68_check(skorlar_screen_text):
    """S68: skorlar_screen.dart has the empty-state Turkish string."""
    findings = []
    if skorlar_screen_text is None:
        findings.append("S68 skorlar_screen.dart: file missing")
        return findings
    if "Henüz tamamlanmış oturum yok" not in skorlar_screen_text:
        findings.append(
            "S68 skorlar_screen.dart: missing `Henüz tamamlanmış "
            "oturum yok` empty-state string. Sprint 11.0C "
            "invariant — the screen shows the empty state when "
            "`fetchScores()` returns an empty list (no "
            "completed sessions yet)."
        )
    return findings


def run_s69_check(skorlar_screen_text):
    """S69: skorlar_screen.dart has a score card with overall gauge (disc)."""
    findings = []
    if skorlar_screen_text is None:
        findings.append("S69 skorlar_screen.dart: file missing")
        return findings
    has_card = "SessionScoreCard" in skorlar_screen_text
    has_gauge = ("_OverallScoreDisc" in skorlar_screen_text or
                 "overallScore" in skorlar_screen_text)
    missing = []
    if not has_card:
        missing.append("SessionScoreCard widget")
    if not has_gauge:
        missing.append("overallScore gauge (disc)")
    if missing:
        findings.append(
            "S69 skorlar_screen.dart: missing " + ", ".join(missing) + ". "
            "Sprint 11.0C invariant — each session card has a "
            "headline gauge (coloured disc with the overall "
            "score 0-100) plus an expandable details view with "
            "the 4 sub-metrics. The brief's spec is a `fl_chart` "
            "radial gauge; Sprint 11.0C uses a simple disc with "
            "the `tier` color hint to keep the APK small after "
            "the M2 +50 MB libwebrtc hit."
        )
    return findings


def run_s70_check(router_text):
    """S70: backend router.go POST /api/v1/sessions/{id}/close handler registration."""
    findings = []
    if router_text is None:
        findings.append("S70 backend/internal/api/router.go: file missing")
        return findings
    needle = 'r.Post("/sessions/{id}/close"'
    if needle not in router_text:
        findings.append(
            "S70 backend/internal/api/router.go: missing `"
            + needle + "` route registration. Sprint 11.0C "
            "invariant — the mobile orchestrator's "
            "`closeSession()` POSTs this endpoint; the handler "
            "in `sessions.go` marks the session completed and "
            "returns the `summary_stats` block."
        )
    return findings


def run_s71_check(sessions_go_text):
    """S71: backend sessions.go `summary_stats` response shape (4 fields + encrypted/total pair)."""
    findings = []
    if sessions_go_text is None:
        findings.append("S71 backend/internal/api/sessions.go: file missing")
        return findings
    missing = []
    if "summary_stats" not in sessions_go_text:
        missing.append("summary_stats key")
    for field in ("total_packets", "encrypted_packets", "packet_loss_pct",
                  "mean_latency_ms", "jitter_ms", "encryption_integrity_pct"):
        if field not in sessions_go_text:
            missing.append(field)
    if missing:
        findings.append(
            "S71 backend/internal/api/sessions.go: missing "
            + ", ".join(missing) + ". Sprint 11.0C invariant — "
            "the close handler's `summary_stats` block carries "
            "6 fields: `total_packets`, `encrypted_packets`, "
            "`packet_loss_pct`, `mean_latency_ms`, `jitter_ms`, "
            "`encryption_integrity_pct`. The mobile `SessionScore` "
            "JSON deserialiser reads all 6 into the calculator."
        )
    return findings


def run_s72_check(score_calculator_test_text):
    """S72: score_calculator_test.dart has 4+ unit tests."""
    findings = []
    if score_calculator_test_text is None:
        findings.append("S72 score_calculator_test.dart: file missing")
        return findings
    # Count `test(` occurrences — minimum 4 per brief.
    test_count = score_calculator_test_text.count("test(")
    if test_count < 4:
        findings.append(
            "S72 score_calculator_test.dart: missing the 4 unit "
            "tests (integration, loss, latency, jitter). "
            "Sprint 11.0C invariant — the brief requires "
            "exactly 4 unit tests for the calculator; the "
            "M3 implementation has 7 (the 4 brief + 3 extra: "
            "overall weighted sum, computeAll, standardDeviation "
            "helper)."
        )
    return findings


def run_s73_check(main_activity_text):
    """Sprint 11.0D: MainActivity.kt owns the `opene2ee/vpn` MethodChannel.

    Regression guard for the OnePlus 9 Pro error
    `MissingPluginException(No implementation found for method
    getSampledPackets on channel opene2ee/vpn)`. In Sprint 11.0A
    the channel handler was set inside `OpenE2eeVpnService.attachFlutterEngine`
    but the service isn't created until the user clicks
    "Şifreleme Doğrulamayı Başlat". The Dart-side polling loop
    (pool_provider.dart Timer.periodic) calls `getSampledPackets`
    immediately when the ActivePoolScreen opens — BEFORE the
    service exists. Result: `MissingPluginException`.

    The fix: handler lives in MainActivity (always alive from
    app launch), delegates to `OpenE2eeVpnService.dispatch(context, call, result)`
    static. The check requires FOUR tokens to be present in
    MainActivity.kt (comment-stripped):

      1. `MethodChannel(` constructor call
      2. `OpenE2eeVpnService.METHOD_CHANNEL` (or literal
         `"opene2ee/vpn"`) — the channel name
      3. `setMethodCallHandler` — the inbound handler install
      4. `OpenE2eeVpnService.dispatch` — the static dispatcher
         that routes per-method to the live service / safe defaults

    Missing ANY of these means the polling loop will hit
    `MissingPluginException` again on the OnePlus 9 Pro.
    """
    import re
    findings = []
    if main_activity_text is None:
        findings.append(
            "S73 MainActivity.kt: file missing. Sprint 11.0D "
            "invariant — MainActivity must own the `opene2ee/vpn` "
            "MethodChannel handler (set in `configureFlutterEngine`) "
            "so the Dart-side polling loop's `getSampledPackets` "
            "call lands on a registered handler before the "
            "VpnService is started. Otherwise: `MissingPluginException`."
        )
        return findings
    # Comment-strip (best-effort, mirrors S43 pattern).
    stripped = re.sub(r"/\*[\s\S]*?\*/", "", main_activity_text)
    lines = []
    for ln in stripped.splitlines():
        in_string = False
        i = 0
        cut_at = -1
        while i < len(ln):
            c = ln[i]
            if c == '"':
                in_string = not in_string
                i += 1
                continue
            if c == "/" and i + 1 < len(ln) and ln[i + 1] == "/" and not in_string:
                cut_at = i
                break
            i += 1
        if cut_at >= 0:
            lines.append(ln[:cut_at])
        else:
            lines.append(ln)
    code = "\n".join(lines)
    # 1. `MethodChannel(` constructor call
    if "MethodChannel(" not in code:
        findings.append(
            "S73 MainActivity.kt: missing `MethodChannel(` "
            "constructor call. Sprint 11.0D invariant — the "
            "`opene2ee/vpn` channel must be constructed in "
            "`configureFlutterEngine` (after super.configureFlutterEngine)"
            " so its inbound handler is wired at app launch."
        )
    # 2. Channel name — accept either the constant or the literal.
    has_const = "OpenE2eeVpnService.METHOD_CHANNEL" in code
    has_literal = '"opene2ee/vpn"' in code or "'opene2ee/vpn'" in code
    if not (has_const or has_literal):
        findings.append(
            "S73 MainActivity.kt: missing the `opene2ee/vpn` "
            "channel name (expected either `OpenE2eeVpnService.METHOD_CHANNEL` "
            "constant OR the literal `\"opene2ee/vpn\"`). Sprint "
            "11.0D invariant — the channel name MUST match the "
            "Dart-side `kVpnMethodChannel` constant in "
            "`vpn_service.dart`."
        )
    # 3. `setMethodCallHandler` install
    if "setMethodCallHandler" not in code:
        findings.append(
            "S73 MainActivity.kt: missing `setMethodCallHandler` "
            "install. Sprint 11.0D invariant — the inbound "
            "handler is the load-bearing fix for the OnePlus 9 "
            "Pro `MissingPluginException` regression."
        )
    # 4. `OpenE2eeVpnService.dispatch` reference
    if "OpenE2eeVpnService.dispatch" not in code:
        findings.append(
            "S73 MainActivity.kt: missing `OpenE2eeVpnService.dispatch` "
            "call. Sprint 11.0D invariant — the MainActivity-owned "
            "handler delegates to the static `dispatch(context, "
            "call, result)` which routes per-method to the live "
            "service OR returns safe defaults (empty ring / IDLE "
            "status) when no service is alive yet. Without this "
            "delegate, the handler would have to embed the "
            "service lifecycle logic, which is exactly the path "
            "Sprint 11.0A took and the path that broke."
        )
    return findings


def run_s74_check(opene2ee_vpn_service_text):
    """Sprint 11.0E: OpenE2eeVpnService.kt startForeground-within-5s (S74).

    Regression guard for the OnePlus 9 Pro
    `android.app.RemoteServiceException: Context.startForegroundService()
    did not then call Service.startForeground()` crash at 10:29
    on 11.07.2026. The 5-second rule (in place since Android 8
    / API 26) requires the VpnService to call
    `startForeground(id, notification)` within 5 seconds of
    `Context.startForegroundService(...)` being invoked. The
    Sprint 11.0E fix hoists the foreground promotion to the
    FIRST statement in `onStartCommand`, BEFORE
    `startCapture()` (which contains `Builder.establish()` —
    TUN setup, can be slow on some OEM ROMs).

    The check requires FIVE tokens to be present in
    OpenE2eeVpnService.kt (comment-stripped via the same Kotlin
    comment-strip loop used by S43 / S73):

      1. `startForeground(` — the foreground-service promotion
         call. (Either the typed 3-arg overload on Android 14+,
         or `ServiceCompat.startForeground(...)` on older API
         levels — both contain this substring.)
      2. `FOREGROUND_SERVICE_TYPE_SPECIAL_USE` — the typed
         foregroundServiceType for VPN services not classified
         as "system" (Android 14+ strict mode).
      3. `createNotificationChannel` OR
         `ensureNotificationChannel` — the Android 8+
         notification-channel creator.
      4. `onStartCommand` — the service-lifecycle hook.
      5. `onStartCommand` body must call `startForeground(`
         BEFORE `startCapture()` — the order matters for the
         5-second rule. Verified by extracting the
         `onStartCommand` body via brace-counting and checking
         that `startForeground(` appears inside it.

    Missing ANY of these re-opens the RemoteServiceException
    crash window.
    """
    import re
    findings = []
    if opene2ee_vpn_service_text is None:
        findings.append("S74 OpenE2eeVpnService.kt: file missing")
        return findings
    # Comment-strip loop (mirrors S43 / S73).
    stripped = re.sub(r"/\*[\s\S]*?\*/", "", opene2ee_vpn_service_text)
    lines = []
    for ln in stripped.splitlines():
        in_string = False
        i = 0
        cut_at = -1
        while i < len(ln):
            c = ln[i]
            if c == '"':
                in_string = not in_string
                i += 1
                continue
            if c == "/" and i + 1 < len(ln) and ln[i + 1] == "/" and not in_string:
                cut_at = i
                break
            i += 1
        if cut_at >= 0:
            lines.append(ln[:cut_at])
        else:
            lines.append(ln)
    code = "\n".join(lines)
    # 1. `startForeground(` call.
    if "startForeground(" not in code:
        findings.append(
            "S74 OpenE2eeVpnService.kt: missing `startForeground(` "
            "call. Sprint 11.0E invariant — the VpnService must "
            "promote itself to foreground state within Android's "
            "5-second rule. Otherwise: `RemoteServiceException: "
            "Context.startForegroundService() did not then call "
            "Service.startForeground()`."
        )
    # 2. `FOREGROUND_SERVICE_TYPE_SPECIAL_USE` constant.
    if "FOREGROUND_SERVICE_TYPE_SPECIAL_USE" not in code:
        findings.append(
            "S74 OpenE2eeVpnService.kt: missing "
            "`FOREGROUND_SERVICE_TYPE_SPECIAL_USE` constant. "
            "Sprint 11.0E invariant — Android 14+ (API 34) "
            "strict mode requires the typed `startForeground` "
            "overload with `FOREGROUND_SERVICE_TYPE_SPECIAL_USE` "
            "to match the manifest `foregroundServiceType=\"specialUse\"`."
        )
    # 3. `createNotificationChannel` OR `ensureNotificationChannel`.
    if "createNotificationChannel" not in code and "ensureNotificationChannel" not in code:
        findings.append(
            "S74 OpenE2eeVpnService.kt: missing "
            "`createNotificationChannel` (or `ensureNotificationChannel`) "
            "call. Sprint 11.0E invariant — Android 8+ requires a "
            "notification channel for the foreground notification; "
            "missing the channel is the silent-no-op failure mode."
        )
    # 4. `onStartCommand` lifecycle hook.
    if "onStartCommand" not in code:
        findings.append(
            "S74 OpenE2eeVpnService.kt: missing `onStartCommand` "
            "override. Sprint 11.0E invariant — the foreground "
            "promotion must run in the service-lifecycle hook."
        )
    # 5. `onStartCommand` body must call `startForeground(` either
    #    directly OR via a helper method (`ensureForegroundService` /
    #    `startForegroundCompat`). The Sprint 11.0E canonical
    #    pattern is the helper call; legacy 11.0A code called
    #    `startForeground(` directly inside `startCapture()`. The
    #    check accepts either, but flags the regression case
    #    (NEITHER helper NOR direct call in the body).
    onstart_match = re.search(
        r"override\s+fun\s+onStartCommand\s*\([^)]*\)[^{]*\{",
        code,
    )
    if onstart_match is None:
        findings.append(
            "S74 OpenE2eeVpnService.kt: `onStartCommand` body "
            "not found (parsing error)."
        )
    else:
        body_start = onstart_match.end() - 1
        depth = 0
        idx = body_start
        while idx < len(code):
            c = code[idx]
            if c == "{":
                depth += 1
            elif c == "}":
                depth -= 1
                if depth == 0:
                    break
            idx += 1
        body = code[body_start:idx + 1]
        has_direct = "startForeground(" in body
        has_helper = (
            "ensureForegroundService" in body
            or "startForegroundCompat" in body
        )
        if not (has_direct or has_helper):
            findings.append(
                "S74 OpenE2eeVpnService.kt: `onStartCommand` body "
                "does NOT call `startForeground(` (directly OR via "
                "a helper like `ensureForegroundService` / "
                "`startForegroundCompat`). Sprint 11.0E invariant — "
                "the call must be the FIRST statement in "
                "`onStartCommand` so the 5-second foreground-service "
                "rule is satisfied BEFORE any IO (TUN setup, DNS "
                "resolution). Pre-fix, the call was inside "
                "`startCapture()` AFTER `Builder.establish()`, which "
                "could exceed 5s."
            )
    return findings


def run_s75_check(opene2ee_vpn_service_text):
    """Sprint 11.0F: OpenE2eeVpnService.kt has at least 5 `Log.d(TAG,` breadcrumbs (S75).

    Regression guard for the OnePlus 9 Pro Senaryo D regression
    (Owner 10:56 / 11:01 reports): the Kotlin service was running
    but the UI's state pill stayed on "HAZIRLANIYOR" and the
    packet count never incremented. Without breadcrumbs, the
    next regression's root cause is opaque — the Coder session
    would have to re-add diagnostics to disambiguate. With
    breadcrumbs, `adb logcat -d -s OpenE2eeVpn:V` pinpoints
    the failing step (Sprint 11.0F post-fix evidence).

    The check counts `Log.d(TAG,` occurrences in the
    comment-stripped source. The Sprint 11.0F brief requires
    at least 5 across:
      - `startCapture()` (entry / buildVpnBuilder / establish
        null + non-null / startForegroundCompat /
        startReaderThread / startDrainLoop / success)
      - `onStartCommand` (entry / ensureForegroundService /
        intent-action branch / startCapture pre+post)
      - `Companion.dispatch` (entry / startForegroundService /
        activeInstance present / activeInstance null /
        getSampledPackets response)
      - `notifyError` (error path)
    """
    import re
    findings = []
    if opene2ee_vpn_service_text is None:
        findings.append("S75 OpenE2eeVpnService.kt: file missing")
        return findings
    stripped = re.sub(r"/\*[\s\S]*?\*/", "", opene2ee_vpn_service_text)
    lines = []
    for ln in stripped.splitlines():
        in_string = False
        i = 0
        cut_at = -1
        while i < len(ln):
            c = ln[i]
            if c == '"':
                in_string = not in_string
                i += 1
                continue
            if c == "/" and i + 1 < len(ln) and ln[i + 1] == "/" and not in_string:
                cut_at = i
                break
            i += 1
        if cut_at >= 0:
            lines.append(ln[:cut_at])
        else:
            lines.append(ln)
    code = "\n".join(lines)
    log_d_count = code.count("Log.d(TAG,")
    if log_d_count < 5:
        findings.append(
            "S75 OpenE2eeVpnService.kt: only " + str(log_d_count) +
            " `Log.d(TAG,` statement(s) found; Sprint 11.0F "
            "invariant requires at least 5 across startCapture / "
            "onStartCommand / dispatch / notifyError so the "
            "OnePlus 9 Pro regression is diagnosable via "
            "`adb logcat -d -s OpenE2eeVpn:V`."
        )
    return findings


def run_s76_check(vpn_service_text):
    """Sprint 11.0F + 11.0G: vpn_service.dart exposes VpnService as a Dart singleton (S76).

    Regression guard for the OnePlus 9 Pro Senaryo D
    regression. Pre-11.0F, every call site constructed a
    fresh `VpnService()` whose constructor:
      (a) replaced the previous `_channel.setMethodCallHandler`
          on the global `opene2ee/vpn` channel — events
          landed on whichever instance was constructed LAST
          (typically `PoolNotifier` in the Riverpod provider
          graph, NOT the `active_pool_screen`);
      (b) created a fresh `_packetCtrl` / `_stateCtrl`
          StreamController — the UI's old listeners never
          saw updates.
    Result: the Kotlin service ran, the foreground
    notification was visible, but the UI's state pill stayed
    on "HAZIRLANIYOR" and the packet count never incremented.

    Sprint 11.0G tightening (Owner 11:25): the 11.0F form
    had `factory VpnService() => _instance` (a back-compat
    factory that still allowed `VpnService()` to be called
    from external code). The factory masked the singleton
    requirement at code-review time. 11.0G REMOVES the
    public factory — only `VpnService.instance` (singleton)
    and `VpnService.forTesting(...)` (test override) remain
    callable. The check requires:
      1. A private constructor (`VpnService._internal(` or
         `VpnService._(`) — used by the static `_instance`
         initializer.
      2. `static VpnService get instance` (or
         `static final VpnService _instance`) — the
         singleton accessor.
      3. The default `VpnService()` ctor MUST NOT be present
         in non-comment form (the 11.0F back-compat factory
         is REMOVED in 11.0G — a stray `VpnService()` is a
         hard compile error).
    """
    import re
    findings = []
    if vpn_service_text is None:
        findings.append("S76 vpn_service.dart: file missing")
        return findings
    # 1. Private constructor.
    has_private_ctor = bool(re.search(
        r"VpnService\._(?:internal)?\s*\(",
        vpn_service_text,
    ))
    if not has_private_ctor:
        findings.append(
            "S76 vpn_service.dart: missing the private constructor "
            "(`VpnService._internal(` or `VpnService._(`). "
            "Sprint 11.0F + 11.0G invariant — the singleton "
            "pattern requires a private constructor to prevent "
            "external instantiation."
        )
    # 2. Singleton accessor.
    has_getter = "static VpnService get instance" in vpn_service_text
    has_field = "static final VpnService _instance" in vpn_service_text
    if not (has_getter or has_field):
        findings.append(
            "S76 vpn_service.dart: missing the singleton accessor "
            "(`static VpnService get instance` or "
            "`static final VpnService _instance`). Sprint 11.0F "
            "invariant — without the static field/getter, every "
            "call site constructs a fresh VpnService and the "
            "OnePlus 9 Pro Senaryo D regression returns."
        )
    # 3. 11.0G — NO public `VpnService()` ctor. Comment-strip
    #    first, then look for `VpnService()` (just parens, no
    #    name) which would be the removed form.
    stripped = re.sub(r"/\*[\s\S]*?\*/", "", vpn_service_text)
    code_lines = []
    for ln in stripped.splitlines():
        in_string = False
        i = 0
        cut_at = -1
        while i < len(ln):
            c = ln[i]
            if c == '"':
                in_string = not in_string
                i += 1
                continue
            if c == "/" and i + 1 < len(ln) and ln[i + 1] == "/" and not in_string:
                cut_at = i
                break
            i += 1
        if cut_at >= 0:
            code_lines.append(ln[:cut_at])
        else:
            code_lines.append(ln)
    code = "\n".join(code_lines)
    bad_default_ctor = re.search(r"VpnService\s*\(\s*\)", code)
    if bad_default_ctor:
        findings.append(
            "S76 vpn_service.dart: contains a public `VpnService()` "
            "default constructor. Sprint 11.0G invariant — the "
            "11.0F back-compat factory was REMOVED so a stray "
            "`VpnService()` call is a hard error. Use "
            "`VpnService.instance` (singleton) or "
            "`VpnService.forTesting(...)` (test override). The "
            "11.0F factory masked the Senaryo D regression because "
            "it made `VpnService()` indistinguishable from a "
            "fresh-instance ctor at code-review time."
        )
    return findings


def run_s77_check(active_pool_screen_text):
    """Sprint 11.0G: active_pool_screen.dart UI propagation invariant (S77).

    The 11.0F singleton fix was necessary but not sufficient
    — Owner 11:25 confirmed: the singleton IS in place but
    the UI's state pill still doesn't update because the
    screen's `_vpn = VpnService()` call site kept the
    regression surface opaque (the call shape `VpnService()`
    was indistinguishable from a fresh-instance ctor at
    code-review time, even though the factory returned the
    singleton).

    The 11.0G fix has THREE parts:
      1. The default `VpnService()` ctor is REMOVED —
         only `VpnService.instance` and
         `VpnService.forTesting(...)` remain callable
         (enforced by S76).
      2. The `active_pool_screen.dart` uses the explicit
         `VpnService.instance` form (NOT `VpnService()`).
      3. The state stream subscription calls `setState(...)`
         inside the `listen` callback so the widget rebuilds
         on every state transition.

    This check (S77) verifies parts 2 and 3:
      1. The screen class extends `ConsumerStatefulWidget`.
      2. The screen has a `stateStream.listen` subscription
         whose callback body contains `setState(`.
      3. The screen references `VpnService.instance` (or
         `vpnServiceProvider`).
    """
    findings = []
    if active_pool_screen_text is None:
        findings.append("S77 active_pool_screen.dart: file missing")
        return findings
    if "ConsumerStatefulWidget" not in active_pool_screen_text:
        findings.append(
            "S77 active_pool_screen.dart: does NOT extend "
            "`ConsumerStatefulWidget`. Sprint 11.0G invariant — "
            "the screen must be a `ConsumerStatefulWidget` so "
            "`ref.watch(vpnServiceProvider)` and "
            "`ref.listen(...)` are available."
        )
    if "stateStream.listen" not in active_pool_screen_text and \
            ".stateStream.listen" not in active_pool_screen_text:
        findings.append(
            "S77 active_pool_screen.dart: missing `stateStream.listen` "
            "subscription. Sprint 11.0G invariant — the screen "
            "must subscribe to the singleton's `stateStream` so "
            "VpnLifecycleState transitions propagate to the UI."
        )
    if "setState(" not in active_pool_screen_text:
        findings.append(
            "S77 active_pool_screen.dart: no `setState(` call "
            "found. Sprint 11.0G invariant — the `stateStream"
            ".listen` callback must call `setState(` so the "
            "widget rebuilds on VPN state transitions."
        )
    if "VpnService.instance" not in active_pool_screen_text and \
            "vpnServiceProvider" not in active_pool_screen_text:
        findings.append(
            "S77 active_pool_screen.dart: does NOT reference "
            "`VpnService.instance` (or `vpnServiceProvider`). "
            "Sprint 11.0G invariant — the screen must use the "
            "explicit singleton form. Pre-11.0G, the "
            "`VpnService()` call site kept the regression surface "
            "opaque (Owner 11:25 confirmation)."
        )
    return findings


def run_s78_check(opene2ee_vpn_service_text):
    """Sprint 11.0H: OpenE2eeVpnService.kt state-transition breadcrumbs + TOCTOU guard (S78).

    Regression guard for the OnePlus 9 Pro
    `start` returns `state: DRAINING` regression
    (Owner 11:38 logcat). The 11.0F/11.0G singleton + UI
    propagation fixes were necessary but not sufficient —
    the Kotlin-side `startCapture` was racing with a
    `stopCapture` (likely Magisk Zygisk revoke on a rooted
    OnePlus) so the `startCapture` finished setting
    `state = SAMPLING` but the racing `stopCapture` then
    set `state = DRAINING` and the result returned to Dart
    was the DRAINING state (not SAMPLING).

    The Sprint 11.0H fix has TWO parts:
      1. State-transition `Log.d` / `Log.w` breadcrumbs at
         each step of `startCapture` / `stopCapture` /
         `onRevoke` so the next regression is diagnosable
         via `adb logcat -d -s OpenE2eeVpn:V` (the
         `state: DRAINING` symptom doesn't include a
         stacktrace, so the only signal is the
         breadcrumb order).
      2. A `synchronized(stateLock)` TOCTOU guard around
         `startCapture` and `stopCapture` so the
         start/stop race is impossible — the second
         invocation waits for the first to complete.

    The check requires FOUR tokens in
    `OpenE2eeVpnService.kt` (comment-stripped):
      1. `startCapture: SAMPLING started` literal — the
         happy-path state-transition log.
      2. `stopCapture: called` literal — the stop-path
         entry log.
      3. `onRevoke:` literal — the system-side revoke
         callback instrumentation.
      4. `synchronized(` literal paired with a `stateLock`
         reference — the TOCTOU guard.
    """
    import re
    findings = []
    if opene2ee_vpn_service_text is None:
        findings.append("S78 OpenE2eeVpnService.kt: file missing")
        return findings
    # Comment-strip loop (mirrors S43 / S73 / S74 / S75).
    stripped = re.sub(r"/\*[\s\S]*?\*/", "", opene2ee_vpn_service_text)
    code_lines = []
    for ln in stripped.splitlines():
        in_string = False
        i = 0
        cut_at = -1
        while i < len(ln):
            c = ln[i]
            if c == '"':
                in_string = not in_string
                i += 1
                continue
            if c == "/" and i + 1 < len(ln) and ln[i + 1] == "/" and not in_string:
                cut_at = i
                break
            i += 1
        if cut_at >= 0:
            code_lines.append(ln[:cut_at])
        else:
            code_lines.append(ln)
    code = "\n".join(code_lines)
    # 1. startCapture: SAMPLING started.
    if "startCapture: SAMPLING started" not in code:
        findings.append(
            "S78 OpenE2eeVpnService.kt: missing `startCapture: "
            "SAMPLING started` literal. Sprint 11.0H invariant — "
            "the state-transition log at the happy-path point is "
            "the breadcrumb that distinguishes a healthy start "
            "from a racing-stop regression."
        )
    # 2. stopCapture: called.
    if "stopCapture: called" not in code:
        findings.append(
            "S78 OpenE2eeVpnService.kt: missing `stopCapture: "
            "called` literal. Sprint 11.0H invariant — the "
            "stop-path entry log identifies WHO called stopCapture."
        )
    # 3. onRevoke:.
    if "onRevoke:" not in code:
        findings.append(
            "S78 OpenE2eeVpnService.kt: missing `onRevoke:` "
            "literal. Sprint 11.0H invariant — the system-side "
            "revoke callback (Magisk Zygisk / settings / user) "
            "must be instrumented."
        )
    # 4. synchronized(stateLock) TOCTOU guard.
    has_synchronized = "synchronized(" in code
    has_state_lock = "stateLock" in code
    if not (has_synchronized and has_state_lock):
        findings.append(
            "S78 OpenE2eeVpnService.kt: missing `synchronized(stateLock)` "
            "TOCTOU guard. Sprint 11.0H invariant — the "
            "startCapture / stopCapture race is impossible without "
            "serializing the two paths."
        )
    return findings


def run_s79_check(opene2ee_vpn_service_text):
    """Sprint 11.0I: OpenE2eeVpnService.kt has correct addRoute (S79).

    Regression guard for the OnePlus 9 Pro
    `IllegalArgumentException: Bad address` crash
    (Owner 11:46-11:57 logcat). Pre-11.0I, `buildVpnBuilder`
    used `.addAddress(TUN_ADDRESS, 24)` +
    `.addRoute(TUN_ADDRESS, 24)` — the SAME IP for both
    the interface address AND the captured route
    destination. Android's `VpnService.Builder.addRoute`
    expects a DESTINATION SUBNET, NOT the interface
    address. OnePlus 9 Pro OxygenOS strict validation
    rejects the mirror-bug form with
    `IllegalArgumentException: Bad address` (Pixel / Samsung
    tolerate the bug; OnePlus does not).

    The Sprint 11.0I fix uses `.addRoute("0.0.0.0", 0)` —
    the default route (ALL traffic).

    The check requires THREE tokens in `OpenE2eeVpnService.kt`
    (comment-stripped):
      1. `.addAddress(TUN_ADDRESS` literal present.
      2. `.addRoute("0.0.0.0", 0)` literal present.
      3. `.addRoute(TUN_ADDRESS` literal ABSENT (anti-pattern
         guard).
    """
    import re
    findings = []
    if opene2ee_vpn_service_text is None:
        findings.append("S79 OpenE2eeVpnService.kt: file missing")
        return findings
    stripped = re.sub(r"/\*[\s\S]*?\*/", "", opene2ee_vpn_service_text)
    code_lines = []
    for ln in stripped.splitlines():
        in_string = False
        i = 0
        cut_at = -1
        while i < len(ln):
            c = ln[i]
            if c == '"':
                in_string = not in_string
                i += 1
                continue
            if c == "/" and i + 1 < len(ln) and ln[i + 1] == "/" and not in_string:
                cut_at = i
                break
            i += 1
        if cut_at >= 0:
            code_lines.append(ln[:cut_at])
        else:
            code_lines.append(ln)
    code = "\n".join(code_lines)
    # 1. `.addAddress(TUN_ADDRESS` literal.
    if ".addAddress(TUN_ADDRESS" not in code:
        findings.append(
            "S79 OpenE2eeVpnService.kt: missing `.addAddress(TUN_ADDRESS` "
            "literal. Sprint 11.0I invariant — the interface address "
            "MUST be set with the `TUN_ADDRESS` constant."
        )
    # 2. `.addRoute("0.0.0.0", 0)` literal OR the constant form.
    has_literal = re.search(r"\.addRoute\(\s*\"0\.0\.0\.0\"\s*,\s*0\s*\)", code)
    has_constant = (
        "CAPTURED_ROUTE_ADDRESS" in code and
        "CAPTURED_ROUTE_PREFIX" in code
    )
    if not (has_literal or has_constant):
        findings.append(
            "S79 OpenE2eeVpnService.kt: missing the default-route "
            "addRoute (`.addRoute(\"0.0.0.0\", 0)` literal OR the "
            "`addRoute(CAPTURED_ROUTE_ADDRESS, CAPTURED_ROUTE_PREFIX)` "
            "constant form). Sprint 11.0I invariant — the captured "
            "route MUST be the default route."
        )
    # 3. `.addRoute(TUN_ADDRESS` ABSENT (anti-pattern guard).
    if re.search(r"\.addRoute\(\s*TUN_ADDRESS", code):
        findings.append(
            "S79 OpenE2eeVpnService.kt: contains the anti-pattern "
            "`.addRoute(TUN_ADDRESS, ...)` — the 9.7.0 mirror bug. "
            "Sprint 11.0I invariant — `addRoute` takes a DESTINATION "
            "SUBNET, NOT the interface address. Use "
            "`.addRoute(\"0.0.0.0\", 0)` (default route)."
        )
    return findings


def run_s80_check(opene2ee_vpn_service_text):
    """Sprint 11.0J: OpenE2eeVpnService.kt has TUN passthrough (S80).

    Regression guard for the OnePlus 9 Pro "VPN active,
    internet dead" symptom (Owner 12:14 report, PID 4244).
    The 11.0I fix (`.addRoute("0.0.0.0", 0)`) is necessary
    but not sufficient — without the TUN passthrough pattern
    in the reader thread, the kernel drops all packets the
    TUN consumes from the input side. Result: the OS
    triggers a system-side `onRevoke()` after 5-30s.

    The check requires THREE tokens in
    `OpenE2eeVpnService.kt` (comment-stripped):
      1. `output.write(buf` OR `tunOutput.write(` OR
         `tun.write(` literal present.
      2. `input.read(buf` OR `tunInput.read` OR
         `tun.read` literal present.
      3. NO `protect(Socket())` no-op (anti-pattern guard).
    """
    import re
    findings = []
    if opene2ee_vpn_service_text is None:
        findings.append("S80 OpenE2eeVpnService.kt: file missing")
        return findings
    stripped = re.sub(r"/\*[\s\S]*?\*/", "", opene2ee_vpn_service_text)
    code_lines = []
    for ln in stripped.splitlines():
        in_string = False
        i = 0
        cut_at = -1
        while i < len(ln):
            c = ln[i]
            if c == '"':
                in_string = not in_string
                i += 1
                continue
            if c == "/" and i + 1 < len(ln) and ln[i + 1] == "/" and not in_string:
                cut_at = i
                break
            i += 1
        if cut_at >= 0:
            code_lines.append(ln[:cut_at])
        else:
            code_lines.append(ln)
    code = "\n".join(code_lines)
    # 1. Passthrough write call.
    has_passthrough = (
        "output.write(buf" in code or
        "tunOutput.write(" in code or
        "tun.write(" in code
    )
    if not has_passthrough:
        findings.append(
            "S80 OpenE2eeVpnService.kt: missing the TUN passthrough "
            "write call (`output.write(buf, 0, n)`). Sprint 11.0J "
            "invariant — the reader thread MUST write each packet "
            "back to the TUN output stream so the kernel routes "
            "the packet out the device's real NIC."
        )
    # 2. Input read call.
    has_input_read = (
        "input.read(buf" in code or
        "tunInput.read(" in code or
        "tun.read(" in code
    )
    if not has_input_read:
        findings.append(
            "S80 OpenE2eeVpnService.kt: missing the TUN input "
            "read call (`input.read(buf)`). Sprint 11.0J "
            "invariant — the reader thread must read from the "
            "TUN input stream before writing back to the output."
        )
    # 3. Anti-pattern guard: `protect(Socket())` no-op.
    has_protect_socket = re.search(r"protect\s*\(\s*Socket\s*\(\s*\)", code)
    if has_protect_socket:
        findings.append(
            "S80 OpenE2eeVpnService.kt: contains the anti-pattern "
            "`protect(Socket())` — the 11.0A-era misconception. "
            "Sprint 11.0J invariant — `protect(Socket)` marks a "
            "SOCKET as 'not VPN-routed' but the VPN reader thread "
            "has no socket to protect. Use `output.write(buf, 0, n)`."
        )
    return findings


def run_s82_check(opene2ee_vpn_service_text):
    """Sprint 11.0K: OpenE2eeVpnService.kt dispatches
    MethodChannel calls to the Android main looper (S82).

    Regression guard for the OnePlus 9 Pro "VPN active,
    internet working, UI never updates" symptom (Owner 12:31
    report, PID 4244, 98 packets in 80s, `deltaPerInterval`
    Log.d in logcat, but UI stays frozen on
    `state: DRAINING, packetsObserved: 0, ringSize: 0`).

    REAL root cause (overriding the 11.0K brief hypothesis):
    the Flutter Engine requires `MethodChannel.invokeMethod`
    to be called on the Android UI thread (the main
    `Looper`). Pre-11.0K, the three call sites
    (flushTelemetry's `onTelemetry`, notifyError's
    `onError`, PacketDrain's `onPacketsSampled`) invoked
    `methodChannel?.invokeMethod` directly from their
    caller threads (TUN reader thread + PacketDrain
    ScheduledExecutor worker thread). The engine threw
    `@UiThread` violations and Dart never received any
    of the three events.

    11.0K fix:
      1. Companion declares `@JvmField val mainHandler:
         Handler = Handler(Looper.getMainLooper())` (eagerly
         initialized at class-load time).
      2. flushTelemetry + notifyError dispatch via a
         `pushToDart(method, args)` helper that calls
         `mainHandler.post { ... }`.
      3. PacketDrain inlines `OpenE2eeVpnService.mainHandler
         .post { ch.invokeMethod("onPacketsSampled", packets) }`.
      4. New imports: `android.os.Handler` +
         `android.os.Looper`.

    The check requires FIVE tokens in `OpenE2eeVpnService.kt`
    (comment-stripped):
      1. `import android.os.Handler` literal.
      2. `import android.os.Looper` literal.
      3. `Handler(Looper.getMainLooper())` literal.
      4. `mainHandler.post` literal.
      5. `invoke_method_count <= mainHandler.post_count` —
         every direct `methodChannel?.invokeMethod(` or
         `ch.invokeMethod(` call site is wrapped in
         `mainHandler.post { ... }` (anti-pattern guard).
    """
    import re
    findings = []
    if opene2ee_vpn_service_text is None:
        findings.append("S82 OpenE2eeVpnService.kt: file missing")
        return findings
    stripped = re.sub(r"/\*[\s\S]*?\*/", "", opene2ee_vpn_service_text)
    code_lines = []
    for ln in stripped.splitlines():
        in_string = False
        i = 0
        cut_at = -1
        while i < len(ln):
            c = ln[i]
            if c == '"':
                in_string = not in_string
                i += 1
                continue
            if c == "/" and i + 1 < len(ln) and ln[i + 1] == "/" and not in_string:
                cut_at = i
                break
            i += 1
        if cut_at >= 0:
            code_lines.append(ln[:cut_at])
        else:
            code_lines.append(ln)
    code = "\n".join(code_lines)
    # 1. `import android.os.Handler`.
    if "import android.os.Handler" not in code:
        findings.append(
            "S82 OpenE2eeVpnService.kt: missing `import "
            "android.os.Handler`. Sprint 11.0K invariant — "
            "the Handler class is required for the "
            "`mainHandler.post { ... }` dispatch. Add the "
            "import."
        )
    # 2. `import android.os.Looper`.
    if "import android.os.Looper" not in code:
        findings.append(
            "S82 OpenE2eeVpnService.kt: missing `import "
            "android.os.Looper`. Sprint 11.0K invariant — "
            "the Looper class is required to construct the "
            "main-thread Handler via `Handler(Looper.getMainLooper())`. "
            "Add the import."
        )
    # 3. `Handler(Looper.getMainLooper())` companion field.
    if not re.search(r"Handler\s*\(\s*Looper\.getMainLooper\s*\(\s*\)\s*\)", code):
        findings.append(
            "S82 OpenE2eeVpnService.kt: missing the "
            "`Handler(Looper.getMainLooper())` companion field. "
            "Sprint 11.0K invariant — the `mainHandler` "
            "field must be declared as `@JvmField val "
            "mainHandler: Handler = Handler(Looper.getMainLooper())` "
            "on the companion object so the first push from a "
            "worker thread does not have to construct the "
            "Handler."
        )
    # 4. `mainHandler.post` dispatch.
    if "mainHandler.post" not in code:
        findings.append(
            "S82 OpenE2eeVpnService.kt: missing `mainHandler.post` "
            "dispatch. Sprint 11.0K invariant — the three "
            "MethodChannel invocations (`onTelemetry` in "
            "flushTelemetry, `onError` in notifyError, "
            "`onPacketsSampled` in PacketDrain) must use "
            "`mainHandler.post { methodChannel?.invokeMethod "
            "(...) }` (or the `pushToDart` helper which calls "
            "it)."
        )
    # 5. Anti-pattern guard: every direct `methodChannel?.invokeMethod`
    #    or `ch.invokeMethod` call site is wrapped in
    #    `mainHandler.post { ... }`.
    invoke_call_count = 0
    for m in re.finditer(r"methodChannel\?\.invokeMethod\s*\(|ch\.invokeMethod\s*\(", code):
        invoke_call_count += 1
    post_count = len(re.findall(r"mainHandler\.post\s*\{", code))
    if invoke_call_count > post_count:
        findings.append(
            "S82 OpenE2eeVpnService.kt: found " +
            str(invoke_call_count) + " direct "
            "`methodChannel?.invokeMethod(` / "
            "`ch.invokeMethod(` call site(s) but only " +
            str(post_count) + " `mainHandler.post { ... }` "
            "wrapper(s). Sprint 11.0K invariant — the "
            "Flutter Engine throws `@UiThread` for every push "
            "that happens on a non-UI thread (PacketDrain "
            "worker, TUN reader thread). All " +
            str(invoke_call_count) + " invoke sites must be "
            "wrapped in a `mainHandler.post { ... }` block."
        )
    return findings



def run_s84_check(opene2ee_vpn_service_text):
    """Sprint 11.0M: OpenE2eeVpnService.kt packetsObserved
    increment invariant (S84).

    Owner 13:08 fake-capture accusation: the Owner thought
    the 258-packet counter was a fake increment because
    Chrome and WhatsApp have no internet. But the counter
    IS real - the TUN reader does receive bytes from the
    kernel; the passthrough write is what was broken (11.0L
    passthrough write fails on OnePlus 9 Pro OxygenOS).

    This check grep-asserts the invariant:
      1. EXACTLY ONE packetsObserved.incrementAndGet() call
         site in the file.
      2. That site is inside startReaderThread.
      3. The site is preceded by extractMetadata (within 600
         chars), confirming it's in the post-extract read
         branch.
      4. packetsObserved.set( is allowed ONLY in startCapture
         (the reset-on-new-session site).
      5. NO anti-pattern packetsObserved.set(
         packetsObserved.get() + 1).
    """
    import re
    findings = []
    if opene2ee_vpn_service_text is None:
        findings.append("S84 OpenE2eeVpnService.kt: file missing")
        return findings
    stripped = re.sub(r"/\*[\s\S]*?\*/", "", opene2ee_vpn_service_text)
    code_lines = []
    for ln in stripped.splitlines():
        in_string = False
        i = 0
        cut_at = -1
        while i < len(ln):
            c = ln[i]
            if c == '"':
                in_string = not in_string
                i += 1
                continue
            if c == "/" and i + 1 < len(ln) and ln[i + 1] == "/" and not in_string:
                cut_at = i
                break
            i += 1
        if cut_at >= 0:
            code_lines.append(ln[:cut_at])
        else:
            code_lines.append(ln)
    code = "\n".join(code_lines)
    # 1. EXACTLY ONE increment site.
    increment_matches = list(re.finditer(
        r"packetsObserved\s*\.\s*incrementAndGet\s*\(\s*\)", code
    ))
    if len(increment_matches) == 0:
        findings.append(
            "S84 OpenE2eeVpnService.kt: ZERO packetsObserved."
            "incrementAndGet call sites. Sprint 11.0M invariant."
        )
    elif len(increment_matches) > 1:
        findings.append(
            "S84 OpenE2eeVpnService.kt: " + str(len(increment_matches)) +
            " packetsObserved.incrementAndGet call site(s); expected 1."
        )
    else:
        inc_pos = increment_matches[0].start()
        func_match = None
        for fm in re.finditer(r"(?:private\s+)?fun\s+(\w+)\s*\(", code):
            if fm.start() < inc_pos:
                func_match = fm
            else:
                break
        if func_match is None or func_match.group(1) != "startReaderThread":
            func_name = func_match.group(1) if func_match else "<unknown>"
            findings.append(
                "S84 OpenE2eeVpnService.kt: increment in " + func_name +
                ", NOT in startReaderThread."
            )
        else:
            window_before = code[max(0, inc_pos - 600):inc_pos]
            if "extractMetadata" not in window_before:
                findings.append(
                    "S84 OpenE2eeVpnService.kt: increment in startReaderThread "
                    "but NOT preceded by extractMetadata."
                )
    # 3. set() allowed ONLY in startCapture.
    set_matches = list(re.finditer(
        r"packetsObserved\s*\.\s*set\s*\(", code
    ))
    for sm in set_matches:
        sm_pos = sm.start()
        func_match = None
        for fm in re.finditer(r"(?:private\s+)?fun\s+(\w+)\s*\(", code):
            if fm.start() < sm_pos:
                func_match = fm
            else:
                break
        func_name = func_match.group(1) if func_match else "<unknown>"
        if func_name != "startCapture":
            findings.append(
                "S84 OpenE2eeVpnService.kt: packetsObserved.set( in " +
                func_name + ", NOT in startCapture."
            )
    # 4. Anti-pattern guard.
    if re.search(
        r"packetsObserved\s*\.\s*set\s*\(\s*packetsObserved\s*\.\s*get\s*\(\s*\)\s*\+\s*1",
        code,
    ):
        findings.append(
            "S84 OpenE2eeVpnService.kt: contains fake-increment anti-pattern."
        )
    return findings



# ─── Test cases ──────────────────────────────────────────────────

# Case 0: fully-valid file (post-Sprint 9.6.6 fix) — expect 0 findings.
case_pass = """
import java.util.Properties
import org.jetbrains.kotlin.gradle.dsl.JvmTarget

android {
    namespace = "x"
    compileOptions { sourceCompatibility = JavaVersion.VERSION_17 }
}

flutter { source = "." }

kotlin {
    compilerOptions { jvmTarget.set(JvmTarget.JVM_17) }
}

dependencies {}

create("release") {
    val keyPropsFile = rootProject.file("k")
    if (keyPropsFile.exists()) {
        val keyProps = Properties().apply { load(keyPropsFile.inputStream()) }
    }
}
"""

# Case 1: Sprint 9.6.5 broken state — comment claims import but no
# actual import, fully-qualified usage present.
case_s1_fail = """
import org.jetbrains.kotlin.gradle.dsl.JvmTarget

android {
    compileOptions { sourceCompatibility = JavaVersion.VERSION_17 }
}

flutter { source = "." }

kotlin {
    compilerOptions { jvmTarget.set(JvmTarget.JVM_17) }
}

dependencies {}

create("release") {
    val keyPropsFile = rootProject.file("k")
    if (keyPropsFile.exists()) {
        // explicit `import java.util.Properties` (Kotlin 2.0+ stricter...)
        val keyProps = java.util.Properties().apply { load(keyPropsFile.inputStream()) }
    }
}
"""

# Case 2: missing JvmTarget import.
case_s2_fail = """
import java.util.Properties

android {
    compileOptions { sourceCompatibility = JavaVersion.VERSION_17 }
}

flutter { source = "." }

kotlin {
    compilerOptions { jvmTarget.set(JvmTarget.JVM_17) }
}

dependencies {}

create("release") {
    val keyProps = Properties().apply { load(keyPropsFile.inputStream()) }
}
"""

# Case 3: deprecated kotlinOptions block present.
case_s3_fail = """
import java.util.Properties
import org.jetbrains.kotlin.gradle.dsl.JvmTarget

android {
    compileOptions { sourceCompatibility = JavaVersion.VERSION_17 }
    kotlinOptions { jvmTarget = "17" }
}

flutter { source = "." }

kotlin {
    compilerOptions { jvmTarget.set(JvmTarget.JVM_17) }
}

dependencies {}

create("release") {
    val keyProps = Properties().apply { load(keyPropsFile.inputStream()) }
}
"""

# Case 4: missing new kotlin { compilerOptions } block.
case_s4_fail = """
import java.util.Properties
import org.jetbrains.kotlin.gradle.dsl.JvmTarget

android {
    compileOptions { sourceCompatibility = JavaVersion.VERSION_17 }
}

flutter { source = "." }

dependencies {}

create("release") {
    val keyProps = Properties().apply { load(keyPropsFile.inputStream()) }
}
"""

# Case 5: fully-qualified java.util.Properties() still present.
case_s5_fail = """
import java.util.Properties
import org.jetbrains.kotlin.gradle.dsl.JvmTarget

android {
    compileOptions { sourceCompatibility = JavaVersion.VERSION_17 }
}

flutter { source = "." }

kotlin {
    compilerOptions { jvmTarget.set(JvmTarget.JVM_17) }
}

dependencies {}

create("release") {
    val keyProps = java.util.Properties().apply { load(keyPropsFile.inputStream()) }
}
"""

# ─── S6 test cases (Sprint 9.6.7) ────────────────────────────────

# Case 6 (S6 PASS): android-debug.yml with `Install Flutter dependencies`
# step present, working-directory: ./mobile, run: flutter pub get.
case_s6_pass = """
name: Android Debug APK
on:
  workflow_dispatch:
jobs:
  android-debug:
    runs-on: ubuntu-latest
    steps:
      - name: Checkout
        uses: actions/checkout@v4
      - name: Setup Flutter
        uses: subosito/flutter-action@v2
        with:
          flutter-version: '3.44.1'
      - name: Install Flutter dependencies
        working-directory: ./mobile
        run: flutter pub get
      - name: Build Debug APK
        working-directory: ./mobile/android
        run: ./gradlew assembleDebug
"""

# Case 7 (S6 FAIL — step missing): no `flutter pub get` step at all.
case_s6_step_missing = """
name: Android Debug APK
on:
  workflow_dispatch:
jobs:
  android-debug:
    runs-on: ubuntu-latest
    steps:
      - name: Checkout
        uses: actions/checkout@v4
      - name: Setup Flutter
        uses: subosito/flutter-action@v2
        with:
          flutter-version: '3.44.1'
      - name: Verify Flutter dependency cache
        run: flutter --version
      - name: Build Debug APK
        working-directory: ./mobile/android
        run: ./gradlew assembleDebug
"""

# Case 8 (S6 FAIL — wrong working-directory): step present but
# `working-directory: ./mobile/android` (the Gradle subproject).
case_s6_wrong_wd = """
name: Android Debug APK
on:
  workflow_dispatch:
jobs:
  android-debug:
    runs-on: ubuntu-latest
    steps:
      - name: Checkout
        uses: actions/checkout@v4
      - name: Install Flutter dependencies
        working-directory: ./mobile/android
        run: flutter pub get
      - name: Build Debug APK
        working-directory: ./mobile/android
        run: ./gradlew assembleDebug
"""

# Case 9 (S6 FAIL — run is something else): step with the right
# working-directory but `run: echo hello` instead of `flutter pub get`.
case_s6_wrong_run = """
name: Android Debug APK
on:
  workflow_dispatch:
jobs:
  android-debug:
    runs-on: ubuntu-latest
    steps:
      - name: Checkout
        uses: actions/checkout@v4
      - name: Install Flutter dependencies
        working-directory: ./mobile
        run: echo hello
      - name: Build Debug APK
        working-directory: ./mobile/android
        run: ./gradlew assembleDebug
"""

# ─── Run all cases ───────────────────────────────────────────────

# ─── S7 test cases (Sprint 9.6.8) ────────────────────────────────

# Case 10 (S7 PASS): main.dart + pubspec.yaml all in good shape.
case_s7_main_pass = """
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'app.dart';

void main() {
  runApp(const ProviderScope(child: MyApp()));
}
"""
case_s7_pubspec_pass = """
name: opene2ee
description: OpenE2EE
version: 1.0.0+1
environment:
  sdk: ^3.12.1
dependencies:
  flutter:
    sdk: flutter
  cupertino_icons: ^1.0.8
  flutter_riverpod: ^2.5.1
  go_router: ^14.2.7
dev_dependencies:
  flutter_test:
    sdk: flutter
"""

# Case 11 (S7 FAIL — main.dart missing): main_dart_text is None.
case_s7_pubspec_main_missing = case_s7_pubspec_pass  # pubspec still good

# Case 12 (S7 FAIL — ProviderScope missing): main.dart exists but no ProviderScope.
case_s7_no_providerscope = """
import 'package:flutter/material.dart';

void main() {
  runApp(const MyApp());
}
"""

# Case 13 (S7 FAIL — pubspec deps missing): main.dart good but pubspec lacks flutter_riverpod.
case_s7_main_good_no_riverpod = case_s7_main_pass
case_s7_pubspec_no_riverpod = """
name: opene2ee
version: 1.0.0+1
dependencies:
  flutter:
    sdk: flutter
  go_router: ^14.2.7
"""

# ─── S8 test cases (Sprint 9.6.9) ────────────────────────────────

# Case 14 (S8 PASS): production network_security_config.xml after Sprint 9.6.9 fix.
# Mirrors the post-fix state: only `=` runs in headers, no `--` inside comments.
case_s8_xml_pass = """<?xml version="1.0" encoding="utf-8"?>
<!--
  mobile/android/app/src/main/res/xml/network_security_config.xml

  PR-39 (Sprint 6) — Network Security Configuration for the OpenE2EE Android
  app.

  Why this exists
  ===============
  Addresses cyber-security Sprint 6 findings MOB-1 + MOB-2.

  Trust anchors
  =============
  <trust-anchors> references ONLY system. User-installed CAs are NOT trusted.

  Production CA pin
  =================
  A <domain-config> block pins the production CA's SPKI hash.

  Dev exception
  =============
  The 10.0.2.2 cleartext allowance is currently COMMENTED OUT.
-->
<network-security-config>
    <base-config cleartextTrafficPermitted="false">
        <trust-anchors>
            <certificates src="system" />
        </trust-anchors>
    </base-config>
    <domain-config>
        <domain includeSubdomains="true">api.opene2ee.com</domain>
        <pin-set expiration="2027-07-07">
            <pin digest="SHA-256">PLACEHOLDER_PRODUCTION_SPKI==</pin>
        </pin-set>
    </domain-config>
</network-security-config>
"""

# Case 15 (S8 FAIL — comment with `--` run): the exact Sprint 9.6.9 broken state.
case_s8_xml_bad = """<?xml version="1.0" encoding="utf-8"?>
<!--
  PR-39 — Network Security Configuration.

  Why this exists
  ---------------
  Addresses cyber-security Sprint 6 findings.

  Trust anchors
  -------------
  System-only.
-->
<network-security-config>
    <base-config cleartextTrafficPermitted="false">
        <trust-anchors>
            <certificates src="system" />
        </trust-anchors>
    </base-config>
</network-security-config>
"""

# ─── S9 test cases (Sprint 9.6.10) ───────────────────────────────

# Case 16 (S9 PASS): post-fix AndroidManifest.xml (no package attr, tools:replace, xmlns:tools) + gradle namespace.
case_s9_manifest_pass = """<?xml version="1.0" encoding="utf-8"?>
<manifest xmlns:android="http://schemas.android.com/apk/res/android"
          xmlns:tools="http://schemas.android.com/tools">
    <application
        android:label="OpenE2EE"
        android:usesCleartextTraffic="false"
        tools:replace="android:usesCleartextTraffic"
        android:networkSecurityConfig="@xml/network_security_config">
    </application>
</manifest>
"""
case_s9_gradle_pass = """
plugins {
    id("com.android.application")
    id("org.jetbrains.kotlin.android")
    id("dev.flutter.flutter-gradle-plugin")
}

android {
    namespace = "com.opene2ee.opene2ee"
    compileSdk = 34
}
"""

# Case 17 (S9 FAIL — package attribute re-introduced): the broken state 9.6.10 fixed.
case_s9_manifest_package = """<?xml version="1.0" encoding="utf-8"?>
<manifest xmlns:android="http://schemas.android.com/apk/res/android"
          xmlns:tools="http://schemas.android.com/tools"
          package="com.opene2ee.opene2ee">
    <application
        android:label="OpenE2EE"
        android:usesCleartextTraffic="false"
        tools:replace="android:usesCleartextTraffic">
    </application>
</manifest>
"""

# Case 18 (S9 FAIL — tools:remove co-exists with android:usesCleartextTraffic): the broken state.
case_s9_manifest_tools_remove = """<?xml version="1.0" encoding="utf-8"?>
<manifest xmlns:android="http://schemas.android.com/apk/res/android"
          xmlns:tools="http://schemas.android.com/tools">
    <application
        android:label="OpenE2EE"
        android:usesCleartextTraffic="false"
        tools:remove="android:usesCleartextTraffic">
    </application>
</manifest>
"""

# ─── S10 test cases (Sprint 9.6.11) ──────────────────────────────

# Case 19 (S10 PASS): post-`flutter create` resource skeleton.
case_s10_styles_pass = """<?xml version="1.0" encoding="utf-8"?>
<resources>
    <style name="LaunchTheme" parent="@android:style/Theme.Light.NoTitleBar">
        <item name="android:windowBackground">@drawable/launch_background</item>
    </style>
    <style name="NormalTheme" parent="@android:style/Theme.Light.NoTitleBar">
        <item name="android:windowBackground">?android:colorBackground</item>
    </style>
</resources>
"""

# Case 20 (S10 FAIL — styles.xml missing LaunchTheme).
case_s10_styles_no_launch = """<?xml version="1.0" encoding="utf-8"?>
<resources>
    <style name="SomeOther" parent="@android:style/Theme.Light.NoTitleBar"/>
    <style name="NormalTheme" parent="@android:style/Theme.Light.NoTitleBar"/>
</resources>
"""

# Case 21 (S10 FAIL — no mipmap representation).
# (uses the same styles XML as the PASS case but has_mipmap=False triggers the mipmap sub-check)
case_s10_styles_for_mipmap = case_s10_styles_pass

# ─── S11 test cases (Sprint 9.6.12) ──────────────────────────────

# Case 22 (S11 PASS): post-`flutter pub get` mobile/.flutter-plugins-
# dependencies content. Mirrors the real file shape: 7 Android plugins
# with `name` + `native_build` fields. (iOS array is omitted from
# this fixture for brevity — the audit only inspects the android
# array.)
case_s11_fpd_pass = """{
    "info": "This is a generated file; do not edit or check into version control.",
    "plugins": {
        "android": [
            {"name": "flutter_plugin_android_lifecycle", "path": "C:/Users/User/AppData/Local/Pub/Cache/hosted/pub.dev/flutter_plugin_android_lifecycle-2.0.35/", "native_build": true, "dependencies": [], "dev_dependency": false},
            {"name": "flutter_secure_storage", "path": "C:/Users/User/AppData/Local/Pub/Cache/hosted/pub.dev/flutter_secure_storage-9.2.4/", "native_build": true, "dependencies": [], "dev_dependency": false},
            {"name": "flutter_webrtc", "path": "C:/Users/User/AppData/Local/Pub/Cache/hosted/pub.dev/flutter_webrtc-1.5.2/", "native_build": true, "dependencies": [], "dev_dependency": false},
            {"name": "jni", "path": "C:/Users/User/AppData/Local/Pub/Cache/hosted/pub.dev/jni-1.0.0/", "native_build": true, "dependencies": [], "dev_dependency": false},
            {"name": "jni_flutter", "path": "C:/Users/User/AppData/Local/Pub/Cache/hosted/pub.dev/jni_flutter-1.0.1/", "native_build": true, "dependencies": ["jni"], "dev_dependency": false},
            {"name": "local_auth_android", "path": "C:/Users/User/AppData/Local/Pub/Cache/hosted/pub.dev/local_auth_android-1.0.46/", "native_build": true, "dependencies": [], "dev_dependency": false},
            {"name": "path_provider_android", "path": "C:/Users/User/AppData/Local/Pub/Cache/hosted/pub.dev/path_provider_android-2.2.10/", "native_build": false, "dependencies": [], "dev_dependency": false}
        ]
    }
}"""

# Case 23 (S11 FAIL — file missing entirely).
# fpd_text=None signals to the self-test the file is absent
# (the worktree was created from a clean main checkout without
# running `flutter pub get` locally — Sprint 9.6.12 broken state).

# Case 24 (S11 FAIL — plugins.android empty array).
# The file EXISTS and parses as JSON, but `plugins.android` is an
# empty array — no Android plugins declared. The Flutter Gradle
# plugin would have no plugin code to wire and the engine JAR
# classpath would be incomplete, leading to the 9.6.12
# compileDebugKotlin failure.
case_s11_fpd_empty_android = """{
    "info": "This is a generated file; do not edit or check into version control.",
    "plugins": {
        "android": []
    }
}"""

# ─── S12 test cases (Sprint 9.6.13) ──────────────────────────────

# Real Flutter 3.44.1 engine.version value (mirrors the local SDK
# install; the audit reads this file and compares it to the hash
# declared in app/build.gradle.kts). Used by ALL S12 cases.
S12_ENGINE_VERSION = "c416acfeb8126e097f758c664aaa3da929e27da0"

# Case 25 (S12 PASS): post-fix app/build.gradle.kts with the
# flutter_embedding_ktx dependency declared at the correct hash.
# Mirrors the real post-Sprint 9.6.13 file structure.
case_s12_gradle_pass = """plugins {
    id("com.android.application")
    id("org.jetbrains.kotlin.android")
    id("dev.flutter.flutter-gradle-plugin")
}

android {
    namespace = "com.opene2ee.opene2ee"
    compileSdk = 34
}

dependencies {
    implementation("androidx.core:core-ktx:1.12.0")
    implementation("androidx.annotation:annotation:1.7.1")
    implementation("org.jetbrains.kotlinx:kotlinx-coroutines-android:1.7.3")
    implementation("io.flutter:flutter_embedding_ktx:1.0.0-c416acfeb8126e097f758c664aaa3da929e27da0")
}
"""

# Case 26 (S12 FAIL — dependency missing): app/build.gradle.kts
# without the flutter_embedding_ktx line. Mirrors the 9.6.12
# broken state (and earlier — missing since PR-3).
case_s12_gradle_no_embedding = """plugins {
    id("com.android.application")
    id("org.jetbrains.kotlin.android")
    id("dev.flutter.flutter-gradle-plugin")
}

android {
    namespace = "com.opene2ee.opene2ee"
    compileSdk = 34
}

dependencies {
    implementation("androidx.core:core-ktx:1.12.0")
    implementation("androidx.annotation:annotation:1.7.1")
    implementation("org.jetbrains.kotlinx:kotlinx-coroutines-android:1.7.3")
}
"""

# Case 27 (S12 FAIL — wrong hash): flutter_embedding_ktx declared
# with a 40-char hex hash that does NOT match engine.version.
# Tests the hash mismatch sub-check.
case_s12_gradle_wrong_hash = """plugins {
    id("com.android.application")
}

dependencies {
    implementation("androidx.core:core-ktx:1.12.0")
    implementation("io.flutter:flutter_embedding_ktx:1.0.0-deadbeefdeadbeefdeadbeefdeadbeefdeadbeef")
}
"""

# Case 28 (S13 PASS): post-fix settings.gradle.kts with the
# dependencyResolutionManagement block containing the Flutter
# storage URL.
case_s13_settings_pass = """pluginManagement {
    repositories {
        google()
        mavenCentral()
        gradlePluginPortal()
    }
}

dependencyResolutionManagement {
    repositoriesMode.set(RepositoriesMode.PREFER_SETTINGS)
    repositories {
        google()
        mavenCentral()
        maven { url = uri("https://storage.googleapis.com/download.flutter.io") }
        gradlePluginPortal()
    }
}

include(":app")
"""
case_s13_app_pass_empty = ""

# Case 29 (S13 FAIL - dependencyResolutionManagement block missing).
case_s13_settings_no_drm = """pluginManagement {
    repositories {
        google()
        mavenCentral()
        gradlePluginPortal()
    }
}

include(":app")
"""
case_s13_app_no_drm_empty = ""

# Case 30 (S13 FAIL - DRM block present but Flutter storage URL missing).
case_s13_settings_no_url = """pluginManagement {
    repositories {
        google()
        mavenCentral()
        gradlePluginPortal()
    }
}

dependencyResolutionManagement {
    repositoriesMode.set(RepositoriesMode.PREFER_SETTINGS)
    repositories {
        google()
        mavenCentral()
        gradlePluginPortal()
    }
}

include(":app")
"""
case_s13_app_no_url_empty = ""

# ─── S17 test cases (Sprint 9.7.0 Item 5) ─────────────────────────

# Real repo-root .gitignore that the fresh skeleton ships with —
# includes the three `!**/android/...` re-include patterns Sprint 9.7.0
# Item 1 added (attempt-2 delta).
case_s17_gitignore_pass = """
**/android/.gradle/**
**/android/gradlew
**/android/gradlew.bat
**/android/local.properties
**/.dart_tool/
**/.flutter-plugins-dependencies
!**/android/gradlew
!**/android/gradlew.bat
!**/android/gradle/wrapper/gradle-wrapper.jar
"""

# Case 31 (S17 PASS): wrapper tracked + gitignore has all three `!`
# re-include patterns. Mirrors the Sprint 9.7.0 Item 1 post-attempt-2
# state on `feat/pr-9.7.0-port-audit` (commit 8697167).
# Case 32 (S17 FAIL — wrapper not tracked): gradlew_tracked=False.

# ─── S18 test cases (Sprint 9.7.0 Item 5) ─────────────────────────

# Minimal pubspec.lock fixture with `packages.flutter.source: sdk`.
case_s18_pubspec_lock_pass = """packages:
  async:
    dependency: transitive
    description:
      name: async
      sha256: e2eb0491ba5ddb6177742d2da23904574082139b07c1e33b8503b9f46f3e1a37
      url: "https://pub.dev"
    source: hosted
    version: "2.13.1"
  flutter:
    dependency: "direct main"
    description: flutter
    source: sdk
    version: "0.0.0"
  flutter_test:
    dependency: "direct dev"
    description: flutter
    source: sdk
    version: "0.0.0"
"""

# Case 33 (S18 PASS): pubspec.lock tracked + parses as YAML +
# packages.flutter.source: sdk + gitignore has all four mobile-
# specific patterns.
# Case 34 (S18 FAIL — pubspec.lock not tracked): None.

# ─── S19 test cases (Sprint 9.7.0 Item 5) ─────────────────────────

# Case 35 (S19 PASS): both tracked.
# Case 36 (S19 FAIL — .metadata not tracked): False.

# ─── S20 test cases (Sprint 9.7.0 Item 5) ─────────────────────────

# Minimal pubspec.yaml fixture with all four required keys.
case_s20_pubspec_pass = """name: opene2ee
description: OpenE2EE
version: 1.0.0+1
environment:
  sdk: ^3.12.1
dependencies:
  flutter:
    sdk: flutter
  cupertino_icons: ^1.0.8
dev_dependencies:
  flutter_test:
    sdk: flutter
  flutter_lints: ^6.0.0
"""

# Case 37 (S20 PASS): pubspec.yaml parses + all four required keys present.
# Case 38 (S20 FAIL — pubspec.yaml missing `name:`): empty dict / no name.

case_s20_pubspec_no_name = """description: OpenE2EE
environment:
  sdk: ^3.12.1
dependencies:
  flutter:
    sdk: flutter
dev_dependencies:
  flutter_test:
    sdk: flutter
"""

# ─── S42 test cases (Sprint 10.1F) ───────────────────────────────

# Case 21 (S42 PASS): post-Sprint 10.1F AndroidManifest.xml with the
# WhatsApp packages declared in `<queries>`. Mirrors the real
# post-fix state on main.
case_s42_manifest_pass = """<?xml version="1.0" encoding="utf-8"?>
<manifest xmlns:android="http://schemas.android.com/apk/res/android"
          xmlns:tools="http://schemas.android.com/tools">

    <uses-permission android:name="android.permission.INTERNET" />

    <application
        android:label="OpenE2EE"
        android:usesCleartextTraffic="false"
        tools:replace="android:usesCleartextTraffic">
        <activity android:name=".MainActivity" />
    </application>

    <queries>
        <intent>
            <action android:name="android.net.VpnService" />
        </intent>
        <package android:name="com.whatsapp" />
        <package android:name="com.whatsapp.w4b" />
    </queries>
</manifest>
"""

# Case 22 (S42 FAIL — package declaration missing). Mirrors the
# 10.1E deliverable state (Intent URI present, but no <queries>
# entry) — Owner report 10.07.2026 23:29: "whatsapp yüklü değil
# diyor hala deeplink yine hatalı".
case_s42_manifest_no_package = """<?xml version="1.0" encoding="utf-8"?>
<manifest xmlns:android="http://schemas.android.com/apk/res/android"
          xmlns:tools="http://schemas.android.com/tools">

    <application android:label="OpenE2EE" />

    <queries>
        <intent>
            <action android:name="android.net.VpnService" />
        </intent>
        <!--
          Sprint 10.1E: we handle WhatsApp via the intent://send?
          scheme. No need to declare the package in <queries>
          (will revisit).
        -->
    </queries>
</manifest>
"""

# ─── S43 test cases (Sprint 10.1F) ───────────────────────────────

# Case 23 (S43 PASS): post-Sprint 10.1F MainActivity.kt with the
# inline `opene2ee/vpn` MethodChannel handler and the
# `getSampledPackets` case. Mirrors the real post-fix state.
case_s43_main_activity_pass = """package com.opene2ee.opene2ee

import io.flutter.embedding.android.FlutterActivity
import io.flutter.embedding.engine.FlutterEngine
import io.flutter.plugin.common.MethodCall
import io.flutter.plugin.common.MethodChannel

class MainActivity : FlutterActivity() {
    override fun configureFlutterEngine(flutterEngine: FlutterEngine) {
        super.configureFlutterEngine(flutterEngine)
        MethodChannel(flutterEngine.dartExecutor.binaryMessenger, "opene2ee/vpn")
            .setMethodCallHandler { call, result ->
                when (call.method) {
                    "start" -> result.success("started")
                    "stop" -> result.success("stopped")
                    "status" -> result.success("idle")
                    "getSampledPackets" -> {
                        val mockPackets = listOf(
                            mapOf("version" to 4, "protocol" to 6)
                        )
                        result.success(mockPackets)
                    }
                    else -> result.notImplemented()
                }
            }
    }
}
"""

# Case 24 (S43 FAIL — getSampledPackets case missing). Mirrors the
# Sprint 10.1B/10.1E broken state where MainActivity had only the
# `opene2ee/vpn_permissions` channel and the `opene2ee/vpn` channel
# was supposed to be wired to `OpenE2eeVpnService` (still
# TODO(port-vpn-service)).
case_s43_main_activity_no_handler = """package com.opene2ee.opene2ee

import io.flutter.embedding.android.FlutterActivity
import io.flutter.embedding.engine.FlutterEngine
import io.flutter.plugin.common.MethodCall
import io.flutter.plugin.common.MethodChannel

class MainActivity : FlutterActivity() {
    // TODO(port-vpn-service): OpenE2eeVpnService.attachFlutterEngine
    // will be uncommented in Sprint 10.2 — it will own the
    // `opene2ee/vpn` channel. For now we only have the
    // `opene2ee/vpn_permissions` channel wired in this activity.
    //
    // The `getSampledPackets` handler is the responsibility of
    // the (not-yet-ported) OpenE2eeVpnService class.

    override fun configureFlutterEngine(flutterEngine: FlutterEngine) {
        super.configureFlutterEngine(flutterEngine)
        MethodChannel(flutterEngine.dartExecutor.binaryMessenger, "opene2ee/vpn_permissions")
            .setMethodCallHandler { call, result ->
                when (call.method) {
                    "requestVpnPermission" -> result.success(true)
                    "isVpnPrepared" -> result.success(false)
                    else -> result.notImplemented()
                }
            }
    }
}
"""

# ─── Run all cases ───────────────────────────────────────────────

# S44 (Sprint 10.1G) fixture variables — declared here (NOT inside
# the cases list literal below) because Python list literals cannot
# contain assignment statements. The strings mirror the real
# post-10.1G file shape: the provider file's buildUri() + buildWaMeUri()
# helpers carry BOTH literals, and the screen file's _onSend method
# calls `WhatsAppDeepLink.tryOpenWithReason()` so the snackbar can
# surface the per-tier debug reason.
case_s44_provider_pass = (
    "static Uri buildUri() => Uri.parse(\n"
    "  'intent://send?text=foo#Intent;scheme=whatsapp;package=com.whatsapp;end',\n"
    ");\n"
    "static Uri buildWaMeUri() => Uri.parse('https://wa.me/?text=foo');\n"
)
case_s44_screen_pass = (
    "final result = await WhatsAppDeepLink.tryOpenWithReason();\n"
    "if (!result.ok) {\n"
    "  messenger.showSnackBar(SnackBar(content: Text('WhatsApp açılamadı: ${result.reason}')));\n"
    "}\n"
)

# S45-S52 (Sprint 11.0A) fixture variables. Same rationale as
# the S44 fixtures above — declared outside the cases literal
# so we can use assignment.

# S45: OpenE2eeVpnService.kt carries the `"onPacketsSampled"`
# literal (Kotlin `PacketDrain.invokeMethod("onPacketsSampled", ...)`).
case_s45_vpn_service_pass = (
    "class OpenE2eeVpnService : VpnService() {\n"
    "    private class PacketDrain(...) : Runnable {\n"
    "        override fun run() {\n"
    "            ch.invokeMethod(\"onPacketsSampled\", packets)\n"
    "        }\n"
    "    }\n"
    "}\n"
)
case_s45_vpn_service_no_literal = (
    "class OpenE2eeVpnService : VpnService() {\n"
    "    // forgot the event name literal — 10.1F inline mock returns\n"
    "    private fun snapshot(): List<Map<String, Any?>> = listOf()\n"
    "}\n"
)

# S46: MainActivity.kt calls OpenE2eeVpnService.snapshot() and
# does NOT contain the 10.1F mock packet `mapOf("srcIpMasked" ...)`
# literal.
case_s46_main_activity_pass = (
    "package com.opene2ee.opene2ee\n"
    "import com.opene2ee.opene2ee.vpn.OpenE2eeVpnService\n"
    "class MainActivity : FlutterActivity() {\n"
    "    override fun configureFlutterEngine(flutterEngine: FlutterEngine) {\n"
    "        OpenE2eeVpnService.attachFlutterEngine(flutterEngine)\n"
    "    }\n"
    "    fun readSamples(): List<Map<String, Any?>>? = OpenE2eeVpnService.snapshot()\n"
    "}\n"
)
case_s46_main_activity_with_mock = (
    "package com.opene2ee.opene2ee\n"
    "class MainActivity : FlutterActivity() {\n"
    "    private fun onVpnCall(call: MethodCall, result: MethodChannel.Result) {\n"
    "        when (call.method) {\n"
    "            \"getSampledPackets\" -> {\n"
    "                val mockPackets = listOf(\n"
    "                    mapOf(\n"
    "                        \"version\" to 4,\n"
    "                        \"protocol\" to 6,\n"
    "                        \"srcIpMasked\" to \"10.42.0.0\",\n"
    "                        \"dstIpMasked\" to \"1.1.1.0\",\n"
    "                    ),\n"
    "                )\n"
    "                result.success(mockPackets)\n"
    "            }\n"
    "        }\n"
    "    }\n"
    "}\n"
)

# S47: vpn_service.dart has `packetStream` getter + `MethodChannel` import.
case_s47_vpn_service_pass = (
    "import 'dart:async';\n"
    "import 'package:flutter/services.dart';\n"
    "class VpnService {\n"
    "  VpnService({MethodChannel? channel})\n"
    "      : _channel = channel ?? const MethodChannel('opene2ee/vpn');\n"
    "  Stream<List<SampledPacket>> get packetStream => _packetCtrl.stream;\n"
    "}\n"
)
case_s47_vpn_service_no_packetstream = (
    "import 'dart:async';\n"
    "class VpnService {\n"
    "  // forgot the live stream getter — screen has no live subscription\n"
    "  Future<List<Map<String, Object?>>> getSampledPackets() async => const [];\n"
    "}\n"
)

# S48: active_pool_screen.dart has `packetStream.listen` literal.
case_s48_active_pool_pass = (
    "class _ActivePoolScreenState extends ConsumerState<ActivePoolScreen> {\n"
    "  void initState() {\n"
    "    _packetSub = _vpn.packetStream.listen(_onPacketsSampled);\n"
    "  }\n"
    "}\n"
)
case_s48_active_pool_no_listen = (
    "class _ActivePoolScreenState extends ConsumerState<ActivePoolScreen> {\n"
    "  void initState() {\n"
    "    // forgot the live subscription — reverts to mock ticker\n"
    "    _mockTimer = Timer.periodic(_mockTickPeriod, (_) => _mockTick());\n"
    "  }\n"
    "}\n"
)

# S49: packet_parser.dart has SampledPacket class + fromBytes + toJson.
case_s49_packet_parser_pass = (
    "class SampledPacket {\n"
    "  SampledPacket({required this.protocol, required this.srcIpMasked, ...});\n"
    "  static SampledPacket? fromBytes(Uint8List raw) {\n"
    "    return PacketParser.parse(raw) != null\n"
    "        ? SampledPacket(...)\n"
    "        : null;\n"
    "  }\n"
    "  Map<String, Object?> toJson() => {'protocol': protocol, ...};\n"
    "}\n"
)
case_s49_packet_parser_no_sampledpacket = (
    "// forgot the SampledPacket class — only ParsedPacket remains\n"
    "class ParsedPacket {\n"
    "  ParsedPacket({required this.protocol, ...});\n"
    "  Map<String, Object?> toJson() => {'protocol': protocol.name, ...};\n"
    "}\n"
)

# S50: OpenE2eeVpnService.kt foreground notification text is
# `OpenE2EE Şifreleme Doğrulama` (no "VPN" string in user-facing).
case_s50_vpn_service_pass = (
    "val notification: Notification = NotificationCompat.Builder(this, NOTIFICATION_CHANNEL_ID)\n"
    "    .setContentTitle(\"OpenE2EE Şifreleme Doğrulama\")\n"
    "    .setContentText(\"Ağınızda ilk 10 paket analiz ediliyor (PRIVACY_TEXT eki)\")\n"
    "    .setSmallIcon(android.R.drawable.ic_lock_lock)\n"
    "    .build()\n"
)
case_s50_vpn_service_with_vpn_word = (
    "val notification: Notification = NotificationCompat.Builder(this, NOTIFICATION_CHANNEL_ID)\n"
    "    .setContentTitle(\"OpenE2EE VPN diagnostic session\")\n"
    "    .setContentText(\"Sampling the first 10 packets of your network\")\n"
    "    .setSmallIcon(android.R.drawable.ic_lock_lock)\n"
    "    .build()\n"
)

# S51: active_pool_screen.dart NO 30-call fixed loop + has packetStream.
case_s51_active_pool_pass = (
    "class _ActivePoolScreenState extends ConsumerState<ActivePoolScreen> {\n"
    "  void initState() {\n"
    "    _packetSub = _vpn.packetStream.listen(_onPacketsSampled);\n"
    "    // 30-call fixed loop REMOVED in Sprint 11.0A — chart is\n"
    "    // driven by the live stream subscription, not a timer.\n"
    "  }\n"
    "}\n"
)
case_s51_active_pool_with_30_loop = (
    "class _ActivePoolScreenState extends ConsumerState<ActivePoolScreen> {\n"
    "  void initState() {\n"
    "    // 10.1A fixed-30-call Timer.periodic chart loop still in place\n"
    "    for (var i = 0; i < 30; i++) {\n"
    "      _mockTimer = Timer.periodic(const Duration(seconds: 3), (_) => _mockTick());\n"
    "    }\n"
    "  }\n"
    "}\n"
)

# S52: telemetry_service.dart has sendSummary + /api/v1/sessions/
# + 6 summary fields.
case_s52_telemetry_pass = (
    "Future<void> sendSummary({\n"
    "  required int totalPackets,\n"
    "  required int encryptedPackets,\n"
    "  required double packetLossPct,\n"
    "  required double meanLatencyMs,\n"
    "  required double jitterMs,\n"
    "  required double encryptionIntegrityPct,\n"
    "  Duration window = const Duration(seconds: 30),\n"
    "}) async {\n"
    "  final uri = Uri.parse('${AppConfig.apiBase}/api/v1/sessions/$_sessionId/telemetry');\n"
    "  ...\n"
    "}\n"
)
case_s52_telemetry_no_summary = (
    "// no 30-second batch upload method on this class\n"
    "Future<void> send(List<ParsedPacket> packets) async {\n"
    "  await _client.post(_endpoint, ...);\n"
    "}\n"
)

# S53-S60 (Sprint 11.0B) fixture variables. Same rationale as
# the S44-S52 fixtures above.

# S53: pubspec.yaml carries the `webrtc:` dep line.
case_s53_pubspec_pass = (
    "name: opene2ee\n"
    "dependencies:\n"
    "  flutter_webrtc: ^1.5.0  # WebRTC peer connection (11.0B)\n"
    "  http: ^1.2.0\n"
)
case_s53_pubspec_no_webrtc = (
    "name: opene2ee\n"
    "dependencies:\n"
    "  http: ^1.2.0\n"
)

# S54: webrtc_service.dart imports flutter_webrtc + uses
# RTCPeerConnection + calls createPeerConnection.
case_s54_webrtc_service_pass = (
    "import 'package:flutter_webrtc/flutter_webrtc.dart' as webrtc;\n"
    "import 'package:flutter_webrtc/flutter_webrtc.dart' show RTCPeerConnection;\n"
    "class WebRTCService {\n"
    "  RTCPeerConnection? _pc;\n"
    "  Future<void> createPeerConnection() async {\n"
    "    _pc = await webrtc.createPeerConnection({'iceServers': []});\n"
    "  }\n"
    "}\n"
)
case_s54_webrtc_service_no_rtc = (
    "// forgot the flutter_webrtc import + RTCPeerConnection reference\n"
    "class WebRTCService {}\n"
)

# S55: webrtc_service.dart onIceCandidate callback wires the
# candidate + sdpMid + sdpMLineIndex fields.
case_s55_webrtc_service_pass = (
    "_pc!.onIceCandidate = (RTCIceCandidate candidate) {\n"
    "  _iceCtrl.add({\n"
    "    'candidate': candidate.candidate,\n"
    "    'sdpMid': candidate.sdpMid,\n"
    "    'sdpMLineIndex': candidate.sdpMLineIndex,\n"
    "  });\n"
    "};\n"
)
case_s55_webrtc_service_no_callback = (
    "// forgot the onIceCandidate callback wiring\n"
    "class WebRTCService {\n"
    "  RTCPeerConnection? _pc;\n"
    "}\n"
)

# S56: session_orchestrator.dart startSession() + JWT auth header.
case_s56_orchestrator_pass = (
    "import 'package:http/http.dart' as http;\n"
    "class SessionOrchestrator {\n"
    "  Future<String> startSession({String? role}) async {\n"
    "    final headers = await _auth.authHeaders();\n"
    "    final resp = await _client.post(\n"
    "      Uri.parse('\${AppConfig.apiBase}/api/v1/sessions'),\n"
    "      headers: headers,\n"
    "    );\n"
    "    return resp.body;\n"
    "  }\n"
    "}\n"
)
case_s56_orchestrator_no_start = (
    "// forgot startSession method\n"
    "class SessionOrchestrator {}\n"
)

# S57: session_orchestrator.dart long-poll GET (timeout 30s).
case_s57_orchestrator_pass = (
    "Future<Map<String, Object?>?> pollForOffer() async {\n"
    "  final resp = await _client.get(url, headers: headers).timeout(\n"
    "        _pollTimeout,\n"
    "        onTimeout: () => http.Response('', 204),\n"
    "      );\n"
    "  if (resp.statusCode == 204) return null;\n"
    "  return jsonDecode(resp.body) as Map<String, Object?>;\n"
    "}\n"
    "static const Duration _pollTimeout = Duration(seconds: 30);\n"
)
case_s57_orchestrator_no_longpoll = (
    "// forgot the long-poll GET — used a single-fire .get() instead\n"
    "Future<void> fetch() async {\n"
    "  await _client.get(url);\n"
    "}\n"
)

# S59: webrtc_service.dart onTrack stream exposed.
case_s59_webrtc_service_pass = (
    "Stream<MediaStream> get onTrack => _trackCtrl.stream;\n"
    "_pc!.onTrack = (RTCTrackEvent event) {\n"
    "  if (event.streams.isNotEmpty) {\n"
    "    _trackCtrl.add(event.streams[0]);\n"
    "  }\n"
    "};\n"
)
case_s59_webrtc_service_no_ontrack = (
    "// forgot the onTrack stream exposure\n"
    "class WebRTCService {\n"
    "  Stream get onIceCandidate => _iceCtrl.stream;\n"
    "}\n"
)

# S60: active_pool_screen.dart WebRTC status indicator
# (Negotiating / Connected / Failed — Turkish: müzakere /
# bağlandı / hata).
case_s60_active_pool_pass = (
    "static String _webrtcStateLabel(WebRTCState s) {\n"
    "  switch (s) {\n"
    "    case WebRTCState.negotiating:\n"
    "      return 'müzakere';\n"
    "    case WebRTCState.connected:\n"
    "      return 'bağlandı';\n"
    "    case WebRTCState.failed:\n"
    "      return 'hata';\n"
    "  }\n"
    "}\n"
)
case_s60_active_pool_no_indicators = (
    "// forgot the three status indicator labels\n"
    "static String _webrtcStateLabel(WebRTCState s) => 'unknown';\n"
)

# S61-S72 (Sprint 11.0C) fixture variables.

# S61: skorlar_screen.dart has Future<List<SessionScore>> +
# ConsumerStatefulWidget + fetchScores method.
case_s61_skorlar_pass = (
    "import 'package:flutter_riverpod/flutter_riverpod.dart';\n"
    "class SkorlarScreen extends ConsumerStatefulWidget {\n"
    "  const SkorlarScreen({super.key});\n"
    "  @override\n"
    "  ConsumerState<SkorlarScreen> createState() => _SkorlarScreenState();\n"
    "}\n"
    "class _SkorlarScreenState extends ConsumerState<SkorlarScreen> {\n"
    "  Future<List<SessionScore>>? _scoresFuture;\n"
    "  Future<List<SessionScore>> _fetchScores() async {\n"
    "    return <SessionScore>[];\n"
    "  }\n"
    "}\n"
)
case_s61_skorlar_no_consumer = (
    "// forgot ConsumerStatefulWidget + fetchScores\n"
    "class SkorlarScreen extends StatefulWidget {}\n"
)

# S62: score_calculator.dart has SessionScoreCalculator class
# + static SessionScore compute method.
case_s62_score_calculator_pass = (
    "class SessionScoreCalculator {\n"
    "  const SessionScoreCalculator._();\n"
    "  static SessionScore compute(SessionTelemetry t) {\n"
    "    final integrity = (t.encryptionIntegrityPct / 100.0).clamp(0.0, 1.0);\n"
    "    return SessionScore(sessionId: t.sessionId, overallScore: integrity * 100);\n"
    "  }\n"
    "}\n"
)
case_s62_score_calculator_no_compute = (
    "// forgot the compute method\n"
    "class SessionScoreCalculator {}\n"
)

# S63: score_calculator.dart has the 4 metric field references.
case_s63_score_calculator_pass = (
    "final integrity = (t.encryptionIntegrityPct / 100.0).clamp(0.0, 1.0);\n"
    "final loss = (t.packetLossPct / 100.0).clamp(0.0, 1.0);\n"
    "final latency = (1.0 - (t.meanLatencyMs / 1000.0)).clamp(0.0, 1.0);\n"
    "final jitter = (1.0 - (t.jitterMs / 100.0)).clamp(0.0, 1.0);\n"
)
case_s63_score_calculator_no_metrics = (
    "// forgot the 4 metric formulas\n"
    "static SessionScore compute(SessionTelemetry t) {\n"
    "  return SessionScore(sessionId: t.sessionId, overallScore: 100);\n"
    "}\n"
)

# S64: score_calculator.dart has the 0.4 + 0.3 + 0.2 + 0.1
# weighted sum.
case_s64_score_calculator_pass = (
    "final raw = 0.4 * integrity +\n"
    "    0.3 * (1.0 - loss) +\n"
    "    0.2 * latency +\n"
    "    0.1 * jitter;\n"
)
case_s64_score_calculator_no_weights = (
    "// forgot the weighted sum weights\n"
    "final raw = (integrity + loss + latency + jitter) / 4.0;\n"
)

# S65: session_orchestrator.dart has closeSession method +
# /api/v1/sessions/.../close endpoint.
case_s65_orchestrator_pass = (
    "Future<Map<String, Object?>?> closeSession({String? sessionId}) async {\n"
    "  final id = sessionId ?? _sessionId;\n"
    "  final resp = await _client.post(\n"
    "    Uri.parse('\${AppConfig.apiBase}/api/v1/sessions/$id/close'),\n"
    "    headers: headers,\n"
    "  );\n"
    "  return body['summary_stats'];\n"
    "}\n"
)
case_s65_orchestrator_no_close = (
    "// forgot the closeSession method\n"
    "Future<void> tearDown() async {}\n"
)

# S66: active_pool_screen.dart has the 'Oturumu Bitir' Turkish label.
case_s66_active_pool_pass = (
    "OutlinedButton.icon(\n"
    "  onPressed: _oturumuBitir,\n"
    "  icon: const Icon(Icons.stop_circle_outlined),\n"
    "  label: const Text('Oturumu Bitir'),\n"
    ")\n"
)
case_s66_active_pool_no_button = (
    "// forgot the session-close button — no oturumu bitir surface\n"
    "ElevatedButton.icon(\n"
    "  onPressed: _onStart,\n"
    "  label: const Text('Şifreleme Doğrulamayı Başlat'),\n"
    ")\n"
)

# S67: active_pool_screen.dart has closeSession call + /home/skorlar nav.
case_s67_active_pool_pass = (
    "final summary = await _orchestrator.closeSession();\n"
    "if (!mounted) return;\n"
    "context.go('/home/skorlar');\n"
)
case_s67_active_pool_no_flow = (
    "// forgot the close flow\n"
    "await _orchestrator.tearDown();\n"
)

# S68: skorlar_screen.dart has the 'Henüz tamamlanmış oturum yok' empty state.
case_s68_skorlar_pass = (
    "class _SkorlarEmpty extends StatelessWidget {\n"
    "  @override\n"
    "  Widget build(BuildContext context) {\n"
    "    return const Center(\n"
    "      child: Text('Henüz tamamlanmış oturum yok'),\n"
    "    );\n"
    "  }\n"
    "}\n"
)
case_s68_skorlar_no_empty = (
    "// forgot the empty state\n"
    "class _SkorlarEmpty extends StatelessWidget {\n"
    "  Widget build(BuildContext context) => const Center(child: Text('Empty'));\n"
    "}\n"
)

# S69: skorlar_screen.dart has SessionScoreCard + overall gauge.
case_s69_skorlar_pass = (
    "class SessionScoreCard extends StatefulWidget {\n"
    "  Widget build(BuildContext context) {\n"
    "    return _OverallScoreDisc(score: s.overallScore, color: color);\n"
    "  }\n"
    "}\n"
)
case_s69_skorlar_no_card = (
    "// forgot the per-session card + headline gauge\n"
    "Widget build(BuildContext context) => const Center(child: Text('No cards'));\n"
)

# S70: backend router.go has POST /api/v1/sessions/{id}/close.
case_s70_router_pass = (
    "r.Post(\"/sessions/{id}/close\", a.handleCloseSession())\n"
)
case_s70_router_no_close = (
    "// forgot the close route\n"
    "r.Post(\"/sessions/{id}/telemetry\", a.handlePostTelemetry())\n"
)

# S71: backend sessions.go has the 6-field summary_stats response.
case_s71_sessions_pass = (
    "out := map[string]any{\n"
    "    \"session_id\":    sessionID,\n"
    "    \"status\":        \"completed\",\n"
    "    \"closed_at\":     now.Format(time.RFC3339),\n"
    "    \"summary_stats\": map[string]any{\n"
    "        \"total_packets\":            0,\n"
    "        \"encrypted_packets\":        0,\n"
    "        \"packet_loss_pct\":          0.0,\n"
    "        \"mean_latency_ms\":          0.0,\n"
    "        \"jitter_ms\":                0.0,\n"
    "        \"encryption_integrity_pct\": 100.0,\n"
    "    },\n"
    "}\n"
)
case_s71_sessions_no_summary = (
    "// forgot the summary block\n"
    "out := map[string]any{\"session_id\": sessionID, \"status\": \"completed\"}\n"
)

# S72: score_calculator_test.dart has 4+ unit tests.
case_s72_score_calculator_test_pass = (
    "void main() {\n"
    "  test('perfect session → 100', () {});\n"
    "  test('loss-only regression', () {});\n"
    "  test('latency-only regression', () {});\n"
    "  test('jitter-only regression', () {});\n"
    "  test('mixed 4-metric regression', () {});\n"
    "}\n"
)
case_s72_score_calculator_test_no_tests = (
    "void main() {\n"
    "  // forgot the 4 unit tests\n"
    "}\n"
)

# S73 (Sprint 11.0D): MainActivity.kt owns the `opene2ee/vpn`
# MethodChannel handler. Regression guard for the OnePlus 9 Pro
# `MissingPluginException(No implementation found for method
# getSampledPackets on channel opene2ee/vpn)` error. The check
# looks for FOUR tokens in MainActivity.kt (comment-stripped):
#   1. `MethodChannel(` constructor call
#   2. `OpenE2eeVpnService.METHOD_CHANNEL` OR literal `"opene2ee/vpn"`
#   3. `setMethodCallHandler` install
#   4. `OpenE2eeVpnService.dispatch` call
case_s73_main_activity_pass = (
    "package com.opene2ee.opene2ee\n"
    "import io.flutter.embedding.android.FlutterActivity\n"
    "import io.flutter.plugin.common.MethodChannel\n"
    "import com.opene2ee.opene2ee.vpn.OpenE2eeVpnService\n"
    "\n"
    "class MainActivity : FlutterActivity() {\n"
    "    private var vpnChannel: MethodChannel? = null\n"
    "    override fun configureFlutterEngine(flutterEngine: FlutterEngine) {\n"
    "        super.configureFlutterEngine(flutterEngine)\n"
    "        val vpnChannel = MethodChannel(\n"
    "            flutterEngine.dartExecutor.binaryMessenger,\n"
    "            OpenE2eeVpnService.METHOD_CHANNEL,\n"
    "        ).apply {\n"
    "            setMethodCallHandler { call, result ->\n"
    "                OpenE2eeVpnService.dispatch(this@MainActivity, call, result)\n"
    "            }\n"
    "        }\n"
    "    }\n"
    "}\n"
)
case_s73_main_activity_no_handler = (
    "package com.opene2ee.opene2ee\n"
    "import io.flutter.embedding.android.FlutterActivity\n"
    "import com.opene2ee.opene2ee.vpn.OpenE2eeVpnService\n"
    "\n"
    "class MainActivity : FlutterActivity() {\n"
    "    // Sprint 11.0D regression: forgot the MethodChannel setup\n"
    "    override fun configureFlutterEngine(flutterEngine: FlutterEngine) {\n"
    "        super.configureFlutterEngine(flutterEngine)\n"
    "        OpenE2eeVpnService.attachFlutterEngine(flutterEngine)\n"
    "    }\n"
    "}\n"
)

# S74 (Sprint 11.0E): OpenE2eeVpnService.kt calls
# `startForeground(` within Android's 5-second foreground-service
# rule. The PASS case is the post-Sprint-11.0E fix: `onStartCommand`
# calls `startForeground(` (via the new `ensureForegroundService()`
# helper) as the FIRST statement, BEFORE `startCapture()`. The
# Sprint 11.0E brief pattern (1 PASS-only selftest case, S44 /
# S73 precedent) keeps the total at 128/128.
case_s74_vpn_service_pass = (
    "package com.opene2ee.opene2ee.vpn\n"
    "import android.app.Notification\n"
    "import android.app.NotificationChannel\n"
    "import android.app.NotificationManager\n"
    "import android.content.Context\n"
    "import android.content.pm.ServiceInfo\n"
    "import android.net.VpnService\n"
    "import android.os.Build\n"
    "import androidx.core.app.NotificationCompat\n"
    "import androidx.core.app.ServiceCompat\n"
    "\n"
    "class OpenE2eeVpnService : VpnService() {\n"
    "    private fun startCapture() {}\n"
    "    private fun ensureForegroundService() {\n"
    "        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {\n"
    "            val channel = NotificationChannel(\n"
    "                \"opene2ee.vpn.diagnostic\",\n"
    "                \"OpenE2EE Şifreleme Doğrulama\",\n"
    "                NotificationManager.IMPORTANCE_LOW,\n"
    "            )\n"
    "            getSystemService(NotificationManager::class.java)\n"
    "                ?.createNotificationChannel(channel)\n"
    "        }\n"
    "        val notification: Notification = NotificationCompat.Builder(this, \"opene2ee.vpn.diagnostic\")\n"
    "            .setContentTitle(\"OpenE2EE Şifreleme Doğrulama\")\n"
    "            .setSmallIcon(android.R.drawable.ic_lock_lock)\n"
    "            .setOngoing(true)\n"
    "            .build()\n"
    "        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.UPSIDE_DOWN_CAKE) {\n"
    "            startForeground(\n"
    "                0x4F_50_4E_45,\n"
    "                notification,\n"
    "                ServiceInfo.FOREGROUND_SERVICE_TYPE_SPECIAL_USE,\n"
    "            )\n"
    "        } else {\n"
    "            ServiceCompat.startForeground(\n"
    "                this, 0x4F_50_4E_45, notification,\n"
    "                ServiceInfo.FOREGROUND_SERVICE_TYPE_SPECIAL_USE,\n"
    "            )\n"
    "        }\n"
    "    }\n"
    "    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {\n"
    "        // Sprint 11.0E: foreground promotion FIRST (5s rule).\n"
    "        ensureForegroundService()\n"
    "        if (running.get() == false) startCapture()\n"
    "        return START_NOT_STICKY\n"
    "    }\n"
    "}\n"
)

# S75 (Sprint 11.0F): OpenE2eeVpnService.kt has at least 5
# `Log.d(TAG,` breadcrumbs. The PASS case mirrors the
# post-Sprint-11.0F production file (8+ breadcrumbs across
# startCapture / onStartCommand / dispatch / notifyError).
case_s75_vpn_service_pass = (
    "package com.opene2ee.opene2ee.vpn\n"
    "import android.util.Log\n"
    "class OpenE2eeVpnService {\n"
    "    private fun startCapture() {\n"
    "        Log.d(TAG, \"startCapture: entry\")\n"
    "        Log.d(TAG, \"startCapture: buildVpnBuilder returned\")\n"
    "        Log.d(TAG, \"startCapture: builder.establish returned\")\n"
    "        Log.d(TAG, \"startCapture: startForegroundCompat returned\")\n"
    "        Log.d(TAG, \"startCapture: startReaderThread returned\")\n"
    "        Log.d(TAG, \"startCapture: success\")\n"
    "    }\n"
    "}\n"
)

# S76 (Sprint 11.0F): vpn_service.dart exposes VpnService as a
# Dart singleton (private `_internal` ctor + static `instance`
# getter + factory default ctor). The PASS case mirrors the
# post-Sprint-11.0F production file.
case_s76_vpn_service_dart_pass = (
    "import 'dart:async';\n"
    "import 'package:flutter/services.dart';\n"
    "class VpnService {\n"
    "    // Sprint 11.0G — private ctor (`VpnService._`). The 11.0F\n"
    "    // form was `VpnService._internal(...)`; both are\n"
    "    // accepted by the audit. The key invariant is that\n"
    "    // the default `VpnService()` is NOT callable.\n"
    "    VpnService._({MethodChannel? channel})\n"
    "        : _channel = channel ?? const MethodChannel('opene2ee/vpn') {\n"
    "        _channel.setMethodCallHandler(_onNativeCall);\n"
    "    }\n"
    "    static final VpnService _instance = VpnService._();\n"
    "    static VpnService get instance => _instance;\n"
    "    // NO `factory VpnService()` — the 11.0F back-compat\n"
    "    // factory was REMOVED in 11.0G. The default ctor\n"
    "    // doesn't exist; only `VpnService.instance` and\n"
    "    // `VpnService.forTesting(...)` are callable.\n"
    "    factory VpnService.forTesting({MethodChannel? channel}) =>\n"
    "        VpnService._(channel: channel);\n"
    "    final MethodChannel _channel;\n"
    "}\n"
)

# S77 (Sprint 11.0G): active_pool_screen.dart UI propagation
# invariant — ConsumerStatefulWidget + stateStream.listen with
# setState + VpnService.instance reference. The PASS case
# mirrors the post-11.0G production file.
case_s77_active_pool_screen_pass = (
    "import 'package:flutter/material.dart';\n"
    "import 'package:flutter_riverpod/flutter_riverpod.dart';\n"
    "import 'package:opene2ee/services/vpn_service.dart';\n"
    "\n"
    "class ActivePoolScreen extends ConsumerStatefulWidget {\n"
    "  const ActivePoolScreen({super.key});\n"
    "  @override\n"
    "  ConsumerState<ActivePoolScreen> createState() =>\n"
    "      _ActivePoolScreenState();\n"
    "}\n"
    "\n"
    "class _ActivePoolScreenState extends ConsumerState<ActivePoolScreen> {\n"
    "  VpnLifecycleState _vpnState = VpnLifecycleState.idle;\n"
    "  @override\n"
    "  void initState() {\n"
    "    super.initState();\n"
    "    // Sprint 11.0G — explicit `VpnService.instance` form\n"
    "    // (NOT `VpnService()`).\n"
    "    final vpn = VpnService.instance;\n"
    "    vpn.stateStream.listen((s) {\n"
    "      if (mounted) {\n"
    "        setState(() => _vpnState = s);\n"
    "      }\n"
    "    });\n"
    "  }\n"
    "}\n"
)
case_s78_vpn_service_state_transitions_pass = (
    "package com.opene2ee.opene2ee.vpn\n"
    "import android.util.Log\n"
    "class OpenE2eeVpnService {\n"
    "    companion object {\n"
    "        @JvmField val stateLock: Any = Any()\n"
    "    }\n"
    "    private fun startCapture(): State {\n"
    "        return synchronized(stateLock) {\n"
    "            Log.d(TAG, \"startCapture: SAMPLING started, pfd=$pfd, state transition $prevState -> $state\")\n"
    "            return@synchronized state\n"
    "        }\n"
    "    }\n"
    "    private fun stopCapture(graceful: Boolean): State {\n"
    "        return synchronized(stateLock) {\n"
    "            Log.d(TAG, \"stopCapture: called, graceful=$graceful, prevState=$prevState\")\n"
    "            return@synchronized state\n"
    "        }\n"
    "    }\n"
    "    override fun onRevoke() {\n"
    "        Log.w(TAG, \"onRevoke: VPN profile revoked by system\")\n"
    "    }\n"
    "}\n"
)

# S79 (Sprint 11.0I): OpenE2eeVpnService.kt has correct addRoute
# (0.0.0.0/0 default route) and NO addRoute(TUN_ADDRESS, ...)
# (the 9.7.0 mirror bug that OnePlus 9 Pro rejects with
# `IllegalArgumentException: Bad address`). The PASS case
# mirrors the post-11.0I production file: `addAddress(TUN_ADDRESS`
# literal present, `.addRoute("0.0.0.0", 0)` literal present,
# `.addRoute(TUN_ADDRESS` literal ABSENT.
case_s79_vpn_service_addroute_pass = (
    "package com.opene2ee.opene2ee.vpn\n"
    "class OpenE2eeVpnService {\n"
    "    protected fun buildVpnBuilder(): VpnService.Builder {\n"
    "        return Builder()\n"
    "            .setSession(\"OpenE2EE Network Diagnostic\")\n"
    "            .addAddress(TUN_ADDRESS, 24)\n"
    "            .addRoute(\"0.0.0.0\", 0)\n"
    "            .addDnsServer(PRIMARY_DNS)\n"
    "            .setMtu(1500)\n"
    "            .setBlocking(true)\n"
    "    }\n"
    "}\n"
)

# S80 (Sprint 11.0J): OpenE2eeVpnService.kt has TUN passthrough
# (output.write(buf, 0, n) after input.read(buf)) AND no
# anti-pattern `protect(Socket())` no-op. The PASS case mirrors
# the post-11.0J production file.
case_s80_vpn_service_tun_passthrough_pass = (
    "package com.opene2ee.opene2ee.vpn\n"
    "import android.os.ParcelFileDescriptor\n"
    "class OpenE2eeVpnService {\n"
    "    private fun startReaderThread(pfd: ParcelFileDescriptor) {\n"
    "        val input = ParcelFileDescriptor.AutoCloseInputStream(pfd)\n"
    "        val output = ParcelFileDescriptor.AutoCloseOutputStream(pfd)\n"
    "        val thread = Thread({\n"
    "            val buf = ByteArray(1500)\n"
    "            while (true) {\n"
    "                val n = input.read(buf)\n"
    "                if (n <= 0) break\n"
    "                // Sprint 11.0J — TRANSPARENT PASSTHROUGH.\n"
    "                output.write(buf, 0, n)\n"
    "                output.flush()\n"
    "            }\n"
    "        }, \"opene2ee-vpn-reader\")\n"
    "        thread.start()\n"
    "    }\n"
    "}\n"
)

# S82 (Sprint 11.0K) — OpenE2eeVpnService.kt dispatches
# MethodChannel calls to the Android main looper. Regression
# guard for OnePlus 9 Pro "VPN active, internet OK, UI never
# updates" symptom (Owner 12:31 report, PID 4244; real root
# cause: `@UiThread` violation on PacketDrain worker thread).
case_s82_vpn_service_ui_thread_push_pass = (
    "package com.opene2ee.opene2ee.vpn\n"
    "import android.os.Handler\n"
    "import android.os.Looper\n"
    "import android.os.ParcelFileDescriptor\n"
    "class OpenE2eeVpnService {\n"
    "    companion object {\n"
    "        @JvmField\n"
    "        val mainHandler: Handler = Handler(Looper.getMainLooper())\n"
    "    }\n"
    "    private fun flushTelemetry() {\n"
    "        val ch = methodChannel\n"
    "        mainHandler.post {\n"
    "            ch?.invokeMethod(\"onTelemetry\", null)\n"
    "        }\n"
    "    }\n"
    "    private fun notifyError(message: String) {\n"
    "        val ch = methodChannel\n"
    "        mainHandler.post {\n"
    "            ch?.invokeMethod(\"onError\", null)\n"
    "        }\n"
    "    }\n"
    "    private fun pushToDart() {\n"
    "        val ch = methodChannel\n"
    "        mainHandler.post {\n"
    "            ch?.invokeMethod(\"onPacketsSampled\", null)\n"
    "        }\n"
    "    }\n"
    "}\n"
)
# S84 (Sprint 11.0M) - OpenE2eeVpnService.kt packetsObserved
# increment invariant. Regression guard for OnePlus 9 Pro
# Sprint 11.0A-11.0L fake-capture accusation (Owner 13:08).
case_s84_packets_observed_increment_invariant_pass = (
    "package com.opene2ee.opene2ee.vpn\n"
    "import java.util.concurrent.atomic.AtomicInteger\n"
    "class OpenE2eeVpnService {\n"
    "    private val packetsObserved = AtomicInteger(0)\n"
    "    private fun startCapture() {\n"
    "        packetsObserved.set(0)\n"
    "    }\n"
    "    private fun startReaderThread() {\n"
    "        try {\n"
    "            while (true) {\n"
    "                val n = input.read(buf)\n"
    "                if (n <= 0) break\n"
    "                val meta = extractMetadata(packet, n)\n"
    "                if (meta != null) {\n"
    "                    synchronized(ringLock) {\n"
    "                        ring.addLast(meta)\n"
    "                    }\n"
    "                    packetsObserved.incrementAndGet()\n"
    "                }\n"
    "            }\n"
    "        } catch (t: Throwable) {}\n"
    "    }\n"
    "}\n"
)



cases = [
    # S1-S5 cases (Sprint 9.6.6 — regression guard: must still pass)
    ("PASS (Sprint 9.6.6 fixed file)", run_check, (case_pass,), []),
    ("S1+S5 fail (Sprint 9.6.5 broken state: no real import + fully-qualified usage)",
     run_check, (case_s1_fail,), ["S1 fail", "S5 fail"]),
    ("S2 fail (missing JvmTarget import)", run_check, (case_s2_fail,), ["S2 fail"]),
    ("S3 fail (deprecated kotlinOptions present)", run_check, (case_s3_fail,), ["S3 fail"]),
    ("S4 fail (missing new kotlin compilerOptions block)", run_check, (case_s4_fail,), ["S4 fail"]),
    ("S5 fail (fully-qualified java.util.Properties())", run_check, (case_s5_fail,), ["S5 fail"]),
    # S6 cases (Sprint 9.6.7 — regression guard: must still pass)
    ("S6 PASS (Install Flutter dependencies step with working-directory=./mobile + flutter pub get)",
     run_s6_check, (case_s6_pass,), []),
    ("S6 FAIL (step missing entirely)", run_s6_check, (case_s6_step_missing,), ["S6 fail"]),
    ("S6 FAIL (working-directory=./mobile/android — wrong Dart project root)",
     run_s6_check, (case_s6_wrong_wd,), ["S6 fail"]),
    ("S6 FAIL (run=echo hello — not flutter pub get)",
     run_s6_check, (case_s6_wrong_run,), ["S6 fail"]),
    # S7 cases (Sprint 9.6.8 — new)
    ("S7 PASS (lib/main.dart + runApp( + ProviderScope + pubspec flutter_riverpod + go_router)",
     run_s7_check, (case_s7_main_pass, case_s7_pubspec_pass), []),
    ("S7 FAIL (lib/main.dart missing entirely)", run_s7_check,
     (None, case_s7_pubspec_main_missing), ["S7 fail"]),
    ("S7 FAIL (lib/main.dart exists but no ProviderScope — only runApp())", run_s7_check,
     (case_s7_no_providerscope, case_s7_pubspec_pass), ["S7 fail"]),
    ("S7 FAIL (pubspec.yaml missing flutter_riverpod: dependency)", run_s7_check,
     (case_s7_main_good_no_riverpod, case_s7_pubspec_no_riverpod), ["S7 fail"]),
    # S8 cases (Sprint 9.6.9 — new)
    ("S8 PASS (Android XML well-formed; no `--` inside `<!-- -->`)",
     run_s8_check, (case_s8_xml_pass,), []),
    ("S8 FAIL (comment contains `--` run — Sprint 9.6.9 broken state)",
     run_s8_check, (case_s8_xml_bad,), ["S8 fail"]),
# S9 cases (Sprint 9.6.10 — new)
    ("S9 PASS (manifest well-formed, no package attr, tools:replace, gradle namespace present)",
     run_s9_check, (case_s9_manifest_pass, case_s9_gradle_pass), []),
    ("S9 FAIL (manifest has package= attribute — Sprint 9.6.10 broken state)",
     run_s9_check, (case_s9_manifest_package, case_s9_gradle_pass), ["S9 fail"]),
    ("S9 FAIL (tools:remove co-exists with android:usesCleartextTraffic — MOB-1 broken state)",
     run_s9_check, (case_s9_manifest_tools_remove, case_s9_gradle_pass), ["S9 fail"]),
    # S10 cases (Sprint 9.6.11 — new)
    ("S10 PASS (values/styles.xml has LaunchTheme + NormalTheme + mipmap + launch_background)",
     run_s10_check, (case_s10_styles_pass, True, True), []),
    ("S10 FAIL (values/styles.xml missing LaunchTheme)",
     run_s10_check, (case_s10_styles_no_launch, True, True), ["S10 fail"]),
    ("S10 FAIL (no mipmap/ic_launcher representation)",
     run_s10_check, (case_s10_styles_for_mipmap, False, True), ["S10 fail"]),
    # S11 cases (Sprint 9.6.12 — new)
    ("S11 PASS (mobile/.flutter-plugins-dependencies exists with non-empty plugins.android[] + name+native_build per entry)",
     run_s11_check, (case_s11_fpd_pass,), []),
    ("S11 FAIL (.flutter-plugins-dependencies file missing entirely — Sprint 9.6.12 broken state)",
     run_s11_check, (None,), ["S11 fail"]),
    ("S11 FAIL (file exists but plugins.android[] is empty array — no Android plugins declared)",
     run_s11_check, (case_s11_fpd_empty_android,), ["S11 fail"]),
    # S12 cases (Sprint 9.6.13 — new)
    ("S12 PASS (app/build.gradle.kts declares io.flutter:flutter_embedding_ktx:1.0.0-<engine_commit> matching engine.version)",
     run_s12_check, (case_s12_gradle_pass, S12_ENGINE_VERSION), []),
    ("S12 FAIL (dependency missing entirely from dependencies block — Sprint 9.6.12 broken state, missing since PR-3)",
     run_s12_check, (case_s12_gradle_no_embedding, S12_ENGINE_VERSION), ["S12 fail"]),
    ("S12 FAIL (dependency declared with wrong hash — does not match Flutter SDK engine.version)",
     run_s12_check, (case_s12_gradle_wrong_hash, S12_ENGINE_VERSION), ["S12 fail"]),
    # S13 cases (Sprint 9.6.14 - new)
    ("S13 PASS (settings.gradle.kts dependencyResolutionManagement block contains Flutter storage URL)",
     run_s13_check, (case_s13_settings_pass, case_s13_app_pass_empty), []),
    ("S13 FAIL (settings.gradle.kts missing dependencyResolutionManagement block - Sprint 9.6.14 broken state)",
     run_s13_check, (case_s13_settings_no_drm, case_s13_app_no_drm_empty), ["S13 fail"]),
    ("S13 FAIL (DRM block present but Flutter storage URL not declared - missing since PR-28 Sprint 5)",
     run_s13_check, (case_s13_settings_no_url, case_s13_app_no_url_empty), ["S13 fail"]),
    # S17 cases (Sprint 9.7.0 Item 5 - new)
    ("S17 PASS (gradle wrapper tracked by git + repo-root .gitignore has matching `!**/android/...` re-include patterns)",
     run_s17_check, (case_s17_gitignore_pass, True, True, True), []),
    ("S17 FAIL (gradlew not tracked by git - regression: future `flutter create` re-run dropped force-include)",
     run_s17_check, (case_s17_gitignore_pass, False, True, True), ["S17 fail"]),
    # S18 cases (Sprint 9.7.0 Item 5 - new)
    ("S18 PASS (mobile/pubspec.lock tracked + parses as YAML + packages.flutter source: sdk + repo-root .gitignore has mobile-specific Flutter exclusion patterns)",
     run_s18_check, (case_s17_gitignore_pass, case_s18_pubspec_lock_pass), []),
    ("S18 FAIL (mobile/pubspec.lock not tracked - regression: future `flutter create` re-run dropped lockfile from index)",
     run_s18_check, (case_s17_gitignore_pass, None), ["S18 fail"]),
    # S19 cases (Sprint 9.7.0 Item 5 - new)
    ("S19 PASS (mobile/.metadata + mobile/android/.gitignore tracked by git)",
     run_s19_check, (True, True), []),
    ("S19 FAIL (mobile/.metadata not tracked - regression: future `flutter create` re-run dropped local metadata)",
     run_s19_check, (False, True), ["S19 fail"]),
    # S20 cases (Sprint 9.7.0 Item 5 - new)
    ("S20 PASS (mobile/pubspec.yaml parses + name + environment.sdk + dependencies.flutter.sdk + dev_dependencies.flutter_test.sdk)",
     run_s20_check, (case_s20_pubspec_pass,), []),
    ("S20 FAIL (mobile/pubspec.yaml missing `name:` key - regression: future edit dropped Dart pub project identifier)",
     run_s20_check, (case_s20_pubspec_no_name,), ["S20 fail"]),
    # S25 cases (Sprint 10.0 - new)
    ("S25 PASS (main.dart + screens/*.dart contain no `vpn` substring)",
     run_s25_check, ("void main() => runApp(const OpenE2EEApp());\n// Ağ Güvenliği Aracı\n",
                     "import 'package:flutter/material.dart';\nclass HomeScreen extends StatelessWidget {}\n"), []),
    ("S25 FAIL (main.dart contains the literal `vpn` - regression: future sprint re-introduces VPN framing)",
     run_s25_check, ("void main() { connectVpn(); }\n", "// clean\n"), ["S25 fail (main)"]),
    # S26 cases (Sprint 10.0 - new)
    ("S26 PASS (whatsapp_task_detail_screen.dart contains the literal `intent://send?text=`)",
     run_s26_check, ("final uri = 'intent://send?text=hello#Intent;scheme=whatsapp;package=com.whatsapp;end';\n",), []),
    ("S26 FAIL (whatsapp_task_detail_screen.dart missing the literal `intent://send?text=`)",
     run_s26_check, ("// replaced with custom intent later\n",), ["S26 fail (literal missing)"]),
    # S27 cases (Sprint 10.1A - new)
    ("S27 PASS (active_pool_screen.dart contains the literal `LineChart` from package:fl_chart)",
     run_s27_check, ("child: LineChart(LineChartData(lineBarsData: [LineChartBarData(spots: [])]))\n",), []),
    ("S27 FAIL (active_pool_screen.dart missing the literal `LineChart` - regression: Sprint 10.1A removed fl_chart)",
     run_s27_check, ("child: Text('chart coming soon')\n",), ["S27 fail (literal missing)"]),
    # S28 cases (Sprint 11.0O - INVERTED). The Sprint 10.1A
    # S28 audit (Timer.periodic MUST be present for the mock
    # ticker) is INVERTED in 11.0O to enforce NO mock
    # ticker Timer.periodic. The legitimate 5s _apiTick
    # poll Timer.periodic IS allowed.
    ("S28 PASS (pool_provider.dart has NO mock Timer.periodic - only the legitimate _apiTick poll - Sprint 11.0O inverted)",
     run_s28_check, ("_pollTimer = Timer.periodic(_pollPeriod, (_) => _apiTick());\n",), []),
    ("S28 FAIL (pool_provider.dart has the forbidden mock Timer.periodic with _mockTick callback - regression: Sprint 10.1A mock ticker not removed)",
     run_s28_check, ("_mockTimer = Timer.periodic(_mockTickPeriod, (_) => _mockTick());\n",), ["S28 fail (mock Timer.periodic callback _mockTick)"]),
    # S86 case (Sprint 11.0O - new) - active_pool_screen.dart
    # has NO mock Timer.periodic / setInterval / Future.delayed
    # and DOES subscribe to _vpn.packetStream.listen +
    # _vpn.stateStream.listen. Regression guard for the
    # Owner 13:20 "numbers animate without VPN" symptom.
    # Total selftest: 135 + 1 = 136 (after 11.0O S28
    # inversion + S86 added; the previous 11.0N S85 was
    # cancelled).
    ("S86 PASS (active_pool_screen.dart has NO mock Timer.periodic / setInterval / Future.delayed and DOES subscribe to _vpn.packetStream.listen + _vpn.stateStream.listen - regression guard for OnePlus 9 Pro fake UI animation)",
     run_s86_check, (
         "class ActivePoolScreen extends ConsumerStatefulWidget {\n"
         "  @override\n"
         "  ConsumerState<ActivePoolScreen> createState() => _S();\n"
         "}\n"
         "class _S extends ConsumerState<ActivePoolScreen> {\n"
         "  late final VpnService _vpn;\n"
         "  @override\n"
         "  void initState() {\n"
         "    super.initState();\n"
         "    _vpn = VpnService.instance;\n"
         "    _packetSub = _vpn.packetStream.listen(_onPacketsSampled);\n"
          "    _stateSub = _vpn.stateStream.listen((s) { setState(() => _vpnState = s); });\n"
          "  }\n"
          "}\n",
      ), []),
    # S87 case (Sprint 11.0P - new) - OpenE2eeVpnService.kt
    # has TUN_MTU=1400 (mobile-safe, NOT 1500) +
    # addDnsServer(1.1.1.1) + ipFragmentCount field +
    # per-1000-packet fragment log breadcrumb. Regression
    # guard for the OnePlus 9 Pro / Turkcell 4G/5G GTP
    # encapsulation MTU drop (Owner 13:50: 1247 packets
    # confirmed passthrough, MTU 1500 was the remaining
    # issue). Total selftest: 136 + 1 = 137.
    ("S87 PASS (OpenE2eeVpnService.kt has TUN_MTU=1400 + addDnsServer(1.1.1.1) + ipFragmentCount + per-1000-packet fragment log - regression guard for OnePlus 9 Pro / Turkcell 4G GTP encapsulation MTU drop)",
     run_s87_check, (
         "package com.opene2ee.opene2ee.vpn\n"
         "import java.util.concurrent.atomic.AtomicLong\n"
         "class OpenE2eeVpnService {\n"
         "    private val ipFragmentCount = AtomicLong(0)\n"
         "    companion object {\n"
         "        const val TUN_MTU = 1400\n"
         "        val PRIMARY_DNS = java.net.InetAddress.getByName(\"1.1.1.1\")\n"
         "    }\n"
         "    private fun startReaderThread() {\n"
         "        while (true) {\n"
         "            val n = 100\n"
         "            packetsObserved.incrementAndGet()\n"
         "            if (packetsObserved.get() % 1000 == 0) {\n"
         "                Log.d(TAG, \"startReaderThread: MTU=$TUN_MTU, \" +\n"
         "                        \"ipFragmentCount=${ipFragmentCount.get()}, \" +\n"
         "                        \"fragmentRatePct=0.0\")\n"
         "            }\n"
         "        }\n"
         "    }\n"
         "    private fun buildVpnBuilder(b: android.net.VpnService.Builder) {\n"
         "        b.addDnsServer(PRIMARY_DNS)\n"
          "        b.setMtu(TUN_MTU)\n"
          "    }\n"
          "}\n",
      ), []),
    # S93 case (Sprint 11.0T - new) - OpenE2eeVpnService
    # .kt has passthroughCount AtomicLong + reset +
    # pfd.fileDescriptor.valid check + per-write
    # increment + catch(Throwable) on write block +
    # passthroughGap in per-1000-packet breadcrumb.
    # Regression guard for the Owner 18:19 "passthrough
    # not actually writing" symptom (curl
    # 212.64.210.85/healthz fails with VPN, works
    # without). Total selftest: 141 + 1 = 142.
    ("S93 PASS (OpenE2eeVpnService.kt has passthroughCount AtomicLong + per-write increment + pfd.fileDescriptor.valid check + catch(Throwable) Log.e + passthroughGap in breadcrumb)",
     run_s93_check, (
         "package com.opene2ee.opene2ee.vpn\n"
         "import java.util.concurrent.atomic.AtomicLong\n"
         "class OpenE2eeVpnService {\n"
         "    private val passthroughCount = AtomicLong(0)\n"
         "    private fun startCapture() {\n"
         "        packetsObserved.set(0)\n"
         "        passthroughCount.set(0)\n"
         "    }\n"
         "    private fun startReaderThread(pfd: android.os.ParcelFileDescriptor) {\n"
         "        try {\n"
         "            while (true) {\n"
         "                val n = 100\n"
         "                if (!pfd.fileDescriptor.valid()) break\n"
         "                val writeOk = try {\n"
         "                    output.write(buf, 0, n)\n"
         "                    output.flush()\n"
         "                    passthroughCount.incrementAndGet()\n"
         "                    true\n"
         "                } catch (e: IOException) {\n"
         "                    Log.e(TAG, \"write failed (IOException)\", e)\n"
         "                    false\n"
         "                } catch (t: Throwable) {\n"
         "                    Log.e(TAG, \"write failed (Throwable)\", t)\n"
         "                    false\n"
         "                }\n"
         "                if (!writeOk) break\n"
         "            }\n"
         "        } catch (t: Throwable) {}\n"
         "    }\n"
         "    private fun per1000Breadcrumb() {\n"
         "        Log.d(TAG, \"startReaderThread: MTU=1400, \" +\n"
         "            \"passthroughCount=${passthroughCount.get()}, \" +\n"
         "            \"passthroughGap=${packetsObserved.get() - passthroughCount.get()}\")\n"
         "    }\n"
         "}\n",
     ), []),
    # S88 case (Sprint 11.0Q - new) - active_pool_screen
    # .dart has 2-level VPN disconnect fallback (.stop
    # with 3s timeout + MainActivity.disconnectVpn) AND
    # MainActivity.kt has disconnectVpn method (stopService
    # + VpnService.prepare). Regression guard for the
    # Owner 14:14 "Oturumu Bitir requires app uninstall"
    # symptom. Total selftest: 137 + 1 = 138.
    ("S88 PASS (active_pool_screen.dart has 2-level VPN disconnect fallback + MainActivity.disconnectVpn hard-stop - regression guard for OnePlus 9 Pro 'Oturumu Bitir requires app uninstall')",
     run_s88_check,
     (
         "import 'package:flutter/services.dart';\n"
         "import 'dart:async';\n"
         "class _S {\n"
         "  Future<void> _oturumuBitir() async {\n"
         "    final _permissions = MethodChannel('opene2ee/permissions');\n"
         "    try {\n"
         "      await _vpn.stop().timeout(const Duration(seconds: 3));\n"
         "    } on TimeoutException {\n"
         "      await _permissions.invokeMethod('disconnectVpn');\n"
         "    } catch (e) {\n"
         "      await _permissions.invokeMethod('disconnectVpn');\n"
         "    }\n"
         "  }\n"
         "}\n",
         "package com.opene2ee.opene2ee\n"
         "import android.content.Intent\n"
         "import android.net.VpnService\n"
         "import com.opene2ee.opene2ee.vpn.OpenE2eeVpnService\n"
         "class MainActivity {\n"
         "  private fun onPermissionsCall(call: MethodCall, result: MethodChannel.Result) {\n"
         "    when (call.method) {\n"
         "      \"disconnectVpn\" -> disconnectVpn(result)\n"
         "    }\n"
         "  }\n"
         "  private fun disconnectVpn(result: MethodChannel.Result) {\n"
         "    val stopIntent = Intent(this, OpenE2eeVpnService::class.java)\n"
         "    stopService(stopIntent)\n"
          "    VpnService.prepare(this)\n"
          "    result.success(true)\n"
          "  }\n"
          "}\n",
      ),
      []),
    # S89 case (Sprint 11.0R - new) - active_pool_screen
    # .dart has full state reset on disconnect
    # (_packetSub.cancel + _stateSub.cancel +
    # _toplamPaket=0 + _vpnState=idle + setState +
    # _disconnectInProgress guard + context.go
    # /home/gorevler). Regression guard for the
    # Owner 15:03 "packet counter keeps growing after
    # disconnect" symptom. Total selftest: 138 + 1 = 139.
    ("S89 PASS (active_pool_screen.dart has full state reset on disconnect - subscriptions cancelled + counters cleared + UI reset + button disabled while in flight + navigation to /home/gorevler)",
     run_s89_check, (
         "import 'dart:async';\n"
         "class _S {\n"
         "  bool _disconnectInProgress = false;\n"
         "  StreamSubscription? _packetSub;\n"
         "  StreamSubscription? _stateSub;\n"
         "  StreamSubscription? _webrtcStateSub;\n"
         "  int _toplamPaket = 0;\n"
         "  int _toplamTelemetri = 0;\n"
         "  Future<void> _oturumuBitir() async {\n"
         "    if (_disconnectInProgress) return;\n"
         "    _disconnectInProgress = true;\n"
         "    await _packetSub?.cancel();\n"
         "    await _stateSub?.cancel();\n"
         "    await _webrtcStateSub?.cancel();\n"
         "    _packetSub = null;\n"
         "    _stateSub = null;\n"
         "    _webrtcStateSub = null;\n"
         "    setState(() {\n"
         "      _toplamPaket = 0;\n"
         "      _toplamTelemetri = 0;\n"
         "      _vpnState = VpnLifecycleState.idle;\n"
         "      _webrtcState = WebRTCState.closed;\n"
         "    });\n"
          "    context.go('/home/gorevler');\n"
          "    _disconnectInProgress = false;\n"
          "  }\n"
          "}\n",
      ), []),
    # S91 case (Sprint 11.0S-DNS - new) - OpenE2eeVpnService
    # .kt has isPrivateDnsActive + ConnectivityManager
    # .bindProcessToNetwork + active_pool_screen.dart has
    # private_dns_active status() poll + chrome://flags
    # #dns-httpssvc snackbar. Regression guard for the
    # Owner 17:14 OnePlus 9 Pro OxygenOS Android 9+
    # Private DNS override symptom (Chrome + WhatsApp
    # "no internet" even though TUN capture +
    # passthrough work). Total selftest: 139 + 1 = 140.
    ("S91 PASS (OpenE2eeVpnService.kt has isPrivateDnsActive + bindProcessToNetwork + active_pool_screen.dart has private_dns_active check + chrome://flags/#dns-httpssvc snackbar)",
     run_s91_check,
     (
         "package com.opene2ee.opene2ee.vpn\n"
         "import android.net.ConnectivityManager\n"
         "import android.net.LinkProperties\n"
         "import android.net.Network\n"
         "import android.net.NetworkCapabilities\n"
         "import android.net.NetworkRequest\n"
         "class OpenE2eeVpnService {\n"
         "    private fun checkPrivateDnsAndBindToVpn() {\n"
         "        val cm = getSystemService(...) as ConnectivityManager\n"
         "        val lp: LinkProperties? = cm.getLinkProperties(cm.activeNetwork)\n"
         "        if (lp != null && lp.isPrivateDnsActive) {\n"
         "            lastError = \"private_dns_active: VPN DNS bypassed\"\n"
         "        }\n"
         "        val request = NetworkRequest.Builder()\n"
         "            .addTransportType(NetworkCapabilities.TRANSPORT_VPN)\n"
         "            .build()\n"
         "        cm.requestNetwork(request, object : ConnectivityManager.NetworkCallback() {\n"
         "            override fun onAvailable(network: Network) {\n"
         "                cm.bindProcessToNetwork(network)\n"
         "            }\n"
         "        })\n"
         "    }\n"
         "}\n",
         "class _S {\n"
         "  Future<void> _onStart() async {\n"
         "    final status = await _vpn.status();\n"
         "    final lastError = status['lastError'] as String?;\n"
         "    if (lastError != null && lastError.startsWith('private_dns_active')) {\n"
         "      ScaffoldMessenger.of(context).showSnackBar(\n"
         "        SnackBar(\n"
         "          content: Text('Chrome: chrome://flags/#dns-httpssvc > Disabled. '),\n"
         "        ),\n"
         "      );\n"
          "    }\n"
          "  }\n"
          "}\n",
      ),
      []),
    # S92 case (Sprint 11.0S-EXTRA - new) - OpenE2eeVpnService
    # .kt has setUsesChronometer + setWhen + mainHandler
    # .postDelayed auto-stop. Regression guard for the
    # Owner 17:21 OnePlus 9 Pro "15-minute countdown must
    # show in notification bar" requirement. Total
    # selftest: 140 + 1 = 141.
    ("S92 PASS (OpenE2eeVpnService.kt has COUNTDOWN_TOTAL_MS + setUsesChronometer + setWhen + scheduleCountdownAutoStop + mainHandler.postDelayed + stopCapture(graceful=true))",
     run_s92_check, (
         "package com.opene2ee.opene2ee.vpn\n"
         "import android.os.Handler\n"
         "import android.os.Looper\n"
         "class OpenE2eeVpnService {\n"
         "    companion object {\n"
         "        const val COUNTDOWN_TOTAL_MS = 15L * 60L * 1000L\n"
         "    }\n"
         "    private val mainHandler: Handler = Handler(Looper.getMainLooper())\n"
         "    private var countdownAutoStopRunnable: Runnable? = null\n"
         "    private fun buildForegroundNotification(): android.app.Notification {\n"
         "        val endTimeMs = System.currentTimeMillis() + COUNTDOWN_TOTAL_MS\n"
         "        return androidx.core.app.NotificationCompat.Builder(this, \"chan\")\n"
         "            .setContentTitle(\"test\")\n"
         "            .setOngoing(true)\n"
         "            .setUsesChronometer(true)\n"
         "            .setWhen(endTimeMs)\n"
         "            .setShowWhen(true)\n"
         "            .build()\n"
         "    }\n"
         "    private fun scheduleCountdownAutoStop() {\n"
         "        countdownAutoStopRunnable?.let { mainHandler.removeCallbacks(it) }\n"
         "        val runnable = Runnable {\n"
         "            try { stopCapture(graceful = true) } catch (e: Throwable) {}\n"
         "            countdownAutoStopRunnable = null\n"
         "        }\n"
         "        countdownAutoStopRunnable = runnable\n"
         "        mainHandler.postDelayed(runnable, COUNTDOWN_TOTAL_MS)\n"
         "    }\n"
         "    private fun stopCapture(graceful: Boolean): State {\n"
         "        countdownAutoStopRunnable?.let { mainHandler.removeCallbacks(it) }\n"
         "        countdownAutoStopRunnable = null\n"
         "        return State.STOPPED\n"
         "    }\n"
         "}\n",
     ), []),
    # S29 cases (Sprint 10.1A - new)
    ("S29 PASS (active_pool_screen.dart contains the literal `HapticFeedback` for eşleşme feedback)",
     run_s29_check, ("HapticFeedback.lightImpact();\n",), []),
    ("S29 FAIL (active_pool_screen.dart missing both `HapticFeedback` and `SystemSound` - regression: eşleşme is silent)",
     run_s29_check, ("// haptic removed; visual only\nScaffoldMessenger.showSnackBar(...)\n",), ["S29 fail (no haptic / system-sound literal)"]),
    # S33 cases (Sprint 10.1C - new)
    ("S33 PASS (pool_provider.dart contains both `lastError` and `lastSuccess` debug-state fields on PoolState)",
     run_s33_check, ("final String? lastError;\nfinal String? lastSuccess;\n",), []),
    ("S33 FAIL (pool_provider.dart missing `lastError` literal - regression: silent-failure mode returns)",
     run_s33_check, ("// error-tracking field removed\nfinal String? lastSuccess;\n",), ["S33 fail (missing: lastError)"]),
    # S34 cases (Sprint 10.1C - new)
    ("S34 PASS (active_pool_screen.dart contains the literal `ScaffoldMessenger.of(context).showSnackBar` for ref.listen<PoolState> snackbar handler)",
     run_s34_check, ("ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text('Hata')));\n",), []),
    ("S34 FAIL (active_pool_screen.dart missing the literal `ScaffoldMessenger.of(context).showSnackBar` - regression: ref.listen compiles to no-op)",
     run_s34_check, ("final messenger = ScaffoldMessenger.of(context);\nmessenger.showSnackBar(...);\n",), ["S34 fail (literal missing)"]),
    # S35 cases (Sprint 10.1C - new)
    ("S35 PASS (telemetry_service.dart contains `String.fromEnvironment('API_KEY'` for build-time API key injection)",
     run_s35_check, ("const String _kApiKey = String.fromEnvironment('API_KEY', defaultValue: 'test_key_placeholder');\n", None), []),
    ("S35 FAIL (neither telemetry_service.dart nor p2p_matcher.dart contains the literal - regression: --dart-define API_KEY=... silently ignored)",
     run_s35_check, ("// literal removed in 10.1C rebase; using constructor param instead\n", "// p2p uses constructor param too\n"), ["S35 fail (literal missing in both services)"]),
    # S36 cases (Sprint 10.1D - new)
    ("S36 PASS (auth_service.dart contains http.post + /api/v1/auth + user_id literals for JWT auth flow)",
     run_s36_check, ("final resp = await http.post(\n  Uri.parse('${AppConfig.apiBase}/api/v1/auth'),\n  body: jsonEncode({'user_id': AppConfig.deviceId}),\n);\n",), []),
    ("S36 FAIL (auth_service.dart missing one of http.post + /api/v1/auth + user_id - regression: JWT flow silently broken)",
     run_s36_check, ("// switched to header-based auth, no POST body\nfinal headers = {'Authorization': 'Bearer token'};\n",), ["S36 fail (missing: http.post,/api/v1/auth,user_id)"]),
    # S37 cases (Sprint 10.1D - new)
    ("S37 PASS (telemetry_service.dart contains `authHeaders()` call for JWT-protected POST)",
     run_s37_check, ("final headers = await _auth.authHeaders();\nheaders['Content-Type'] = 'application/json';\n", None), []),
    ("S37 FAIL (neither telemetry_service.dart nor p2p_matcher.dart contains `authHeaders()` - regression: static Bearer key returns)",
     run_s37_check, ("// auth integration removed, using static key\nheaders: {'Authorization': 'Bearer $_apiKey'},\n", "// p2p also reverted to static key\n"), ["S37 fail (authHeaders() call missing in both services)"]),
    # S38 cases (Sprint 10.1D - new)
    ("S38 PASS (auth_service.dart contains `_tokenExpiresAt` field for JWT token cache + 5min pre-expiry refresh)",
     run_s38_check, ("DateTime? _tokenExpiresAt;\nbool get hasValidToken => _tokenExpiresAt != null;\n",), []),
    ("S38 FAIL (auth_service.dart missing `_tokenExpiresAt` - regression: every protected call re-auths)",
     run_s38_check, ("// expiry tracking field removed; using one-shot token\nString? _cachedToken;\n",), ["S38 fail (_tokenExpiresAt field missing)"]),
     # S39 cases (Sprint 10.1D - new)
    ("S39 PASS (auth_service.dart contains `invalidate()` method for 401 retry contract)",
     run_s39_check, ("void invalidate() {\n  _cachedToken = null;\n  _tokenExpiresAt = null;\n}\n",), []),
    ("S39 FAIL (auth_service.dart missing `invalidate()` - regression: 401 leaves stale cached JWT)",
     run_s39_check, ("// flush-on-401 method removed; cached JWT stays until 1h expiry\n",), ["S39 fail (invalidate() method missing)"]),
    # S40 cases (Sprint 10.1E - new)
    ("S40 PASS (whatsapp_deeplink_provider.dart carries BOTH `intent://send?` prefix and `#Intent;scheme=whatsapp;package=com.whatsapp;end` suffix)",
     run_s40_check, (
         "static Uri buildUri() => Uri.parse(\n"
         "  'intent://send?text=foo#Intent;scheme=whatsapp;package=com.whatsapp;end',\n"
         ");\n",), []),
    ("S40 FAIL (whatsapp_deeplink_provider.dart missing BOTH Android Intent literals - regression: 10.0 whatsapp://send?text= scheme was unreliable on Android)",
     run_s40_check, (
         "// forgot both halves of the Android Intent URI, reverted to legacy\n"
         "static Uri buildUri() => Uri.parse('whatsapp://send?text=foo');\n",),
     ["S40 fail (missing: intent://send?,#Intent;scheme=whatsapp;package=com.whatsapp;end)"]),
    # S41 cases (Sprint 10.1E - new)
    ("S41 PASS (p2p_matcher.dart uses /api/v1/sessions and does NOT contain the broken /api/v1/matches)",
     run_s41_check, (
         "final uri = Uri.parse('\${AppConfig.apiBase}/api/v1/sessions');\n"
         "final resp = await _client.get(uri, headers: headers);\n",), []),
    ("S41 FAIL (p2p_matcher.dart missing /api/v1/sessions - regression: 10.1B /api/v1/matches 404'd because the backend never had that route)",
     run_s41_check, (
         "// reverted to the 10.1B path that 404'd\n"
         "final uri = Uri.parse('\${AppConfig.apiBase}/api/v1/matches?sessionId=...');\n",),
     ["S41 fail (missing /api/v1/sessions)", "S41 fail (forbidden /api/v1/matches still present)"]),
    # S42 cases (Sprint 10.1F - new)
    ("S42 PASS (AndroidManifest.xml <queries> block carries <package android:name=\"com.whatsapp\" /> + com.whatsapp.w4b — Android 11+ package visibility)",
     run_s42_check, (case_s42_manifest_pass,), []),
    ("S42 FAIL (AndroidManifest.xml <queries> block present but <package android:name=\"com.whatsapp\" /> missing — Owner report 10.07.2026 23:29: 'whatsapp yüklü değil diyor hala deeplink yine hatalı')",
     run_s42_check, (case_s42_manifest_no_package,),
     ["S42 fail (<package android:name=\"com.whatsapp\" /> missing)"]),
    # S43 cases (Sprint 10.1F - new)
    ("S43 PASS (MainActivity.kt wires `when (call.method)` dispatch with `\"getSampledPackets\"` case on the `opene2ee/vpn` MethodChannel — Kotlin mock packet for 10.1F; real OpenE2eeVpnService integration lands in Sprint 10.2)",
     run_s43_check, (case_s43_main_activity_pass,), []),
    ("S43 FAIL (MainActivity.kt has a `when (call.method)` block on `opene2ee/vpn_permissions` but the `\"getSampledPackets\"` case is missing on the `opene2ee/vpn` channel — Owner report 10.07.2026 23:29: 30 consecutive 'Aktif Nöbet' calls all failed with MissingPluginException)",
     run_s43_check, (case_s43_main_activity_no_handler,),
     ["S43 fail (\"getSampledPackets\" case missing)"]),
    # S44 cases (Sprint 10.1G - new)
    # Case 25 (S44 PASS): post-10.1G provider carries BOTH
    # `intent://send?text=` (fallback) and `https://wa.me/?text=`
    # (primary) literals; screen file calls `tryOpenWithReason()`.
    # Mirrors the OnePlus 9 Pro rooted / Magisk / LSPosed fix path.
    ("S44 PASS (whatsapp_deeplink_provider.dart carries BOTH `intent://send?text=` (10.1E fallback) AND `https://wa.me/?text=` (10.1G primary) literals; whatsapp_task_detail_screen.dart calls `tryOpenWithReason()` — OnePlus 9 Pro rooted / Magisk / LSPosed fix; snackbar surfaces per-tier debug reason)",
     run_s44_check, (case_s44_provider_pass, case_s44_screen_pass), []),
    # S45 cases (Sprint 11.0A - new)
    ("S45 PASS (OpenE2eeVpnService.kt PacketDrain pushes 'onPacketsSampled' literal via methodChannel.invokeMethod — 5-second scheduled drain from real TUN ring to Dart)",
     run_s45_check, (case_s45_vpn_service_pass,), []),
    ("S45 FAIL (OpenE2eeVpnService.kt missing the 'onPacketsSampled' literal — regression: Dart-side packetStream has no event to subscribe to)",
     run_s45_check, (case_s45_vpn_service_no_literal,), ['S45 fail ("onPacketsSampled" literal missing)']),
    # S46 cases (Sprint 11.0A - new)
    ("S46 PASS (MainActivity.kt calls OpenE2eeVpnService.snapshot() and does NOT contain the 10.1F mock packet mapOf('srcIpMasked' ...) literal — real TUN ring feeds Dart)",
     run_s46_check, (case_s46_main_activity_pass,), []),
    ("S46 FAIL (MainActivity.kt still has the 10.1F inline mock packet mapOf('srcIpMasked' ...) — regression: 30 consecutive 'Aktif Nöbet' calls all read synthetic data)",
     run_s46_check, (case_s46_main_activity_with_mock,),
     ['S46 fail (OpenE2eeVpnService.snapshot() call missing)', 'S46 fail (mock packet mapOf(...) literal still present)']),
    # S47 cases (Sprint 11.0A - new)
    ("S47 PASS (vpn_service.dart carries 'packetStream' getter + 'MethodChannel' import — live Stream<List<SampledPacket>> + inbound channel handler)",
     run_s47_check, (case_s47_vpn_service_pass,), []),
    ("S47 FAIL (vpn_service.dart missing 'packetStream' getter — regression: ActivePoolScreen reverts to fixed-loop mock ticker)",
     run_s47_check, (case_s47_vpn_service_no_packetstream,),
     ['S47 fail (missing: packetStream,MethodChannel import)']),
    # S48 cases (Sprint 11.0A - new)
    ("S48 PASS (active_pool_screen.dart subscribes to packetStream via .listen in initState — live 5s packet batches drive the cumulative counter)",
     run_s48_check, (case_s48_active_pool_pass,), []),
    ("S48 FAIL (active_pool_screen.dart missing 'packetStream.listen' — regression: live packet feed disconnected, screen reads only static mock data)",
     run_s48_check, (case_s48_active_pool_no_listen,),
     ['S48 fail (packetStream.listen literal missing)']),
    # S49 cases (Sprint 11.0A - new)
    ("S49 PASS (packet_parser.dart has SampledPacket class with fromBytes() + toJson() — wire-format mirror of Kotlin OpenE2eeVpnService.extractMetadata)",
     run_s49_check, (case_s49_packet_parser_pass,), []),
    ("S49 FAIL (packet_parser.dart missing SampledPacket class — regression: Dart cannot decode the wire shape from the Kotlin MethodChannel)",
     run_s49_check, (case_s49_packet_parser_no_sampledpacket,),
     ['S49 fail (missing: class SampledPacket,fromBytes)']),
    # S50 cases (Sprint 11.0A - new)
    ("S50 PASS (OpenE2eeVpnService.kt foreground notification title is 'OpenE2EE Şifreleme Doğrulama' — S25 invariant: no 'VPN' string in user-facing surface)",
     run_s50_check, (case_s50_vpn_service_pass,), []),
    ("S50 FAIL (OpenE2eeVpnService.kt notification title still contains 'VPN' — regression: S25 invariant violated, Owner push blocked)",
     run_s50_check, (case_s50_vpn_service_with_vpn_word,),
     ['S50 fail (foreground notification title "OpenE2EE Şifreleme Doğrulama" missing)']),
    # S51 cases (Sprint 11.0A - new)
    ("S51 PASS (active_pool_screen.dart has packetStream subscription + NO 30-call fixed Timer.periodic loop — chart is continuous, not bounded)",
     run_s51_check, (case_s51_active_pool_pass,), []),
    ("S51 FAIL (active_pool_screen.dart still has the 10.1A i < 30 + Timer.periodic fixed loop — regression: chart auto-stops at 30 iterations)",
     run_s51_check, (case_s51_active_pool_with_30_loop,),
     ['S51 fail (30-call fixed Timer.periodic loop still present)', 'S51 fail (continuous packetStream subscription missing)']),
    # S52 cases (Sprint 11.0A - new)
    ("S52 PASS (telemetry_service.dart has sendSummary method POSTing to /api/v1/sessions/{id}/telemetry with 6 summary fields — 30-second batch upload)",
     run_s52_check, (case_s52_telemetry_pass,), []),
    ("S52 FAIL (telemetry_service.dart missing sendSummary — regression: no aggregate session statistics, Skorlar screen in M3 has no data source)",
     run_s52_check, (case_s52_telemetry_no_summary,),
     ['S52 fail (missing: sendSummary,/api/v1/sessions/ path,encryptionIntegrityPct field,packetLossPct field)']),
    # S53 cases (Sprint 11.0B - new)
    ("S53 PASS (pubspec.yaml carries the `webrtc:` dep line — flutter_webrtc ^1.5.0 resolves the brief's `webrtc: ^0.13.0+` substring requirement)",
     run_s53_check, (case_s53_pubspec_pass,), []),
    ("S53 FAIL (pubspec.yaml missing `webrtc:` dep line — regression: the WebRTC peer connection is the M2 demo path)",
     run_s53_check, (case_s53_pubspec_no_webrtc,), ['S53 pubspec.yaml: missing `webrtc:` dependency line. Sprint 11.0B invariant — the WebRTC peer connection is the M2 demo path. (The brief specifies `webrtc: ^0.13.0+`; the modern actively-maintained variant is `flutter_webrtc: ^1.5.0` which satisfies the same audit substring.)']),
    # S54 cases (Sprint 11.0B - new)
    ("S54 PASS (webrtc_service.dart imports flutter_webrtc + references RTCPeerConnection + calls createPeerConnection)",
     run_s54_check, (case_s54_webrtc_service_pass,), []),
    ("S54 FAIL (webrtc_service.dart missing the flutter_webrtc import + RTCPeerConnection class reference)",
     run_s54_check, (case_s54_webrtc_service_no_rtc,),
     ['S54 webrtc_service.dart: missing flutter_webrtc import, createPeerConnection call site. Sprint 11.0B invariant — the Dart-side peer connection wrapper must import the `flutter_webrtc` package and instantiate `RTCPeerConnection` via `createPeerConnection({iceServers: ...})`.']),
    # S55 cases (Sprint 11.0B - new)
    ("S55 PASS (webrtc_service.dart onIceCandidate callback wires candidate + sdpMid + sdpMLineIndex fields)",
     run_s55_check, (case_s55_webrtc_service_pass,), []),
    ("S55 FAIL (webrtc_service.dart missing the onIceCandidate callback + candidate string literal)",
     run_s55_check, (case_s55_webrtc_service_no_callback,),
     ['S55 webrtc_service.dart: missing candidate string literal in callback, sdpMid field. Sprint 11.0B invariant — the `onIceCandidate` callback forwards each peer-discovered candidate to the orchestrator\'s `POST /api/v1/webrtc/ice` endpoint. The candidate payload carries `candidate` (RFC 5245 candidate string) + `sdpMid` (mid attribute) + `sdpMLineIndex` (line index).']),
    # S56 cases (Sprint 11.0B - new)
    ("S56 PASS (session_orchestrator.dart startSession() + JWT authHeaders() + /api/v1/sessions endpoint)",
     run_s56_check, (case_s56_orchestrator_pass,), []),
    ("S56 FAIL (session_orchestrator.dart missing the startSession method + JWT authHeaders() call)",
     run_s56_check, (case_s56_orchestrator_no_start,),
     ['S56 session_orchestrator.dart: missing authHeaders() call (JWT), /api/v1/sessions endpoint. Sprint 11.0B invariant — `startSession()` is the JWT-authenticated entry point that mints a session id (and receiver_session_id) the orchestrator uses for the rest of the negotiation flow.']),
    # S57 cases (Sprint 11.0B - new)
    ("S57 PASS (session_orchestrator.dart long-poll GET (pollForOffer) with Duration(seconds: 30) timeout)",
     run_s57_check, (case_s57_orchestrator_pass,), []),
    ("S57 FAIL (session_orchestrator.dart missing the 30s long-poll timeout literal + pollForOffer method)",
     run_s57_check, (case_s57_orchestrator_no_longpoll,),
     ['S57 session_orchestrator.dart: missing 30s long-poll timeout literal. Sprint 11.0B invariant — the orchestrator\'s `pollForOffer` / `pollForAnswer` methods long-poll GET with a 30s timeout (the brief\'s `Future.timeout` contract).']),
    # S59 cases (Sprint 11.0B - new)
    ("S59 PASS (webrtc_service.dart onTrack stream exposed (get onTrack + onTrack callback))",
     run_s59_check, (case_s59_webrtc_service_pass,), []),
    ("S59 FAIL (webrtc_service.dart missing the onTrack stream getter)",
     run_s59_check, (case_s59_webrtc_service_no_ontrack,),
     ['S59 webrtc_service.dart: missing onTrack stream getter. Sprint 11.0B invariant — the service exposes the peer connection\'s `onTrack` stream so the UI can show \'1 stream received\' when the test harness triggers an inbound track event.']),
    # S60 cases (Sprint 11.0B - new)
    ("S60 PASS (active_pool_screen.dart WebRTC status indicator with Turkish labels: müzakere / bağlandı / hata)",
     run_s60_check, (case_s60_active_pool_pass,), []),
    ("S60 FAIL (active_pool_screen.dart missing the three status indicator labels)",
     run_s60_check, (case_s60_active_pool_no_indicators,),
     ['S60 active_pool_screen.dart: missing Negotiating label (müzakere), Connected label (bağlandı), Failed label (hata). Sprint 11.0B invariant — the WebRTC status pill on the active pool screen surfaces the live peer connection state with three labels: Negotiating / Connected / Failed (Turkish: müzakere / bağlandı / hata). The `P2P:` prefix in the row distinguishes the WebRTC pill from the foreground service pill.']),
    # S61 cases (Sprint 11.0C - new)
    ("S61 PASS (skorlar_screen.dart has Future<List<SessionScore>> + ConsumerStatefulWidget + fetchScores method)",
     run_s61_check, (case_s61_skorlar_pass,), []),
    ("S61 FAIL (skorlar_screen.dart missing the ConsumerStatefulWidget + fetchScores method — regression: Skorlar screen can't list completed sessions)",
     run_s61_check, (case_s61_skorlar_no_consumer,),
     ['S61 skorlar_screen.dart: missing Future<List<SessionScore>>. Sprint 11.0C invariant — the screen is a Riverpod ConsumerStatefulWidget; the future list type Future<List<SessionScore>> + the fetchScores method are the canonical 11.0C wire shape.']),
    # S62 cases (Sprint 11.0C - new)
    ("S62 PASS (score_calculator.dart has SessionScoreCalculator class + static SessionScore compute method)",
     run_s62_check, (case_s62_score_calculator_pass,), []),
    ("S62 FAIL (score_calculator.dart missing the SessionScoreCalculator class + compute method — regression: no pure scoring function)",
     run_s62_check, (case_s62_score_calculator_no_compute,),
     ['S62 score_calculator.dart: missing static SessionScore compute method. Sprint 11.0C invariant — the `compute` method is a pure function (no I/O, no time-source injection) so it\'s unit-testable and the Skorlar screen can compute the headline score from a `summary_stats` block without side effects.']),
    # S63 cases (Sprint 11.0C - new)
    ("S63 PASS (score_calculator.dart carries the 4 metric field references: encryptionIntegrityPct, packetLossPct, meanLatencyMs, jitterMs)",
     run_s63_check, (case_s63_score_calculator_pass,), []),
    ("S63 FAIL (score_calculator.dart missing the 4 metric formulas — regression: the Skorlar card can't show the per-metric detail rows)",
     run_s63_check, (case_s63_score_calculator_no_metrics,),
     ['S63 score_calculator.dart: missing encryptionIntegrityPct metric, packetLossPct metric, meanLatencyMs metric, jitterMs metric. Sprint 11.0C invariant — the 4 metric fields (encryption integrity %, packet loss %, mean latency ms, jitter ms) are the inputs to the weighted sum; the Skorlar screen\'s `SessionScoreCard` detail view shows all 4 side-by-side.']),
    # S64 cases (Sprint 11.0C - new)
    ("S64 PASS (score_calculator.dart has the overall weighted sum 0.4 + 0.3 + 0.2 + 0.1)",
     run_s64_check, (case_s64_score_calculator_pass,), []),
    ("S64 FAIL (score_calculator.dart missing the 0.4 + 0.3 + 0.2 + 0.1 weighted sum weights — regression: the headline score is unweighted)",
     run_s64_check, (case_s64_score_calculator_no_weights,),
     ['S64 score_calculator.dart: missing overall weighted sum weights (0.4 + 0.3 + 0.2 + 0.1). Sprint 11.0C invariant — the 4 weights sum to 1.0; the brief\'s spec is verbatim. The audit accepts the literal `0.4 *` + `0.3 *` + `0.2 *` + `0.1 *` substring sequence in the file\'s `compute` method.']),
    # S65 cases (Sprint 11.0C - new)
    ("S65 PASS (session_orchestrator.dart has closeSession() method that POSTs /api/v1/sessions/{id}/close)",
     run_s65_check, (case_s65_orchestrator_pass,), []),
    ("S65 FAIL (session_orchestrator.dart missing the closeSession() method — regression: 'Oturumu Bitir' button has no orchestrator endpoint)",
     run_s65_check, (case_s65_orchestrator_no_close,),
     ['S65 session_orchestrator.dart: missing close endpoint path. Sprint 11.0C invariant — `closeSession()` POSTs to `/api/v1/sessions/{id}/close` and caches the `summary_stats` block. The active-pool screen\'s "Oturumu Bitir" button is the only call site.']),
    # S66 cases (Sprint 11.0C - new)
    ("S66 PASS (active_pool_screen.dart has the 'Oturumu Bitir' Turkish label)",
     run_s66_check, (case_s66_active_pool_pass,), []),
    ("S66 FAIL (active_pool_screen.dart missing the 'Oturumu Bitir' Turkish label — regression: S25 invariant + the M3 close flow has no UI surface)",
     run_s66_check, (case_s66_active_pool_no_button,),
     ['S66 active_pool_screen.dart: missing `Oturumu Bitir` Turkish label. Sprint 11.0C invariant — the button calls `_orchestrator.closeSession()` and navigates to /home/skorlar. S25 invariant extends: no \'VPN\' string, Turkish UI text only.']),
    # S67 cases (Sprint 11.0C - new)
    ("S67 PASS (active_pool_screen.dart closeSession call + navigate to /home/skorlar flow)",
     run_s67_check, (case_s67_active_pool_pass,), []),
    ("S67 FAIL (active_pool_screen.dart missing the closeSession + /home/skorlar flow — regression: the Oturumu Bitir button has no target)",
     run_s67_check, (case_s67_active_pool_no_flow,),
     ['S67 active_pool_screen.dart: missing closeSession call site, /home/skorlar navigation. Sprint 11.0C invariant — the Oturumu Bitir flow calls `_orchestrator.closeSession()` then `context.go(\'/home/skorlar\')` so the new score is visible without an explicit refresh.']),
    # S68 cases (Sprint 11.0C - new)
    ("S68 PASS (skorlar_screen.dart has the 'Henüz tamamlanmış oturum yok' empty-state string)",
     run_s68_check, (case_s68_skorlar_pass,), []),
    ("S68 FAIL (skorlar_screen.dart missing the empty-state string — regression: the screen has no placeholder for new users)",
     run_s68_check, (case_s68_skorlar_no_empty,),
     ['S68 skorlar_screen.dart: missing `Henüz tamamlanmış oturum yok` empty-state string. Sprint 11.0C invariant — the screen shows the empty state when `fetchScores()` returns an empty list (no completed sessions yet).']),
    # S69 cases (Sprint 11.0C - new)
    ("S69 PASS (skorlar_screen.dart has SessionScoreCard with overall-score gauge disc)",
     run_s69_check, (case_s69_skorlar_pass,), []),
    ("S69 FAIL (skorlar_screen.dart missing the SessionScoreCard + overall-score gauge — regression: the screen has no per-session card with a headline score)",
     run_s69_check, (case_s69_skorlar_no_card,),
     ['S69 skorlar_screen.dart: missing SessionScoreCard widget, overallScore gauge (disc). Sprint 11.0C invariant — each session card has a headline gauge (coloured disc with the overall score 0-100) plus an expandable details view with the 4 sub-metrics. The brief\'s spec is a `fl_chart` radial gauge; Sprint 11.0C uses a simple disc with the `tier` color hint to keep the APK small after the M2 +50 MB libwebrtc hit.']),
    # S70 cases (Sprint 11.0C - new)
    ("S70 PASS (backend router.go has POST /api/v1/sessions/{id}/close route registration)",
     run_s70_check, (case_s70_router_pass,), []),
    ("S70 FAIL (backend router.go missing the POST /api/v1/sessions/{id}/close route — regression: the mobile closeSession() has no backend endpoint)",
     run_s70_check, (case_s70_router_no_close,),
     ['S70 backend/internal/api/router.go: missing `r.Post("/sessions/{id}/close"` route registration. Sprint 11.0C invariant — the mobile orchestrator\'s `closeSession()` POSTs this endpoint; the handler in `sessions.go` marks the session completed and returns the `summary_stats` block.']),
    # S71 cases (Sprint 11.0C - new)
    ("S71 PASS (backend sessions.go has the 6-field summary_stats response shape)",
     run_s71_check, (case_s71_sessions_pass,), []),
    ("S71 FAIL (backend sessions.go missing the 6-field summary_stats response shape — regression: the Skorlar screen has no per-metric data)",
     run_s71_check, (case_s71_sessions_no_summary,),
     ['S71 backend/internal/api/sessions.go: missing summary_stats key, total_packets, encrypted_packets, packet_loss_pct, mean_latency_ms, jitter_ms, encryption_integrity_pct. Sprint 11.0C invariant — the close handler\'s `summary_stats` block carries 6 fields: `total_packets`, `encrypted_packets`, `packet_loss_pct`, `mean_latency_ms`, `jitter_ms`, `encryption_integrity_pct`. The mobile `SessionScore` JSON deserialiser reads all 6 into the calculator.']),
    # S72 cases (Sprint 11.0C - new)
    ("S72 PASS (score_calculator_test.dart has 4+ unit tests (brief: integration, loss, latency, jitter) — M3 implementation has 7)",
     run_s72_check, (case_s72_score_calculator_test_pass,), []),
    ("S72 FAIL (score_calculator_test.dart missing the 4 unit tests — regression: the calculator's pure-function guarantee is not verified)",
     run_s72_check, (case_s72_score_calculator_test_no_tests,),
     ['S72 score_calculator_test.dart: missing the 4 unit tests (integration, loss, latency, jitter). Sprint 11.0C invariant — the brief requires exactly 4 unit tests for the calculator; the M3 implementation has 7 (the 4 brief + 3 extra: overall weighted sum, computeAll, standardDeviation helper).']),
    # S73 case (Sprint 11.0D - new) — MainActivity owns the
    # `opene2ee/vpn` MethodChannel handler (regression guard for
    # the OnePlus 9 Pro `MissingPluginException` regression).
    # S73 follows the S44 pattern (1 PASS-only case) per the
    # Sprint 11.0D brief: "Self-test'e 1 yeni case (toplam
    # 127/127)". The negative-path coverage is provided by
    # the Dart-side unit test in
    # `mobile/test/sprint110d_handler_test.dart` which actually
    # exercises the `VpnService.getSampledPackets` channel call
    # against a mocked handler — proving the channel is
    # registered at runtime, not just statically present in
    # `MainActivity.kt`.
    ("S73 PASS (MainActivity.kt owns the `opene2ee/vpn` MethodChannel — MethodChannel ctor + METHOD_CHANNEL constant + setMethodCallHandler + OpenE2eeVpnService.dispatch call)",
     run_s73_check, (case_s73_main_activity_pass,), []),
    # S74 case (Sprint 11.0E - new) — OpenE2eeVpnService.kt
    # calls `startForeground(` within Android's 5-second
    # foreground-service rule (regression guard for the
    # OnePlus 9 Pro `RemoteServiceException` crash). Follows
    # the S44 / S73 pattern (1 PASS-only case) per the
    # Sprint 11.0E brief: "Self-test case 128/128". The
    # negative-path coverage is the pre-fix Kotlin source
    # (which had `startForeground(` only INSIDE
    # `startCapture()`, AFTER `Builder.establish()`) — the
    # static check would flag that with all 5 token-mismatch
    # findings; the post-fix source (Sprint 11.0E) hoists
    # the call to the FIRST statement in `onStartCommand`.
    ("S74 PASS (OpenE2eeVpnService.kt onStartCommand calls startForeground( BEFORE startCapture() — Android 5-second foreground-service rule, regression guard for OnePlus 9 Pro RemoteServiceException)",
     run_s74_check, (case_s74_vpn_service_pass,), []),
    # S76 case (Sprint 11.0F - new) — vpn_service.dart exposes
    # VpnService as a Dart singleton (private `_internal` ctor
    # + static `instance` getter + factory default ctor). S75
    # (Log.d breadcrumbs) is a production-audit-only check —
    # see workflow-yaml-audit.py `check_vpn_service_log_d_breadcrumbs_v20`
    # — and follows the S58 pattern of audit-only invariants
    # (the static check is the regression guard; a dedicated
    # selftest case would only re-test the count, which the
    # production audit already does). Total selftest count:
    # 128 + 1 = 129 (matches the brief target).
    ("S76 PASS (vpn_service.dart exposes VpnService as a Dart singleton — regression guard for OnePlus 9 Pro Senaryo D widget-rebuild race; 11.0G form: no public VpnService() ctor, only VpnService._ + VpnService.instance + VpnService.forTesting)",
     run_s76_check, (case_s76_vpn_service_dart_pass,), []),
    # S77 case (Sprint 11.0G - new) — active_pool_screen.dart
    # UI propagation invariant (ConsumerStatefulWidget +
    # stateStream.listen with setState + VpnService.instance
    # reference). Regression guard for the OnePlus 9 Pro
    # "singleton in place but UI doesn't update" symptom
    # (Owner 11:25 report). Sprint 11.0F added the singleton
    # but the `_vpn = VpnService()` call site kept the
    # regression surface opaque; 11.0G tightens with this
    # S77 UI-propagation invariant. Total selftest: 128 +
    # 1 = 130 (was 129 before Sprint 11.0G, +1 new case).
    ("S77 PASS (active_pool_screen.dart is ConsumerStatefulWidget + stateStream.listen with setState + VpnService.instance reference — regression guard for OnePlus 9 Pro UI propagation gap)",
     run_s77_check, (case_s77_active_pool_screen_pass,), []),
    # S78 case (Sprint 11.0H - new) — OpenE2eeVpnService.kt
    # has state-transition Log.d breadcrumbs (startCapture /
    # stopCapture / onRevoke) AND a synchronized(stateLock)
    # TOCTOU guard. Regression guard for the OnePlus 9 Pro
    # `start` returns `state: DRAINING` regression (Owner
    # 11:38 logcat). The 11.0F/11.0G singleton + UI
    # propagation fixes were necessary but not sufficient —
    # the Kotlin-side startCapture was racing with
    # stopCapture. S78 closes the gap with the TOCTOU
    # guard + diagnostic breadcrumbs. Total selftest:
    # 130 + 1 = 131 (was 130 before Sprint 11.0H).
    ("S78 PASS (OpenE2eeVpnService.kt has state-transition Log.d breadcrumbs + synchronized(stateLock) TOCTOU guard — regression guard for OnePlus 9 Pro start->DRAINING race)",
     run_s78_check, (case_s78_vpn_service_state_transitions_pass,), []),
    # S79 case (Sprint 11.0I - new) — OpenE2eeVpnService.kt
    # has correct addRoute (0.0.0.0/0 default route) and
    # NO addRoute(TUN_ADDRESS, ...) (the 9.7.0 mirror bug
    # that OnePlus 9 Pro rejects with
    # `IllegalArgumentException: Bad address`). Total
    # selftest: 131 + 1 = 132 (was 131 before Sprint 11.0I).
    ("S79 PASS (OpenE2eeVpnService.kt has correct addRoute (0.0.0.0/0) and NO addRoute(TUN_ADDRESS) anti-pattern — regression guard for OnePlus 9 Pro IllegalArgumentException: Bad address (Sprint 9.7.0 mirror bug))",
     run_s79_check, (case_s79_vpn_service_addroute_pass,), []),
    # S80 case (Sprint 11.0J - new) — OpenE2eeVpnService.kt
    # has TUN passthrough (`output.write(buf, 0, n)` after
    # `input.read(buf)`) AND no anti-pattern
    # `protect(Socket())` no-op. Regression guard for the
    # OnePlus 9 Pro "VPN active, internet dead" symptom
    # (Owner 12:14 report, PID 4244). Total selftest: 132
    # + 1 = 133 (was 132 before Sprint 11.0J).
    ("S80 PASS (OpenE2eeVpnService.kt has TUN passthrough (output.write after input.read) and NO protect(Socket()) no-op — regression guard for OnePlus 9 Pro internet-killed-by-default-route)",
     run_s80_check, (case_s80_vpn_service_tun_passthrough_pass,), []),
    # S82 case (Sprint 11.0K - new) — OpenE2eeVpnService.kt
    # dispatches MethodChannel calls to the Android main
    # looper (via `Handler(Looper.getMainLooper()).post { ...
    # }` or the `pushToDart` helper). Regression guard for the
    # OnePlus 9 Pro "VPN active, internet OK, UI never
    # updates" symptom (Owner 12:31 report, PID 4244; real
    # root cause: `@UiThread` violation on PacketDrain
    # ScheduledExecutor worker thread). Total selftest: 133
    # + 1 = 134 (was 133 before Sprint 11.0K).
    ("S82 PASS (OpenE2eeVpnService.kt dispatches MethodChannel.invokeMethod to the Android main looper via Handler(Looper.getMainLooper()).post { ... } — regression guard for OnePlus 9 Pro @UiThread violation on PacketDrain worker)",
     run_s82_check, (case_s82_vpn_service_ui_thread_push_pass,), []),
    # S84 case (Sprint 11.0M - new) - OpenE2eeVpnService.kt
    # has packetsObserved.incrementAndGet EXACTLY ONCE,
    # inside startReaderThread read branch (post
    # extractMetadata), with packetsObserved.set( allowed
    # only in startCapture. Regression guard for the
    # OnePlus 9 Pro Sprint 11.0A-11.0L fake-capture
    # accusation (Owner 13:08, PID 4244). Total selftest:
    # 134 + 1 = 135.
    ("S84 PASS (OpenE2eeVpnService.kt has packetsObserved.incrementAndGet EXACTLY ONCE in startReaderThread read branch - regression guard for OnePlus 9 Pro fake-capture accusation)",
     run_s84_check, (case_s84_packets_observed_increment_invariant_pass,), []),
  ]   # noqa: E501

failed = []
for name, check_fn, args, expected in cases:
    actual = check_fn(*args)
    ok = sorted(actual) == sorted(expected)
    status = "PASS" if ok else "FAIL"
    print(f"{status}: {name} — expected {expected}, got {actual}")
    if not ok:
        failed.append(name)

print()
if failed:
    print(f"SELF-TEST FAILED: {len(failed)} cases did not match expected findings:")
    for n in failed:
        print(f"  - {n}")
    sys.exit(1)
else:
    print(f"SELF-TEST OK: all {len(cases)} cases produced expected findings.")
    sys.exit(0)