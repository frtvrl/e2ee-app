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
check_pool_provider_timer_periodic_literal_present (S28),
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
check_main_activity_get_sampled_packets_v13 (S43), and
check_whatsapp_deeplink_wa_me_format_v14 (S44).

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

Total: 72 cases.
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
    """Sprint 10.1A: Timer.periodic literal in pool provider (S28).

    Mirrors check_pool_provider_timer_periodic_literal_present. The
    file `mobile/lib/state/pool_provider.dart` must contain the
    literal `Timer.periodic` (used for the 3-second mock ticker).
    """
    findings = []
    if pool_provider_text is None:
        findings.append("S28 fail (file missing)")
        return findings
    if "Timer.periodic" not in pool_provider_text:
        findings.append("S28 fail (literal missing)")
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
    # S28 cases (Sprint 10.1A - new)
    ("S28 PASS (pool_provider.dart contains the literal `Timer.periodic` for 3s mock ticker)",
     run_s28_check, ("_timer = Timer.periodic(const Duration(seconds: 3), (_) => _tick());\n",), []),
    ("S28 FAIL (pool_provider.dart missing the literal `Timer.periodic` - regression: periyodik update replaced with one-shot)",
     run_s28_check, ("// _timer removed; using Future.delayed loop instead\n",), ["S28 fail (literal missing)"]),
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