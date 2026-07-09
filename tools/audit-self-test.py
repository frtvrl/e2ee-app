"""Self-test for check_app_build_gradle_syntax_v2 (S1-S5),
check_android_debug_workflow_v3 (S6),
check_mobile_entry_point_v4 (S7),
check_android_xml_comments_v5 (S8),
check_android_manifest_v6 (S9),
check_android_res_skeleton_v7 (S10),
check_flutter_plugins_dependencies_v8 (S11), and
check_flutter_kotlin_embedding_v9 (S12).

Per Architect brief (Sprint 9.6.6): "self-checks (negative test:
revert + audit finds 4 FAIL)". Sprint 9.6.7 extends to S6.
9.6.8 extends to S7. 9.6.9 extends to S8. 9.6.10 extends to S9.
9.6.11 extends to S10. 9.6.12 extends to S11. 9.6.13 extends to S12.

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

Total: 28 cases.
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

# ─── Run all cases ───────────────────────────────────────────────

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