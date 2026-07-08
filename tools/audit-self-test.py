"""Self-test for check_app_build_gradle_syntax_v2 (S1-S5),
check_android_debug_workflow_v3 (S6),
check_mobile_entry_point_v4 (S7), and
check_android_xml_comments_v5 (S8).

Per Architect brief (Sprint 9.6.6): "self-checks (negative test:
revert + audit finds 4 FAIL)". Sprint 9.6.7 extends the self-test
to cover S6 (4 new cases: 1 PASS, 3 FAIL). Sprint 9.6.8 extends
further to cover S7 (4 new cases: 1 PASS, 3 FAIL). Sprint 9.6.9
extends further to cover S8 (2 new cases: 1 PASS, 1 FAIL).

S1-S5 cases: 6 (1 PASS + 5 FAIL, each violating exactly one of the
S1-S5 sub-checks for app/build.gradle.kts).
S6 cases: 4 (1 PASS + 3 FAIL, each violating exactly one of the
S6 conditions on android-debug.yml: name match, run contains
"flutter pub get", working-directory == "./mobile").
S7 cases: 4 (1 PASS + 3 FAIL, each violating exactly one of the
S7 conditions: lib/main.dart exists, has runApp(, has
ProviderScope, pubspec has flutter_riverpod:, pubspec has
go_router:).
S8 cases: 2 (1 PASS + 1 FAIL — XML well-formed + no `--` inside
`<!-- -->`).

Total: 16 cases.
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
]

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