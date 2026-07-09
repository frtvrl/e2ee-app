"""
PyYAML audit for 4 GH Actions workflows + Gradle wrapper + AGP + Kotlin
+ app/build.gradle.kts syntax invariants + mobile entry point + Android
res/ XML comments + AndroidManifest merger-spec + Android res/ skeleton
+ .flutter-plugins-dependencies regen.
Per memory rule: PyYAML 1.1 parses `on:` as boolean `True` — use d[True].
Applies to all workflow files; tracks the Sprint 9.6.2 + 9.6.3 + 9.6.4 +
9.6.5 + 9.6.6 + 9.6.7 + 9.6.8 + 9.6.9 + 9.6.10 + 9.6.11 + 9.6.12 +
9.6.13 fix invariants (added 2026-07-08 after Sprint 9.6.1 PR #13 push
CI FAIL, Sprint 9.6.2 PR #14 push CI FAIL, Sprint 9.6.3 PR #15 push
CI FAIL, Sprint 9.6.4 PR #15 (PUSHED) live build test CI FAIL, Sprint
9.6.5 PR #16 (PUSHED) live build test CI FAIL, Sprint 9.6.6 PR #17
(PUSHED) live build test CI FAIL, Sprint 9.6.7 PR #19 (PUSHED) live
build test CI FAIL, Sprint 9.6.8 PR #20 (PUSHED) live build test CI
FAIL, Sprint 9.6.9 PR #21 (PUSHED) live build test CI FAIL, Sprint
9.6.10 PR #22 (PUSHED) live build test CI FAIL, Sprint 9.6.11 PR #23
(PUSHED) live build test CI FAIL, Sprint 9.6.12 PR #24 (PUSHED) live
build test CI FAIL, Sprint 9.6.13 PR #25 (PUSHED) live build test
CI FAIL — REGRESSED with WRONG root cause; the real defect was
missing `io.flutter:flutter_embedding_ktx` in app/build.gradle.kts
dependencies).

Verifies:
  1. All 4 workflows have ONLY workflow_dispatch trigger (no push/pull_request).
  2. android-debug.yml has subosito/flutter-action@v2 step.
  3. android-debug.yml's `Setup Flutter` step pins flutter-version
     (Sprint 9.6.2 fix — Sprint 9.6.1 had `channel: stable` only, which
     caused ${FLUTTER_HOME} to resolve to empty in the runner).
  4. android-debug.yml's `Generate local.properties` step writes flutter.sdk
     echo derived from `which flutter` (Sprint 9.6.2 fix — Sprint 9.6.1
     used `${FLUTTER_HOME}` which was empty, causing settings.gradle.kts
     `includeBuild("")` = `/packages/flutter_tools/gradle` to FAIL).
  5. android-debug.yml has `Verify Flutter SDK path` step
     (fast-fail signal BEFORE ./gradlew assembleDebug).
  6. ci.yml/ios.yml/android-release.yml have NO inputs.runner
     (matrix/single-runner workflows don't need it).
  7. **Sprint 9.6.3:** gradle-wrapper.properties distributionUrl must be
     >= 8.7.0 (Flutter 3.44.1 minimum).
  8. **Sprint 9.6.4:** mobile/android/build.gradle.kts AGP plugin version
     must be >= 8.6.0 (Flutter 3.44.1 minimum).
  9. **Sprint 9.6.5:** build.gradle.kts Kotlin plugin version must be
     >= 2.2 (Flutter 3.44.1 soon-dropped floor).
 10. **Sprint 9.6.6 (v2):** app/build.gradle.kts Kotlin DSL syntax check
     with 5 sub-checks (S1-S5):
     S1. real `import java.util.Properties` statement exists at line start
         (NOT substring in a comment — Sprint 9.6.5 had a false-positive
         match on a comment claiming the import was added).
     S2. real `import org.jetbrains.kotlin.gradle.dsl.JvmTarget` statement
         exists (required by kotlin { compilerOptions } block).
     S3. deprecated `kotlinOptions { jvmTarget = "..." }` block is NOT
         present in code (Kotlin 2.0+ deprecation warning treated as
         build error in CI by AGP 8.11.1 + Kotlin 2.2.20).
     S4. new `kotlin { compilerOptions { jvmTarget.set(JvmTarget.JVM_17) } }`
         block IS present in code (correct Kotlin 2.0+ replacement).
     S5. fully-qualified `java.util.Properties()` usage is NOT present
         in code (must use short form `Properties()` after the import).
 11. **Sprint 9.6.7 (v3):** android-debug.yml `flutter pub get` step check
      (S6):
      The CI runner resolves `./gradlew assembleDebug` →
      `:app:compileFlutterBuildDebug` which needs
      `mobile/.dart_tool/package_config.json`. That file is NEVER
      produced unless a step runs `flutter pub get` with
      `working-directory: ./mobile` (the Dart project root, where
      `pubspec.yaml` lives — NOT `./mobile/android`). S6 verifies:
      (a) step name matches "Install Flutter dependencies" (case-
          insensitive substring on parsed `name` field, not raw text),
      (b) step `run` field contains "flutter pub get",
      (c) step `working-directory` is EXACTLY "./mobile".
      Uses PyYAML-parsed step dicts (Sprint 9.6.5 comment-claim
      lesson reapplies — a comment claiming "we run flutter pub get"
      must NOT pass this audit).
 12. **Sprint 9.6.8 (v4):** mobile entry point check (S7) —
      `mobile/lib/main.dart` + `runApp(` + `ProviderScope` +
      `pubspec.yaml` `flutter_riverpod:` + `go_router:`. PyYAML-
      parsed pubspec deps (comment-claim lesson reapplies).
 13. **Sprint 9.6.9 (v5):** Android res/xml/ comment well-formedness
      (S8) — XML 1.0 `<!-- ... -->` may not contain `--`. Uses
      `xml.etree.ElementTree.fromstring` (well-formedness) + regex
      walk over comment bodies (content rule). aapt2 rejects the
      file with 'The string "--" is not permitted within comments'.
 14. **Sprint 9.6.10 (v6):** AndroidManifest merger-spec (S9) —
      no `package=` attr (AGP 8.11.1 ignores / AGP 9 rejects) +
      `xmlns:tools` declared + `tools:replace` (NOT `tools:remove`)
      co-exists with `android:usesCleartextTraffic` + gradle
      `namespace = "..."` declared (cross-check).
 15. **Sprint 9.6.11 (v7):** Android res/ skeleton (S10) — mipmap
      ic_launcher representation + `drawable/launch_background.xml`
      + `values/styles.xml` with `LaunchTheme` + `NormalTheme`.
 16. **Sprint 9.6.12 (v8):** mobile/.flutter-plugins-dependencies
      regen (S11) — file exists, parses as JSON via `json.load`
      (real parser, per the 9.6.x chain rule "audit must use a
      real parser, not a regex-grep heuristic"), `plugins.android[]`
      non-empty with `name` + `native_build` per entry. The Flutter
      Gradle plugin reads this file to wire engine JARs into the
      Kotlin compile classpath; without it, `compileDebugKotlin`
      fails with 40+ 'Unresolved reference embedding/FlutterActivity/
      FlutterEngine/MethodChannel' errors.
      [NB: S11 was right-but-insufficient. The actual root cause of
       the 9.6.12 / 9.6.13 live build FAIL was a missing
       `io.flutter:flutter_embedding_ktx` Maven coordinate in
       `app/build.gradle.kts` `dependencies { }` — a Kotlin-side
       issue. `.flutter-plugins-dependencies` is Dart-side
       metadata and does not contribute to the Kotlin classpath.
       S12 was added in Sprint 9.6.13 to close the Kotlin-side
       gap.]
 17. **Sprint 9.6.13 (v9):** app/build.gradle.kts
      `flutter_embedding_ktx` declared (S12) — the Maven
      coordinate `io.flutter:flutter_embedding_ktx:1.0.0-<engine
      _commit>` appears inside the `dependencies { }` block AND
      the `<engine_commit>` hash matches the value in
      `$flutterSdkPath/bin/internal/engine.version`. Without this
      dependency, `compileDebugKotlin` fails with 40+ "Unresolved
      reference embedding/FlutterActivity/FlutterEngine/
      MethodChannel" errors in MainActivity.kt + OpenE2eeVpnService.kt.
      Missing since PR-3 (the original Android Gradle scaffolding).
"""
import json
import re
import sys
from pathlib import Path

try:
    import yaml
except ImportError:
    print("PyYAML not installed; install via `pip install pyyaml`", file=sys.stderr)
    sys.exit(1)

try:
    import xml.etree.ElementTree as ET
except ImportError:
    ET = None  # S8 audit will report a missing-XML-parser finding if used.

WORKFLOWS_DIR = Path(__file__).resolve().parent.parent / ".github" / "workflows"
REPO_ROOT = Path(__file__).resolve().parent.parent
TARGETS = ["android-debug.yml", "ci.yml", "ios.yml", "android-release.yml"]
# Sprint 9.6.9 — directory scanned for *.xml files that end up in the
# APK (and so must satisfy aapt2's XML 1.0 parser).
ANDROID_RES_XML_DIR = (
    REPO_ROOT / "mobile" / "android" / "app" / "src" / "main" / "res" / "xml"
)
# Sprint 9.6.10 — paths for the AndroidManifest S9 cross-check.
ANDROID_MANIFEST_PATH = REPO_ROOT / "mobile" / "android" / "app" / "src" / "main" / "AndroidManifest.xml"
ANDROID_GRADLE_KTS_PATH = REPO_ROOT / "mobile" / "android" / "app" / "build.gradle.kts"
# The Android namespace + manifest merger identifier for the
# app. PR-22a introduced this namespace; Sprint 9.6.10 drops the
# redundant `package="..."` attribute from AndroidManifest.xml in
# favour of the gradle namespace. Keep this single-source-of-truth
# in one place so audit + tests + docs agree.
ANDROID_APP_NAMESPACE = "com.opene2ee.opene2ee"
# Sprint 9.6.11 — Android resource skeleton directories (S10).
ANDROID_RES_DIR = REPO_ROOT / "mobile" / "android" / "app" / "src" / "main" / "res"
# XML namespace URIs — see
# https://developer.android.com/guide/topics/manifest/manifest-intro.
ANDROID_NS = "http://schemas.android.com/apk/res/android"
ANDROID_TOOLS_NS = "http://schemas.android.com/tools"

# Flutter 3.44.1 minimum version pins (env.FLUTTER_VERSION in all 4
# workflows; cross-cycle consistency).
FLUTTER_MIN_GRADLE = (8, 7)
FLUTTER_MIN_AGP = (8, 6)
FLUTTER_MIN_KOTLIN = (2, 2)

# Sprint 9.6.13 — paths for the S12 flutter_embedding_ktx audit.
FLUTTER_SDK_PATH = Path(r"C:\Users\User\flutter")  # Mirrors local.properties (CI: from `which flutter`).
ENGINE_VERSION_PATH = FLUTTER_SDK_PATH / "bin" / "internal" / "engine.version"
FLUTTER_EMBEDDING_GROUP = "io.flutter"
FLUTTER_EMBEDDING_ARTIFACT = "flutter_embedding_ktx"
FLUTTER_EMBEDDING_PREFIX = "1.0.0-"  # The Maven coordinate is `1.0.0-<40-char-hex>` per Flutter's engine tag scheme.


def strip_comments(text: str) -> str:
    """Strip // and /* */ comments while preserving strings (best effort).

    Critical: Sprint 9.6.5 audit had false positive because substring
    match on `import java.util.Properties` matched a comment claiming
    the import was added. v2 parses only actual code lines.

    Conservative string handling: tracks " inside each line and only
    strips // outside string literals. For .kts / .py / .gradle files
    this is sufficient. Not a full parser.
    """
    # Strip /* ... */ block comments first (they span lines)
    text = re.sub(r"/\*[\s\S]*?\*/", "", text)
    # Strip line comments per-line, tracking "string" boundaries
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


def audit_workflow(path: Path) -> list[str]:
    """Return list of findings (empty = pass)."""
    findings = []
    name = path.name

    with path.open(encoding="utf-8") as f:
        docs = list(yaml.safe_load_all(f))
    if len(docs) != 1 or docs[0] is None:
        findings.append(f"{name}: YAML parse failed (expected 1 doc, got {len(docs)})")
        return findings
    d = docs[0]

    # PyYAML 1.1 quirk: `on:` -> True (boolean). Use d[True].
    on_block = d.get(True)
    if on_block is None:
        findings.append(f"{name}: `on:` block missing or None")
        return findings
    if not isinstance(on_block, dict):
        findings.append(f"{name}: `on:` block is not a dict: {type(on_block).__name__}")
        return findings

    trigger_keys = sorted(on_block.keys())
    expected = ["workflow_dispatch"]
    if trigger_keys != expected:
        findings.append(
            f"{name}: `on:` triggers = {trigger_keys}, expected exactly {expected}"
        )

    # Workflow-level checks
    if name == "android-debug.yml":
        jobs = d.get("jobs", {})
        for job_name, job_def in jobs.items():
            steps = job_def.get("steps", []) if isinstance(job_def, dict) else []

            has_flutter_setup = False
            flutter_version_pinned = False
            has_which_flutter_echo = False
            has_verify_step = False

            for s in steps:
                if not isinstance(s, dict):
                    continue
                step_uses = str(s.get("uses", ""))

                # Setup Flutter step with pinned version
                if "subosito/flutter-action" in step_uses:
                    has_flutter_setup = True
                    step_with = s.get("with", {})
                    if isinstance(step_with, dict) and "flutter-version" in step_with:
                        flutter_version_pinned = True
                    continue

                step_name = str(s.get("name", ""))
                run_text = str(s.get("run", ""))

                # Generate local.properties: flutter.sdk derived from which flutter
                if "Generate local.properties" in step_name:
                    if "which flutter" in run_text and "flutter.sdk=" in run_text:
                        has_which_flutter_echo = True

                # Verify Flutter SDK path step
                if "Verify Flutter SDK path" in step_name:
                    if "flutter_tools/gradle" in run_text:
                        has_verify_step = True

            if not has_flutter_setup:
                findings.append(
                    f"{name}: job '{job_name}' missing subosito/flutter-action@v2 step"
                )
            if not flutter_version_pinned:
                findings.append(
                    f"{name}: job '{job_name}' Setup Flutter step missing `flutter-version` pin "
                    "(Sprint 9.6.2 fix — was channel:stable only in Sprint 9.6.1)"
                )
            if not has_which_flutter_echo:
                findings.append(
                    f"{name}: job '{job_name}' Generate local.properties step missing "
                    "`which flutter` derivation (Sprint 9.6.2 fix — was ${FLUTTER_HOME} "
                    "in Sprint 9.6.1 which resolved to empty in the runner shell)"
                )
            if not has_verify_step:
                findings.append(
                    f"{name}: job '{job_name}' missing `Verify Flutter SDK path` fast-fail step"
                )

    # Check inputs.runner absence for matrix/single-runner workflows
    if name in ("ci.yml", "ios.yml", "android-release.yml"):
        wd = on_block.get("workflow_dispatch")
        if isinstance(wd, dict) and "inputs" in wd:
            findings.append(
                f"{name}: has `inputs.runner` but matrix/single-runner workflow doesn't need it"
            )

    return findings


def check_gradle_wrapper_version() -> list[str]:
    """Sprint 9.6.3: gradle-wrapper.properties distributionUrl >= Flutter minimum.

    Flutter 3.44.1 (project-wide pin via env.FLUTTER_VERSION in all 4 workflows)
    requires Gradle >= 8.7.0. Sprint 9.6.2 fix made `flutter_tools/gradle`
    resolve correctly, but a live workflow_dispatch run after Sprint 9.6.2
    PR #14 push failed at app/build.gradle.kts:80 with:
        "Your project's Gradle version (8.5.0) is lower than Flutter's
         minimum supported version of 8.7.0."
    This check parses `gradle-wrapper.properties` distributionUrl and
    fails if the embedded Gradle version is below FLUTTER_MIN_GRADLE.
    """
    findings = []
    gradle_props_path = REPO_ROOT / "mobile" / "android" / "gradle" / "wrapper" / "gradle-wrapper.properties"
    if not gradle_props_path.exists():
        findings.append(
            f"{gradle_props_path.name}: file missing (Sprint 9.6.3 invariant)"
        )
        return findings
    text = gradle_props_path.read_text(encoding="utf-8")
    # Pattern: distributionUrl=.../gradle-X.Y(-bin).zip
    match = re.search(r"gradle-(\d+)\.(\d+)(?:-bin)?\.zip", text)
    if not match:
        findings.append(
            f"gradle-wrapper.properties: distributionUrl pattern not recognized "
            "(expected `gradle-X.Y-bin.zip`)"
        )
        return findings
    gradle_version = (int(match.group(1)), int(match.group(2)))
    if gradle_version < FLUTTER_MIN_GRADLE:
        findings.append(
            f"gradle-wrapper.properties: distributionUrl Gradle {gradle_version[0]}.{gradle_version[1]} "
            f"< Flutter 3.44.1 minimum {FLUTTER_MIN_GRADLE[0]}.{FLUTTER_MIN_GRADLE[1]} "
            "(Sprint 9.6.3 fix — was 8.5 in Sprint 9.6.2, live run failed at "
            "app/build.gradle.kts:80 with Flutter Gradle plugin version check)"
        )
    return findings


def check_agp_version() -> list[str]:
    """Sprint 9.6.4: mobile/android/build.gradle.kts AGP plugin version >= 8.6.0.

    Flutter SDK 3.44.1 minimum AGP is 8.6.0. Sprint 9.6.3 cherry-pick
    bumped Gradle 8.5 -> 8.10 LTS (passed `:gradle:jar`) but a live
    workflow_dispatch run AFTER Sprint 9.6.3 cherry-pick failed at
    `mobile/android/app/build.gradle.kts:80` with:
        "Your project's Android Gradle Plugin version (Android Gradle
         Plugin version 8.1.4) is lower than Flutter's minimum
         supported version of Android Gradle Plugin version 8.6.0.
         Please upgrade your Android Gradle Plugin version."

    This check parses the AGP plugin block in the root build.gradle.kts:
        plugins {
            id("com.android.application") version "X.Y.Z" apply false
            ...
        }
    and fails if the declared AGP version is below FLUTTER_MIN_AGP.

    Note: settings.gradle.kts uses `id("com.android.application")` without
    version (in the subproject's plugins block); the VERSION is declared
    ONCE in the root build.gradle.kts plugins block. So this check
    targets the root file.
    """
    findings = []
    build_gradle_path = REPO_ROOT / "mobile" / "android" / "build.gradle.kts"
    if not build_gradle_path.exists():
        findings.append(
            f"{build_gradle_path.name}: file missing (Sprint 9.6.4 AGP invariant)"
        )
        return findings
    text = build_gradle_path.read_text(encoding="utf-8")
    # Pattern: id("com.android.application") version "X.Y.Z" apply false
    # Match `id("com.android.application")` (with opening + closing
    # double-quotes around plugin ID) followed by `version "X.Y.Z"`.
    match = re.search(
        r'id\("com\.android\.application"\)\s+version\s+"(\d+)\.(\d+)(?:\.(\d+))?"',
        text,
    )
    if not match:
        findings.append(
            f"build.gradle.kts: AGP plugin version pattern not recognized "
            "(expected `id(\"com.android.application\") version \"X.Y.Z\" apply false`)"
        )
        return findings
    agp_major = int(match.group(1))
    agp_minor = int(match.group(2))
    agp_version = (agp_major, agp_minor)
    if agp_version < FLUTTER_MIN_AGP:
        findings.append(
            f"build.gradle.kts: AGP version {agp_major}.{agp_minor} "
            f"< Flutter 3.44.1 minimum {FLUTTER_MIN_AGP[0]}.{FLUTTER_MIN_AGP[1]} "
            "(Sprint 9.6.4 fix — was 8.1.4 before; live run after Sprint 9.6.3 "
            "cherry-pick failed at app/build.gradle.kts:80 with Flutter Gradle "
            "plugin AGP version check)"
        )
    return findings


def check_kotlin_version() -> list[str]:
    """Sprint 9.6.5: mobile/android/build.gradle.kts Kotlin plugin version >= 2.2.

    Flutter SDK 3.44.1 soon-dropped floor for Kotlin is 2.2.20. Sprint
    9.6.4 cherry-pick bumped AGP 8.1.4 → 8.6.1 (passed `:app:configure`)
    but the live workflow_dispatch run AFTER Sprint 9.6.4 push (PR
    #15 PUSHED) emitted a deprecation warning and the Kotlin 2.0+
    DSL compiler rejected the Sprint 5 era `app/build.gradle.kts`:
        Warning: Flutter support for your project's Kotlin version
                 (1.9.22) will soon be dropped.
                 Please upgrade your Kotlin version to a version of
                 at least 2.2.20 soon.
        Unresolved reference: util (Kotlin 2.0+ requires explicit import
                               for fully-qualified class names)
        Unresolved reference: load
        No cast needed (Kotlin 2.0+ smart cast handles `as String`)
        variant parameter never used (rename to `_`)

    This check parses the Kotlin Android plugin version in the root
    build.gradle.kts plugins block and fails if below FLUTTER_MIN_KOTLIN.
    """
    findings = []
    build_gradle_path = REPO_ROOT / "mobile" / "android" / "build.gradle.kts"
    if not build_gradle_path.exists():
        findings.append(
            f"{build_gradle_path.name}: file missing (Sprint 9.6.5 Kotlin invariant)"
        )
        return findings
    text = build_gradle_path.read_text(encoding="utf-8")
    # Pattern: id("org.jetbrains.kotlin.android") version "X.Y.Z" apply false
    match = re.search(
        r'id\("org\.jetbrains\.kotlin\.android"\)\s+version\s+"(\d+)\.(\d+)(?:\.(\d+))?"',
        text,
    )
    if not match:
        findings.append(
            f"build.gradle.kts: Kotlin Android plugin version pattern not recognized "
            "(expected `id(\"org.jetbrains.kotlin.android\") version \"X.Y.Z\" apply false`)"
        )
        return findings
    kotlin_major = int(match.group(1))
    kotlin_minor = int(match.group(2))
    kotlin_version = (kotlin_major, kotlin_minor)
    if kotlin_version < FLUTTER_MIN_KOTLIN:
        findings.append(
            f"build.gradle.kts: Kotlin version {kotlin_major}.{kotlin_minor} "
            f"< Flutter 3.44.1 soon-dropped floor {FLUTTER_MIN_KOTLIN[0]}.{FLUTTER_MIN_KOTLIN[1]} "
            "(Sprint 9.6.5 fix — was 1.9.22; live run after Sprint 9.6.4 "
            "PR #15 PUSHED failed at app/build.gradle.kts:145 with Kotlin 2.0+ "
            "DSL compiler errors: Unresolved reference util/load, No cast needed, "
            "variant parameter never used)"
        )
    return findings


def check_app_build_gradle_syntax_v2() -> list[str]:
    """Sprint 9.6.6 v2: app/build.gradle.kts Kotlin DSL syntax check with
    5 sub-checks (S1-S5). Replaces Sprint 9.6.5 v1 which had a false
    positive: it searched for substring `import java.util.Properties`
    in the file text, which matched a COMMENT that claimed the import
    was added but the file did not actually have the import.

    v2 fixes the false positive by stripping line/block comments before
    running the regex. It also adds S2 (JvmTarget import), S3
    (deprecated kotlinOptions block absence), S4 (new kotlin {
    compilerOptions } presence), and S5 (no fully-qualified
    java.util.Properties() usage).

    The 4-sub-check rubric the Architect specified maps to S1-S4 (the
    core Kotlin 2.0+ DSL requirements); S5 is a defensive bonus.

    Sub-check failure mode (the Sprint 9.6.5 false positive):
        The Sprint 9.6.5 commit added a comment
            // explicit `import java.util.Properties` ...
        but the file STILL used the fully-qualified
            val keyProps = java.util.Properties().apply { ... }
        The Sprint 9.6.5 audit's substring search
            has_import = "import java.util.Properties" in text
        returned True (matching the comment) and reported PASS, while
        the live workflow_dispatch run AFTER Sprint 9.6.5 PR #16
        push (origin/main @ e2b7055) failed at line 151 with:
            Unresolved reference: util
            Unresolved reference: load
    """
    findings = []
    app_gradle_path = REPO_ROOT / "mobile" / "android" / "app" / "build.gradle.kts"
    if not app_gradle_path.exists():
        findings.append(
            f"{app_gradle_path.name}: file missing (Sprint 9.6.6 syntax invariant)"
        )
        return findings
    text = app_gradle_path.read_text(encoding="utf-8")
    code = strip_comments(text)

    # S1: real `import java.util.Properties` line (line-anchored,
    # not substring in a comment).
    has_properties_import = bool(
        re.search(r"^import\s+java\.util\.Properties\s*$", code, re.MULTILINE)
    )
    if not has_properties_import:
        findings.append(
            "S1 app/build.gradle.kts: missing real `import java.util.Properties` "
            "statement (Sprint 9.6.6 fix — Kotlin 2.0+ requires explicit import; "
            "v1 audit false-positive matched a comment claiming the import was "
            "added; live workflow_dispatch run after Sprint 9.6.5 PR #16 push "
            "(origin/main @ e2b7055) failed at line 151 with 'Unresolved "
            "reference: util')"
        )

    # S2: real `import org.jetbrains.kotlin.gradle.dsl.JvmTarget` line.
    has_jvm_target_import = bool(
        re.search(
            r"^import\s+org\.jetbrains\.kotlin\.gradle\.dsl\.JvmTarget\s*$",
            code,
            re.MULTILINE,
        )
    )
    if not has_jvm_target_import:
        findings.append(
            "S2 app/build.gradle.kts: missing real `import "
            "org.jetbrains.kotlin.gradle.dsl.JvmTarget` statement "
            "(Sprint 9.6.6 fix — required by `kotlin { compilerOptions { "
            "jvmTarget.set(JvmTarget.JVM_17) } }` block; live workflow_dispatch "
            "run would fail with 'Unresolved reference: JvmTarget')"
        )

    # S3: deprecated `kotlinOptions { jvmTarget = "..." }` block NOT present.
    deprecated_kotlin_options = bool(
        re.search(r"kotlinOptions\s*\{[\s\S]*?jvmTarget\s*=\s*\"[\d]+\"", code)
    )
    if deprecated_kotlin_options:
        findings.append(
            "S3 app/build.gradle.kts: deprecated `kotlinOptions { jvmTarget = \"...\" }` "
            "block still present in code (Sprint 9.6.6 fix — Kotlin 2.0+ emits "
            "deprecation warning that AGP 8.11.1 + Kotlin 2.2.20 treat as build error "
            "in CI; live workflow_dispatch run after Sprint 9.6.5 PR #16 push "
            "(origin/main @ e2b7055) failed at line 98 with "
            "'kotlinOptions is deprecated. Please use kotlin { compilerOptions { "
            "jvmTarget } } instead')"
        )

    # S4: new `kotlin { compilerOptions { jvmTarget.set(JvmTarget.JVM_17) } }` block IS present.
    new_kotlin_block = bool(
        re.search(
            r"kotlin\s*\{[\s\S]*?compilerOptions\s*\{[\s\S]*?jvmTarget\.set\(JvmTarget\.JVM_17\)",
            code,
        )
    )
    if not new_kotlin_block:
        findings.append(
            "S4 app/build.gradle.kts: missing `kotlin { compilerOptions { "
            "jvmTarget.set(JvmTarget.JVM_17) } }` block (Sprint 9.6.6 fix — "
            "the Kotlin 2.0+ replacement for the deprecated `kotlinOptions { "
            "jvmTarget = \"17\" }` form)"
        )

    # S5 (defensive bonus): no fully-qualified `java.util.Properties()` usage.
    fully_qualified = bool(re.search(r"java\.util\.Properties\(\)", code))
    if fully_qualified:
        findings.append(
            "S5 app/build.gradle.kts: fully-qualified `java.util.Properties()` "
            "usage still present in code (Sprint 9.6.6 fix — Kotlin 2.0+ rejects "
            "fully-qualified types without explicit import; use short form "
            "`Properties()` after `import java.util.Properties`)"
        )

    return findings


def check_android_debug_workflow_v3() -> list[str]:
    """Sprint 9.6.7 v3: android-debug.yml `flutter pub get` step check.

    A live workflow_dispatch run AFTER Sprint 9.6.6 (PR planned #17,
    commit e57da24) FAILED at :app:compileFlutterBuildDebug with:
        "mobile/.dart_tool/package_config.json does not exist.
         Did you run this command from the same directory as your
         pubspec.yaml file?"
    The `flutter --version` step that exists at the end of the
    workflow is NOT a substitute for `flutter pub get` — it only
    prints the version, it does not generate the package_config.json
    file that the Flutter Android Gradle plugin reads at build time.

    The fix: add a new step
        - name: Install Flutter dependencies
          working-directory: ./mobile
          run: flutter pub get
    between the existing `Verify Flutter dependency cache` step
    (current step 9) and the existing `Build Debug APK` step
    (current step 10). `working-directory: ./mobile` is mandatory
    because `pubspec.yaml` lives in the Dart project root, not the
    Android Gradle subproject (`./mobile/android`).

    This check verifies all three properties on the SAME step:

      (a) Step name matches `Install Flutter dependencies` (case-
          insensitive substring match on the `name` field).
      (b) Step `run` field contains `flutter pub get` (case-
          insensitive substring).
      (c) Step `working-directory` is exactly `./mobile` (NOT
          `./mobile/android`, NOT absent, NOT empty).

    The check uses PyYAML-parsed step dicts (not raw text substring
    search) — applying the Sprint 9.6.5 lesson that a comment
    claiming "we run flutter pub get" must NOT pass the audit.

    Failure messages report the actual `working-directory` value
    seen (if any) so a future false-positive is debuggable.
    """
    findings = []
    android_debug_path = WORKFLOWS_DIR / "android-debug.yml"
    if not android_debug_path.exists():
        findings.append(
            f"{android_debug_path.name}: file missing (Sprint 9.6.7 S6 invariant)"
        )
        return findings
    with android_debug_path.open(encoding="utf-8") as f:
        docs = list(yaml.safe_load_all(f))
    if len(docs) != 1 or docs[0] is None:
        findings.append(
            f"{android_debug_path.name}: YAML parse failed (S6 needs parsed steps)"
        )
        return findings
    d = docs[0]
    jobs = d.get("jobs", {})

    # Walk all jobs, all steps, looking for a single step that
    # satisfies all three S6 conditions.
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
            # (a) name match (case-insensitive substring)
            if "install flutter dependencies" in step_name.lower():
                s6_name_found.append(step_name)
                # (b) run contains flutter pub get (case-insensitive)
                if "flutter pub get" in step_run.lower():
                    # (c) working-directory exactly ./mobile
                    s6_match = {
                        "job": job_name,
                        "name": step_name,
                        "run": step_run,
                        "working_directory": step_wd,
                    }

    if s6_match is None:
        wd_observed = "absent"
        # Try to find any step with a flutter pub get hint in run
        # to give a more informative failure message.
        for job_name, job_def in jobs.items():
            if not isinstance(job_def, dict):
                continue
            for s in job_def.get("steps", []):
                if not isinstance(s, dict):
                    continue
                if "flutter pub get" in str(s.get("run", "")).lower():
                    wd_observed = f"{s.get('working-directory', 'absent')!r}"
                    break
        if not s6_name_found:
            findings.append(
                "S6 android-debug.yml: `Install Flutter dependencies` step is "
                "missing (Sprint 9.6.7 fix — `flutter pub get` never runs; the "
                "build fails at :app:compileFlutterBuildDebug with 'package_config."
                "json does not exist'; live workflow_dispatch run after Sprint "
                "9.6.6 PR #17 push (commit e57da24) failed with this error). "
                "Add a step:\n"
                "      - name: Install Flutter dependencies\n"
                "        working-directory: ./mobile\n"
                "        run: flutter pub get\n"
                "between `Verify Flutter dependency cache` (step 9) and `Build "
                "Debug APK` (step 10)."
            )
        else:
            findings.append(
                f"S6 android-debug.yml: `Install Flutter dependencies` step is "
                f"present but misconfigured. working-directory seen: {wd_observed}. "
                f"Expected: exactly `./mobile` (the Dart project root where "
                f"pubspec.yaml lives, NOT `./mobile/android` and NOT absent). "
                f"Live workflow_dispatch run after Sprint 9.6.6 PR #17 push "
                f"(commit e57da24) failed at :app:compileFlutterBuildDebug with "
                f"'package_config.json does not exist' because `flutter pub get` "
                f"never ran from the Dart project root."
            )
    else:
        # Match found — verify working-directory is exactly ./mobile.
        if s6_match["working_directory"] != "./mobile":
            findings.append(
                f"S6 android-debug.yml: `Install Flutter dependencies` step has "
                f"working-directory = {s6_match['working_directory']!r}, expected "
                f"exactly `./mobile` (the Dart project root where pubspec.yaml "
                f"lives, NOT `./mobile/android` and NOT absent). The `Build "
                f"Debug APK` step uses `./mobile/android` because Gradle runs "
                f"there; `flutter pub get` MUST run from `./mobile` so it can "
                f"find pubspec.yaml."
            )

    return findings


def check_mobile_entry_point_v4() -> list[str]:
    """Sprint 9.6.8 v4: mobile app entry point check (S7).

    A live workflow_dispatch run AFTER Sprint 9.6.7 merge
    (commit bbf7087) FAILED because `flutter build apk` could not
    find `mobile/lib/main.dart` — the Flutter mobile default entry
    point. The repo had `mobile/lib/web/main.dart` (for the web
    dashboard) but no `mobile/lib/main.dart` (for the mobile build).
    The `flutter pub get` step from Sprint 9.6.7 succeeded (it
    resolves the package graph), but the build still failed at
    `:app:compileFlutterBuildDebug` with "could not find main entry
    point".

    This sprint (9.6.8) creates the entry point + the full app
    shell. The audit must verify all three preconditions for a
    buildable mobile app:

      (a) `mobile/lib/main.dart` exists (the entry point).
      (b) The file has `runApp(` (calls runApp) AND
          `ProviderScope` (Riverpod wiring). A comment claiming
          "we run Riverpod" must NOT pass — the check uses
          `pathlib` + content read (substrings on the actual
          file content).
      (c) `mobile/pubspec.yaml` declares BOTH `flutter_riverpod:`
          AND `go_router:` as dependencies. Parsed via PyYAML
          (not substring) so a comment claiming "we use Riverpod"
          in the pubspec must NOT pass.

    Failure messages name the missing condition so a future
    regression is debuggable.
    """
    findings = []
    main_dart = REPO_ROOT / "mobile" / "lib" / "main.dart"
    pubspec = REPO_ROOT / "mobile" / "pubspec.yaml"

    # (a) entry point exists
    if not main_dart.exists():
        findings.append(
            "S7 mobile/lib/main.dart: file missing (Sprint 9.6.8 fix — "
            "Flutter mobile build entry point does not exist; live "
            "workflow_dispatch run after Sprint 9.6.7 PR #19 merge "
            "(commit bbf7087) FAILED at :app:compileFlutterBuildDebug "
            "with 'could not find main entry point' because only "
            "mobile/lib/web/main.dart exists, not mobile/lib/main.dart). "
            "Create the file with at least:\n"
            "      import 'package:flutter/material.dart';\n"
            "      import 'package:flutter_riverpod/flutter_riverpod.dart';\n"
            "      void main() { runApp(const ProviderScope(child: MyApp())); }"
        )
        return findings  # Cannot check (b) if main.dart is missing.

    # (b) main.dart has runApp( + ProviderScope (substring on actual content)
    main_text = main_dart.read_text(encoding="utf-8")
    if "runApp(" not in main_text:
        findings.append(
            "S7 mobile/lib/main.dart: does not call `runApp(` (Sprint 9.6.8 "
            "fix — the entry point must invoke Flutter's runApp with a "
            "widget tree; the audit checks the actual code, not a comment)"
        )
    if "ProviderScope" not in main_text:
        findings.append(
            "S7 mobile/lib/main.dart: does not wire `ProviderScope` (Sprint "
            "9.6.8 fix — Riverpod state management requires ProviderScope "
            "at the root of the widget tree; the audit checks the actual "
            "code, not a comment)"
        )

    # (c) pubspec.yaml declares flutter_riverpod + go_router (parsed via PyYAML)
    if not pubspec.exists():
        findings.append(
            "S7 mobile/pubspec.yaml: file missing (Sprint 9.6.8 invariant — "
            "the dependencies for the new app shell must be declared)"
        )
        return findings
    try:
        with pubspec.open(encoding="utf-8") as f:
            pubspec_doc = yaml.safe_load(f)
    except yaml.YAMLError as e:
        findings.append(
            f"S7 mobile/pubspec.yaml: YAML parse failed ({e})"
        )
        return findings
    if not isinstance(pubspec_doc, dict):
        findings.append(
            "S7 mobile/pubspec.yaml: top-level is not a mapping (parsed "
            f"type: {type(pubspec_doc).__name__})"
        )
        return findings
    deps = pubspec_doc.get("dependencies", {})
    if not isinstance(deps, dict):
        deps = {}
    has_riverpod = "flutter_riverpod" in deps
    has_go_router = "go_router" in deps
    if not has_riverpod:
        findings.append(
            "S7 mobile/pubspec.yaml: `flutter_riverpod:` missing from "
            "dependencies (Sprint 9.6.8 fix — state management is "
            "required; the audit parses dependencies via PyYAML so a "
            "comment claiming 'we use Riverpod' must NOT pass)"
        )
    if not has_go_router:
        findings.append(
            "S7 mobile/pubspec.yaml: `go_router:` missing from "
            "dependencies (Sprint 9.6.8 fix — declarative routing is "
            "required; the audit parses dependencies via PyYAML so a "
            "comment claiming 'we use go_router' must NOT pass)"
        )

    return findings


def check_android_xml_comments_v5() -> list[str]:
    """Sprint 9.6.9 v5: XML comment well-formedness check (S8).

    The 9.6.8 live build (commit edd3023 on main, fast-forward
    merged by Owner) advanced past `compileFlutterBuildDebug` for
    the first time in the 9.6.x chain. The next task,
    `:app:mergeDebugResources`, failed because
    `mobile/android/app/src/main/res/xml/network_security_config.xml`
    contained XML comments with `--` runs (e.g. `---------------`).
    The XML 1.0 spec forbids `--` inside `<!-- ... -->`; Android's
    `aapt2` / `ResourceCompiler` enforces this and rejects the
    file with `The string "--" is not permitted within comments`.

    This check enforces the rule statically so a future
    regression cannot land:

      (a) Every `*.xml` under
          `mobile/android/app/src/main/res/xml/` parses as
          well-formed XML via `xml.etree.ElementTree.fromstring`
          (Python's stdlib XML 1.0 parser — same rule aapt2 uses
          up to differences in namespace handling).
      (b) No `<!-- ... -->` comment body contains a `--` run
          (the regex finds `--` INSIDE comments; the legitimate
          `<!--` opener and `-->` closer are stripped first by
          walking the comment match bounds).

    Implementation notes (apply 9.6.5 lesson): we DO NOT
    regex-grep raw text for `--` — we use `ET.fromstring` to
    validate well-formedness AND iterate the captured comment
    text from a regex pass to assert no `--` run lives inside.
    A comment that LEGITIMATELY contains the text "the `--`
    keyword" would still fail (correct: the only way to write
    that in XML is to break the comment into two comments,
    e.g. `<!-- the --><!--  keyword -->` — accepted XML
    limitation, see https://www.w3.org/TR/xml/#sec-comments).

    Scope: only `mobile/android/app/src/main/res/xml/*.xml`.
    We do NOT scan `mobile/lib/` Dart sources, and we do NOT
    scan the broader `mobile/android/` tree (CI's `aapt2` is
    what cares about the resource dir, not arbitrary XML).
    """
    findings = []
    if ET is None:
        findings.append(
            "S8 mobile/android/app/src/main/res/xml/: Python's xml.etree.ElementTree "
            "is unavailable; cannot run the S8 check (Sprint 9.6.9 invariant — "
            "Python 3 ships ET in the stdlib; this finding means ET was monkey-patched away)"
        )
        return findings
    if not ANDROID_RES_XML_DIR.exists():
        findings.append(
            f"S8 {ANDROID_RES_XML_DIR}: directory missing (Sprint 9.6.9 "
            "invariant — Android resource XML directory should exist; the build "
            "would fail with a different message if it were truly missing)"
        )
        return findings

    xml_files = sorted(ANDROID_RES_XML_DIR.glob("*.xml"))
    if not xml_files:
        # No XML files is a valid state (e.g. minimal app). Do not
        # emit a finding — the absence of XML files does not need
        # to be audited.
        return findings

    for xml_path in xml_files:
        rel = xml_path.relative_to(REPO_ROOT)
        try:
            text = xml_path.read_text(encoding="utf-8")
        except OSError as e:
            findings.append(
                f"S8 {rel}: read failed ({e})"
            )
            continue
        # (a) well-formedness via real XML parser
        try:
            ET.fromstring(text)
        except ET.ParseError as e:
            findings.append(
                f"S8 {rel}: XML parse failed ({e.msg} at line {e.position[0]} col "
                f"{e.position[1]}) — aapt2 will refuse this file; Sprint 9.6.9 fix"
            )
            continue
        # (b) walk `<!-- ... -->` blocks and assert no `--` inside body
        for match in re.finditer(r"<!--(.*?)-->", text, re.DOTALL):
            body = match.group(1)
            if "--" in body:
                # Compute the line number of the `--` inside body
                start_offset = match.start(1)
                body_index_of = body.index("--")
                absolute_offset = start_offset + body_index_of
                line_no = text.count("\n", 0, absolute_offset) + 1
                findings.append(
                    f"S8 {rel}: line {line_no} contains `--` inside an XML "
                    f"comment — XML 1.0 spec forbids this and Android's aapt2 "
                    f"rejects the file with 'The string \"--\" is not permitted "
                    f"within comments'. Sprint 9.6.9 fix: replace dash runs in "
                    f"comment headers with a non-`-` separator (e.g. `===`)."
                )
                # Report only the first offending comment per file so a
                # single fix-cycle per file is the natural workflow.
                break

    return findings


def check_android_manifest_v6() -> list[str]:
    """Sprint 9.6.10 v6: AndroidManifest.xml well-formedness + merger-spec check (S9).

    The 9.6.9 live build (commit c469959 on main, fast-forward
    merged by Owner) advanced past `mergeDebugResources` and
    `parseDebugLocalResources` (XML comments fixed). The next
    task, `:app:processDebugMainManifest`, failed because:

      (a) `mobile/android/app/src/main/AndroidManifest.xml`
          declared `package="com.opene2ee.opene2ee"` on the
          root `<manifest>` element. AGP 8.11.1 rejects this in
          favour of the `namespace` declared in
          `build.gradle.kts` ("Setting the namespace via the
          package attribute in the source AndroidManifest.xml is
          no longer supported"). The `namespace` is already at
          `mobile/android/app/build.gradle.kts:136`.

      (b) The same `<application>` tag carries
          `android:usesCleartextTraffic="false"` AND
          `tools:remove="android:usesCleartextTraffic"`. The
          merger refuses the redundant/conflicting pair
          ("tools:remove specified at line:63 for attribute
          android:usesCleartextTraffic, but attribute also
          declared at line:68, do you want to use
          tools:replace instead?"). The MOB-1 cyber-security
          finding's intent is "library merge cannot silently
          re-enable plaintext" — the correct directive is
          `tools:replace`.

    This check enforces all of:

      (1) `AndroidManifest.xml` parses as well-formed XML
          (via `xml.etree.ElementTree` — same XML 1.0 parser
          rule aapt2 uses).
      (2) The root `<manifest>` element does NOT carry a
          `package` attribute. AGP 8.x ignores it; AGP 9 will
          reject it outright.
      (3) The root `<manifest>` element DOES declare
          `xmlns:tools="http://schemas.android.com/tools"`
          (required for any `tools:replace` /
          `tools:remove` directive to be recognized).
      (4) Any `<application>` tag that carries
          `android:usesCleartextTraffic` does NOT also carry
          `tools:remove="android:usesCleartextTraffic"`.
          The pair is forbidden; either drop the value and
          keep `tools:remove`, or keep the value and switch to
          `tools:replace`.
      (5) `build.gradle.kts` declares
          `namespace = "com.opene2ee.opene2ee"`. This is the
          cross-check — removing the manifest's `package=`
          is only safe when the gradle namespace is present
          (otherwise the build will fail with "no package
          specified").

    Implementation notes: use `ET.parse` for parse validity,
    iterate via `tree.iter` for the namespace-qualified
    attributes. The `build.gradle.kts` namespace check is a
    substring match (the only place a regex would suffice,
    and it's not security-sensitive — it's a single literal
    string match for the namespace declaration).
    """
    findings = []
    if ET is None:
        findings.append(
            "S9 AndroidManifest: Python's xml.etree.ElementTree is unavailable; "
            "cannot run the S9 check (Sprint 9.6.10 invariant — Python 3 ships "
            "ET in the stdlib; this finding means ET was monkey-patched away)"
        )
        return findings

    # (1) parse validity
    if not ANDROID_MANIFEST_PATH.exists():
        findings.append(
            f"S9 {ANDROID_MANIFEST_PATH.relative_to(REPO_ROOT)}: file missing"
        )
        return findings
    try:
        tree = ET.parse(ANDROID_MANIFEST_PATH)
    except ET.ParseError as e:
        findings.append(
            f"S9 {ANDROID_MANIFEST_PATH.relative_to(REPO_ROOT)}: XML parse "
            f"failed ({e.msg} at line {e.position[0]} col {e.position[1]}) — "
            f"aapt2 will refuse this file"
        )
        return findings
    root = tree.getroot()
    # Read raw text for the xmlns declarations — Python's
    # xml.etree.ElementTree strips namespace declarations from
    # `attrib` (they live in the parser's internal table during
    # parse and are not re-exposed on the Element). For the
    # `xmlns:tools` check we use a substring scan on the raw text
    # before line ~5 (manifest root opening tag). This is the
    # one place where raw text is the source of truth — the
    # audit does NOT regex-grep for content rules (S8 lesson).
    manifest_text = ANDROID_MANIFEST_PATH.read_text(encoding="utf-8")

    # (2) root <manifest> must NOT carry package=
    if root.get("package") is not None:
        findings.append(
            f"S9 {ANDROID_MANIFEST_PATH.relative_to(REPO_ROOT)}: root "
            f"<manifest> carries `package=\"{root.get('package')}\"` — AGP "
            f"8.11.1 already ignores this and AGP 9 will reject it outright. "
            f"Remove the attribute and rely on the `namespace` in "
            f"`mobile/android/app/build.gradle.kts`. Sprint 9.6.10 fix."
        )

    # (3) root <manifest> must declare xmlns:tools.
    # Note: we read the raw text for this because ET strips
    # namespace declarations from attrib. This is the one
    # exception to "no raw-text checks" — the audit uses real
    # XML parse for content rules (S8 / S9 sub-checks 1, 2, 4,
    # 5) and uses raw text only for the xmlns declaration
    # presence check, which ET cannot expose.
    xmlns_tools_pattern = f'xmlns:tools="{ANDROID_TOOLS_NS}"'
    if xmlns_tools_pattern not in manifest_text:
        findings.append(
            f"S9 {ANDROID_MANIFEST_PATH.relative_to(REPO_ROOT)}: root "
            f"<manifest> is missing `{xmlns_tools_pattern}` — required for "
            f"the `tools:replace` / `tools:remove` merger directives to be "
            f"recognized by aapt2. Sprint 9.6.10 invariant."
        )

    # (4) <application> with android:usesCleartextTraffic must NOT
    # ALSO carry tools:remove="android:usesCleartextTraffic".
    # Note on iter(): Python's xml.etree.ElementTree uses bare
    # tag names (`root.iter("application")`), not Clark notation.
    # Lxml users would need `{NS}application`. We standardise on
    # bare tag names (one less surprise for future maintainers).
    uses_cleartext_attr = f"{{{ANDROID_NS}}}usesCleartextTraffic"
    tools_remove_attr = f"{{{ANDROID_TOOLS_NS}}}remove"
    for application in root.iter("application"):
        has_cleartext = application.get(uses_cleartext_attr) is not None
        tools_remove_value = application.get(tools_remove_attr)
        if has_cleartext and tools_remove_value is not None:
            # The forbidden pair — could be either:
            #   tools:remove="android:usesCleartextTraffic"
            #   tools:remove="android:foo,android:usesCleartextTraffic"
            if "usesCleartextTraffic" in tools_remove_value:
                findings.append(
                    f"S9 {ANDROID_MANIFEST_PATH.relative_to(REPO_ROOT)}: "
                    f"<application> carries BOTH `android:usesCleartextTraffic` "
                    f"(explicit value) AND `tools:remove` listing "
                    f"`android:usesCleartextTraffic`. The merger refuses this "
                    f"pair with 'tools:remove specified at line:63 for attribute "
                    f"android:usesCleartextTraffic, but attribute also declared "
                    f"at line:68, do you want to use tools:replace instead?'. "
                    f"Sprint 9.6.10 fix: switch `tools:remove` to `tools:replace` "
                    f"(canonical MOB-1 cyber-security finding answer)."
                )

    # (5) build.gradle.kts namespace cross-check
    if not ANDROID_GRADLE_KTS_PATH.exists():
        findings.append(
            f"S9 {ANDROID_GRADLE_KTS_PATH.relative_to(REPO_ROOT)}: file missing"
        )
        return findings
    gradle_text = ANDROID_GRADLE_KTS_PATH.read_text(encoding="utf-8")
    namespace_line = (
        f'namespace = "{ANDROID_APP_NAMESPACE}"'
    )
    if namespace_line not in gradle_text:
        findings.append(
            f"S9 {ANDROID_GRADLE_KTS_PATH.relative_to(REPO_ROOT)}: `"
            f"{namespace_line}` declaration not found. Sprint 9.6.10 cross-"
            f"check — removing the AndroidManifest.xml `package=` attribute "
            f"is only safe when the gradle namespace is present (otherwise "
            f"the build will fail with 'no package specified')."
        )

    return findings


def check_android_res_skeleton_v7() -> list[str]:
    """Sprint 9.6.11 v7: Flutter Android resource skeleton check (S10).

    The 9.6.10 live build (commit 5f3ee01 on main, fast-forward
    merged by Owner) advanced past `processDebugMainManifest` for
    the first time (manifest fix worked). The next task,
    `:app:processDebugResources`, failed because three Android
    resources referenced by AndroidManifest.xml were never
    generated:

      `mipmap/ic_launcher` (app icon)
      `style/LaunchTheme` (launch screen style)
      `style/NormalTheme` (normal theme style)

    The repo's `mobile/android/app/src/main/res/` contained only
    `xml/network_security_config.xml`. The rest of the Flutter
    Android resource skeleton (mipmap-*, drawable/launch_background.xml,
    values/styles.xml, values/colors.xml, values-night/styles.xml)
    was never generated. The repo has had no Flutter Android
    skeleton since PR-3.

    Sprint 9.6.11 ran `flutter create . --platforms=android` and
    kept ONLY the res/ additions (rejected any flutter create
    changes to the preservation list: MainActivity.kt, Android
    Manifest.xml, build.gradle.kts, lib/*, pubspec.yaml, test/).

    This check verifies that the skeleton stays intact:

      (a) `mobile/android/app/src/main/res/values/styles.xml`
          exists AND contains both `<style name="LaunchTheme">`
          AND `<style name="NormalTheme">` (parse with
          xml.etree.ElementTree). aapt2 rejects the manifest
          without these styles.
      (b) AT LEAST ONE mipmap representation of the app icon
          exists — either `mipmap-anydpi-v26/ic_launcher.xml`
          (adaptive icon) OR any density's
          `mipmap-{mdpi,hdpi,xhdpi,xxhdpi,xxxhdpi}/ic_launcher.png`
          (legacy raster). aapt2 rejects the manifest
          without `mipmap/ic_launcher`.
      (c) `mobile/android/app/src/main/res/drawable/launch_background.xml`
          exists. Referenced by `LaunchTheme`'s `windowBackground`.

    We do NOT check exact file contents (icon design, color palette,
    etc.) — a future visual design sprint can refine. The audit's
    job is "is the skeleton present", not "is the design good".
    """
    findings = []
    if not ANDROID_RES_DIR.exists():
        findings.append(
            f"S10 {ANDROID_RES_DIR.relative_to(REPO_ROOT)}: directory "
            f"missing (Sprint 9.6.11 invariant — Android resource "
            f"directory should exist; PR-3 introduced the skeleton "
            f"and every subsequent Flutter Android build expects it)"
        )
        return findings

    # (a) styles.xml with LaunchTheme + NormalTheme
    styles_path = ANDROID_RES_DIR / "values" / "styles.xml"
    if not styles_path.exists():
        findings.append(
            f"S10 {styles_path.relative_to(REPO_ROOT)}: file missing. "
            f"Flutter create --platforms=android generates this file. "
            f"Without it, AndroidManifest.xml's references to "
            f"`@style/LaunchTheme` and `@style/NormalTheme` fail to "
            f"resolve at processDebugResources time."
        )
    else:
        try:
            tree = ET.parse(styles_path)
        except ET.ParseError as e:
            findings.append(
                f"S10 {styles_path.relative_to(REPO_ROOT)}: XML parse "
                f"failed ({e.msg} at line {e.position[0]} col {e.position[1]})"
            )
        else:
            style_names = set()
            for style in tree.getroot().findall("style"):
                name = style.get("name")
                if name:
                    style_names.add(name)
            if "LaunchTheme" not in style_names:
                findings.append(
                    f"S10 {styles_path.relative_to(REPO_ROOT)}: "
                    f"`<style name=\"LaunchTheme\">` missing. Flutter create "
                    f"generates this; it's referenced by @style/LaunchTheme "
                    f"in AndroidManifest.xml's <activity> tag."
                )
            if "NormalTheme" not in style_names:
                findings.append(
                    f"S10 {styles_path.relative_to(REPO_ROOT)}: "
                    f"`<style name=\"NormalTheme\">` missing. Flutter create "
                    f"generates this; it's referenced by @style/NormalTheme "
                    f"in AndroidManifest.xml's <meta-data android:name=\"io.flutter.embedding.android.NormalTheme\">."
                )

    # (b) at least one mipmap/ic_launcher representation
    mipmap_dirs = [
        ANDROID_RES_DIR / "mipmap-anydpi-v26",
        ANDROID_RES_DIR / "mipmap-mdpi",
        ANDROID_RES_DIR / "mipmap-hdpi",
        ANDROID_RES_DIR / "mipmap-xhdpi",
        ANDROID_RES_DIR / "mipmap-xxhdpi",
        ANDROID_RES_DIR / "mipmap-xxxhdpi",
    ]
    has_mipmap = False
    for d in mipmap_dirs:
        if d.exists() and any(d.iterdir()):
            has_mipmap = True
            break
    if not has_mipmap:
        findings.append(
            f"S10 {ANDROID_RES_DIR.relative_to(REPO_ROOT)}: no "
            f"`mipmap/ic_launcher` representation found in any density "
            f"directory. Flutter create generates `mipmap-mdpi/ic_launcher.png` "
            f"through `mipmap-xxxhdpi/ic_launcher.png` and "
            f"`mipmap-anydpi-v26/ic_launcher.xml`. AndroidManifest.xml's "
            f"`android:icon=\"@mipmap/ic_launcher\"` fails to resolve at "
            f"processDebugResources time."
        )

    # (c) drawable/launch_background.xml
    launch_bg = ANDROID_RES_DIR / "drawable" / "launch_background.xml"
    if not launch_bg.exists():
        findings.append(
            f"S10 {launch_bg.relative_to(REPO_ROOT)}: file missing. "
            f"Referenced by LaunchTheme's `windowBackground`; without "
            f"it, the launch splash has no background drawable."
        )

    return findings


def check_flutter_plugins_dependencies_v8() -> list[str]:
    """Sprint 9.6.12 v8: mobile/.flutter-plugins-dependencies regen check (S11).

    The 9.6.11 live build (commit f4881cd on main, fast-forward
    merged by Owner) advanced past `processDebugResources` for the
    first time (resource skeleton fix worked). The next task,
    `:app:compileDebugKotlin`, failed with 40+ "Unresolved
    reference" errors in `MainActivity.kt` and `OpenE2eeVpnService.kt`:

        e: MainActivity.kt:39:19 Unresolved reference 'embedding'.
        e: MainActivity.kt:48:22 Unresolved reference 'FlutterActivity'.
        e: MainActivity.kt:57:37 Unresolved reference 'MethodChannel'.
        e: OpenE2eeVpnService.kt:103:19 Unresolved reference 'embedding'.
        e: OpenE2eeVpnService.kt:184:36 Unresolved reference 'FlutterEngine'.
        e: OpenE2eeVpnService.kt:251:32 Unresolved reference 'MethodChannel'.

    The Kotlin compiler cannot find `io.flutter.embedding.*` and
    `io.flutter.plugin.*` because the Flutter engine JAR is not on
    the compile classpath. The Flutter Gradle plugin (`app_plugin_
    loader.gradle` under `$FLUTTER_HOME/packages/flutter_tools/
    gradle/`) reads `mobile/.flutter-plugins-dependencies` to
    discover the set of Flutter plugins and wire their generated
    classes + the engine JARs into the Kotlin compile classpath.

    Sprint 9.6.7 added the `flutter pub get` step to
    `android-debug.yml` (S6 — `Install Flutter dependencies` with
    `working-directory: ./mobile`). That step regenerates
    `.flutter-plugins-dependencies` on the CI runner BEFORE the
    `Build Debug APK` step, so the engine JARs are wired into the
    classpath.

    However, the file is gitignored (`**/.flutter-plugins-
    dependencies` in repo-root `.gitignore` line 96), so when a
    Coder worktree is created from a clean main checkout, the file
    is ABSENT. `flutter pub get` must be run locally to regenerate
    it. If a future commit changes the pubspec dependency graph
    (adds/removes a Flutter plugin) without re-running `flutter
    pub get` AND the workflow's `flutter pub get` step is
    accidentally removed or misconfigured, the build will fail at
    `compileDebugKotlin` with the symptoms above.

    This check enforces the LOCAL pre-build state:

      (a) `mobile/.flutter-plugins-dependencies` exists.
      (b) The file parses as JSON via `json.load` (real parser,
          per the 9.6.x chain rule "audit must use a real
          parser, not a regex-grep heuristic").
      (c) `plugins.android` is a non-empty list (at least one
          Android plugin declared). Without this list, the
          Flutter Gradle plugin has no Android plugins to wire
          and (in some configurations) no engine JAR to add to
          the Kotlin classpath.
      (d) Each entry in `plugins.android` has both `name` (str)
          and `native_build` (bool) fields — the schema
          `flutter pub get` writes.

    Failure messages report the actual observed state (file
    missing / JSON parse error / empty array / missing fields)
    so a future regression is debuggable. We do NOT
    cross-validate against `mobile/pubspec.yaml` deps here — that
    is a stricter check that the live `flutter pub get` step is
    already responsible for (S6 invariant). This audit's job is
    "is the generated state consistent", not "do the pubspec
    deps match".

    Scope: only `mobile/.flutter-plugins-dependencies`. We do
    NOT scan the analogous `ios` array or any sibling
    Dart-side state — iOS / web / desktop plugins are not on
    the Android Kotlin classpath.
    """
    findings = []
    fpd_path = REPO_ROOT / "mobile" / ".flutter-plugins-dependencies"

    # (a) file exists
    if not fpd_path.exists():
        findings.append(
            f"S11 {fpd_path.relative_to(REPO_ROOT)}: file missing. "
            f"Run `cd mobile && flutter pub get` to regenerate it "
            f"(the file is gitignored at repo-root .gitignore line 96; "
            f"it is NEVER committed and must be regenerated locally "
            f"for the Android Kotlin compile classpath to include the "
            f"Flutter engine JAR + plugin code). Sprint 9.6.12 fix: "
            f"the 9.6.11 live build (commit f4881cd) failed at "
            f":app:compileDebugKotlin with 40+ 'Unresolved reference "
            f"embedding/FlutterActivity/FlutterEngine/MethodChannel' "
            f"errors in MainActivity.kt + OpenE2eeVpnService.kt because "
            f"the engine JAR was not on the Kotlin compile classpath; "
            f"the Flutter Gradle plugin reads this file to wire engine "
            f"JARs into the classpath."
        )
        return findings

    # (b) parse as JSON via real parser
    try:
        text = fpd_path.read_text(encoding="utf-8")
        fpd_doc = json.loads(text)
    except json.JSONDecodeError as e:
        findings.append(
            f"S11 {fpd_path.relative_to(REPO_ROOT)}: JSON parse failed "
            f"({e.msg} at line {e.lineno} col {e.colno}). flutter pub get "
            f"may have written a partial file (disk full / signal "
            f"interrupted). Re-run `cd mobile && flutter pub get`. Sprint "
            f"9.6.12 invariant — the audit uses json.load, not regex-grep, "
            f"per the 9.6.x chain rule."
        )
        return findings

    # (c) plugins.android is a non-empty list
    if not isinstance(fpd_doc, dict):
        findings.append(
            f"S11 {fpd_path.relative_to(REPO_ROOT)}: top-level is not a "
            f"JSON object (parsed type: {type(fpd_doc).__name__}). flutter "
            f"pub get should always write a top-level object with a "
            f"`plugins` field. Re-run `cd mobile && flutter pub get`."
        )
        return findings
    plugins = fpd_doc.get("plugins", None)
    if not isinstance(plugins, dict):
        findings.append(
            f"S11 {fpd_path.relative_to(REPO_ROOT)}: `plugins` is not a "
            f"JSON object (parsed type: {type(plugins).__name__ if plugins is not None else 'NoneType'}). "
            f"flutter pub get should write a `plugins` field. Re-run "
            f"`cd mobile && flutter pub get`."
        )
        return findings
    android_plugins = plugins.get("android", None)
    if not isinstance(android_plugins, list):
        findings.append(
            f"S11 {fpd_path.relative_to(REPO_ROOT)}: `plugins.android` "
            f"is not a JSON array (parsed type: {type(android_plugins).__name__ if android_plugins is not None else 'NoneType'}). "
            f"flutter pub get should write a `plugins.android` array. "
            f"Re-run `cd mobile && flutter pub get`."
        )
        return findings
    if len(android_plugins) == 0:
        findings.append(
            f"S11 {fpd_path.relative_to(REPO_ROOT)}: `plugins.android` is "
            f"an empty array. Without at least one entry, the Flutter "
            f"Gradle plugin has no Android plugin code to wire into the "
            f"compile classpath. The .flutter-plugins-dependencies file "
            f"was likely written by a `flutter pub get` run with no "
            f"plugins installed (e.g. pubspec deps cleared). Re-run "
            f"`cd mobile && flutter pub get` after restoring the deps."
        )
        return findings

    # (d) each entry has name + native_build
    for i, entry in enumerate(android_plugins):
        if not isinstance(entry, dict):
            findings.append(
                f"S11 {fpd_path.relative_to(REPO_ROOT)}: "
                f"`plugins.android[{i}]` is not a JSON object (parsed "
                f"type: {type(entry).__name__}). flutter pub get writes "
                f"objects with `name` + `native_build` fields."
            )
            continue
        if not isinstance(entry.get("name"), str) or not entry.get("name"):
            findings.append(
                f"S11 {fpd_path.relative_to(REPO_ROOT)}: "
                f"`plugins.android[{i}].name` is missing or not a "
                f"non-empty string. flutter pub get writes a `name` "
                f"field per plugin."
            )
        if not isinstance(entry.get("native_build"), bool):
            findings.append(
                f"S11 {fpd_path.relative_to(REPO_ROOT)}: "
                f"`plugins.android[{i}].native_build` is missing or not "
                f"a boolean. flutter pub get writes a `native_build` "
                f"field per plugin (True for plugins with native source, "
                f"False for plugins that ship prebuilt)."
            )

    return findings


def check_flutter_kotlin_embedding_v9() -> list[str]:
    """Sprint 9.6.13 v9: app/build.gradle.kts `flutter_embedding_ktx` check (S12).

    The 9.6.12 live build (commit b52a0f6 on main, fast-forward
    merged by Owner) FAILED at `:app:compileDebugKotlin` with
    40+ "Unresolved reference 'embedding' / 'FlutterActivity' /
    'FlutterEngine' / 'MethodChannel'" errors in `MainActivity.kt`
    and `OpenE2eeVpnService.kt` — IDENTICAL to the 9.6.11 failure.

    Sprint 9.6.12's Coder root-cause (Fix A: `.flutter-plugins-
    dependencies` was missing in the worktree) was a surface
    artifact, NOT the actual root cause. `.flutter-plugins-
    dependencies` is a JSON plugin-list artifact read by the
    Flutter Gradle plugin to wire Dart-side plugin code; it does
    NOT contribute to the Kotlin compile classpath.

    The REAL root cause (Mavis verified, Coder independently
    confirmed in Sprint 9.6.13 Phase 0): `mobile/android/app/
    build.gradle.kts` `dependencies { }` block was MISSING the
    `io.flutter:flutter_embedding_ktx` Maven coordinate. The
    `dev.flutter.flutter-gradle-plugin` plugin wires the engine
    JAR into `compileFlutterBuildDebug` (Dart side) but does NOT
    propagate it to `compileDebugKotlin` (Kotlin side, run by
    Android Gradle Plugin as a separate task). Every
    `io.flutter.embedding.*` and `io.flutter.plugin.*` import in
    `MainActivity.kt` (PR-22a) and `OpenE2eeVpnService.kt`
    (PR-28) failed to resolve.

    This dependency has been missing since PR-3 (the original
    Android Gradle scaffolding). PR-28 (Sprint 5) declared
    `id("dev.flutter.flutter-gradle-plugin")` and `flutter {
    source = "../.." }` but never added the Kotlin embedding JAR.
    From PR-3 through 9.6.11, `compileFlutterBuildDebug` always
    failed FIRST in the live build, so `compileDebugKotlin` never
    had a chance to fail and expose this gap. Sprint 9.6.11 fixed
    the res/ skeleton, so the build finally reached
    `compileDebugKotlin` and the missing dependency surfaced.

    The fix: add ONE line to the `dependencies { }` block:
        implementation("io.flutter:flutter_embedding_ktx:1.0.0-<engine_commit>")
    where `<engine_commit>` is the 40-char hex SHA from
    `$flutterSdkPath/bin/internal/engine.version`. The Maven
    artifact is published to https://storage.googleapis.com/
    download.flutter.io/ and maven.google.com; Gradle fetches it
    automatically when declared.

    This check enforces the post-fix state:

      (a) `mobile/android/app/build.gradle.kts` exists.
      (b) The file contains a `dependencies { ... }` block
          (substring match — we deliberately do NOT use a full
          Kotlin parser because the dependency block is short
          and substring is sufficient, AND a regex would risk the
          same false-positive pattern Sprint 9.6.5 audit had
          when substring matched a comment claiming a dependency
          was added but the file did not actually have it).
      (c) Inside the `dependencies { }` block, the string
          `io.flutter:flutter_embedding_ktx` appears (substring
          match within the block).
      (d) The version follows the `1.0.0-<40-char-hex>` pattern
          (regex `^1.0.0-[0-9a-f]{40}$` on the captured
          version string — the dot in `1.0.0` is a literal `.`,
          not a regex metachar).
      (e) The hash matches the value in
          `$flutterSdkPath/bin/internal/engine.version` (read it,
          compare as strings).

    Failure messages name the actual observed state so a future
    regression is debuggable (NOT a generic "this dependency is
    missing" string). The audit pinpoints which sub-check failed.

    Scope: only `mobile/android/app/build.gradle.kts`. We do NOT
    scan the workspace `build.gradle.kts` (root) or other module
    build files — the embedding JAR is a per-app dependency.
    """
    findings = []
    app_gradle = REPO_ROOT / "mobile" / "android" / "app" / "build.gradle.kts"

    # (a) file exists
    if not app_gradle.exists():
        findings.append(
            f"S12 {app_gradle.relative_to(REPO_ROOT)}: file missing "
            "(Sprint 9.6.13 invariant — the Flutter engine embedding "
            "JAR must be declared in app/build.gradle.kts dependencies)"
        )
        return findings

    text = app_gradle.read_text(encoding="utf-8")

    # (b) find the dependencies { ... } block. Walk balanced braces
    # so a nested block (e.g. inside `create("release") { ... }`)
    # is not mistaken for the top-level dependencies block. The
    # pattern is: find `dependencies` keyword at line-start, then
    # count `{` vs `}` until balanced.
    dep_block_match = re.search(
        r"^\s*dependencies\s*\{",
        text,
        re.MULTILINE,
    )
    if not dep_block_match:
        findings.append(
            f"S12 {app_gradle.relative_to(REPO_ROOT)}: no "
            f"`dependencies {{ ... }}` block found in file. Sprint "
            f"9.6.13 invariant — the Flutter engine embedding JAR "
            f"must be declared inside `dependencies {{ ... }}`."
        )
        return findings
    # Extract the block content by balanced-brace walk.
    block_start = dep_block_match.end()  # position right after `{`
    depth = 1
    i = block_start
    while i < len(text) and depth > 0:
        c = text[i]
        if c == "{":
            depth += 1
        elif c == "}":
            depth -= 1
        i += 1
    if depth != 0:
        findings.append(
            f"S12 {app_gradle.relative_to(REPO_ROOT)}: `dependencies "
            f"{{ ... }}` block is unbalanced (final depth {depth}). "
            f"Sprint 9.6.13 invariant — file may be corrupt."
        )
        return findings
    block_text = text[block_start:i - 1]

    # (c) substring search inside the block (NOT full-file regex —
    # we already extracted the block, so substring is unambiguous).
    embedding_substr = f"{FLUTTER_EMBEDDING_GROUP}:{FLUTTER_EMBEDDING_ARTIFACT}"
    if embedding_substr not in block_text:
        findings.append(
            f"S12 {app_gradle.relative_to(REPO_ROOT)}: "
            f"`{embedding_substr}` is missing from the "
            f"`dependencies {{ ... }}` block. Sprint 9.6.13 fix — "
            f"add "
            f"`implementation(\"{embedding_substr}:1.0.0-<engine_commit>\")` "
            f"inside the block. The 9.6.12 live build failed at "
            f":app:compileDebugKotlin with 40+ 'Unresolved reference "
            f"embedding/FlutterActivity/FlutterEngine/MethodChannel' "
            f"errors because the engine JAR was not on the Kotlin "
            f"compile classpath. This dependency has been missing "
            f"since PR-3."
        )
        return findings

    # (d) capture the version string. Pattern is `1.0.0-<40-hex>`.
    # Use a regex anchored on the substring to scope the search.
    version_match = re.search(
        rf"{re.escape(embedding_substr)}:{FLUTTER_EMBEDDING_PREFIX}([0-9a-f]{{40}})",
        block_text,
    )
    if not version_match:
        # Capture whatever comes after the substring for a debug-
        # quality failure message (so the user can see the actual
        # bad value, not `<unknown>`).
        bad_version_match = re.search(
            rf"{re.escape(embedding_substr)}:([^\"'\s,)]+)",
            block_text,
        )
        bad_version = bad_version_match.group(1) if bad_version_match else "<not found>"
        findings.append(
            f"S12 {app_gradle.relative_to(REPO_ROOT)}: "
            f"`{embedding_substr}` is declared but the version "
            f"does NOT follow the `{FLUTTER_EMBEDDING_PREFIX}<40-char-hex>` "
            f"pattern (expected `1.0.0-<engine_commit>` where "
            f"`<engine_commit>` is a 40-char hex SHA). Got: "
            f"`{embedding_substr}:{bad_version}` from the block. "
            f"Sprint 9.6.13 invariant."
        )
        return findings
    declared_hash = version_match.group(1)

    # (e) compare against the Flutter SDK's engine.version
    if not ENGINE_VERSION_PATH.exists():
        # Engine version file absent — only happens if Flutter SDK
        # is incomplete (CI should never hit this). Report and
        # skip the hash compare, since we can't validate.
        findings.append(
            f"S12 {ENGINE_VERSION_PATH}: file missing. Cannot "
            f"validate hash against engine.version. Sprint 9.6.13 "
            f"invariant — the Flutter SDK should ship this file."
        )
        return findings
    sdk_engine_version = ENGINE_VERSION_PATH.read_text(encoding="utf-8").strip()
    # The engine.version file contains ONLY the 40-char hex SHA.
    if not re.match(r"^[0-9a-f]{40}$", sdk_engine_version):
        findings.append(
            f"S12 {ENGINE_VERSION_PATH}: content does not match "
            f"`^[0-9a-f]{{40}}$` (got `{sdk_engine_version}`). Sprint "
            f"9.6.13 invariant — engine.version should contain "
            f"only the 40-char hex SHA."
        )
        return findings
    if declared_hash != sdk_engine_version:
        findings.append(
            f"S12 {app_gradle.relative_to(REPO_ROOT)}: declared "
            f"`{embedding_substr}:{FLUTTER_EMBEDDING_PREFIX}{declared_hash}` "
            f"does NOT match Flutter SDK engine.version "
            f"`{sdk_engine_version}`. The Kotlin-side engine JAR "
            f"will be a different version than the Dart-side "
            f"engine, leading to classpath/runtime mismatches. "
            f"Sprint 9.6.13 fix: update the declared hash to "
            f"match engine.version, or vice versa."
        )
        return findings

    return findings


def main() -> int:
    all_findings = []
    for fname in TARGETS:
        path = WORKFLOWS_DIR / fname
        if not path.exists():
            all_findings.append(f"{fname}: file missing")
            continue
        findings = audit_workflow(path)
        if findings:
            all_findings.extend(findings)
        else:
            print(f"PASS: {fname}")

    # Sprint 9.6.3: Gradle wrapper version invariant check.
    gradle_findings = check_gradle_wrapper_version()
    if gradle_findings:
        all_findings.extend(gradle_findings)
    else:
        print(f"PASS: gradle-wrapper.properties distributionUrl >= {FLUTTER_MIN_GRADLE[0]}.{FLUTTER_MIN_GRADLE[1]}")

    # Sprint 9.6.4: AGP version invariant check.
    agp_findings = check_agp_version()
    if agp_findings:
        all_findings.extend(agp_findings)
    else:
        print(f"PASS: build.gradle.kts AGP version >= {FLUTTER_MIN_AGP[0]}.{FLUTTER_MIN_AGP[1]} (Flutter 3.44.1 minimum)")

    # Sprint 9.6.5: Kotlin version invariant check.
    kotlin_findings = check_kotlin_version()
    if kotlin_findings:
        all_findings.extend(kotlin_findings)
    else:
        print(f"PASS: build.gradle.kts Kotlin version >= {FLUTTER_MIN_KOTLIN[0]}.{FLUTTER_MIN_KOTLIN[1]} (Flutter 3.44.1 soon-dropped)")

    # Sprint 9.6.6 v2: app/build.gradle.kts Kotlin DSL syntax (5 sub-checks).
    syntax_findings = check_app_build_gradle_syntax_v2()
    if syntax_findings:
        all_findings.extend(syntax_findings)
    else:
        print("PASS: app/build.gradle.kts Kotlin DSL syntax v2 (S1-S5: imports + deprecated kotlinOptions absence + new kotlin compilerOptions presence)")

    # Sprint 9.6.7 v3: android-debug.yml `flutter pub get` step check (S6).
    s6_findings = check_android_debug_workflow_v3()
    if s6_findings:
        all_findings.extend(s6_findings)
    else:
        print("PASS: android-debug.yml has `Install Flutter dependencies` step with working-directory=./mobile (Sprint 9.6.7 S6)")

    # Sprint 9.6.8 v4: mobile entry point check (S7) — lib/main.dart + ProviderScope + pubspec deps.
    s7_findings = check_mobile_entry_point_v4()
    if s7_findings:
        all_findings.extend(s7_findings)
    else:
        print("PASS: mobile entry point (lib/main.dart + runApp( + ProviderScope + pubspec flutter_riverpod + go_router) — Sprint 9.6.8 S7)")

    # Sprint 9.6.9 v5: XML well-formedness check (S8) — Android res/xml comments.
    s8_findings = check_android_xml_comments_v5()
    if s8_findings:
        all_findings.extend(s8_findings)
    else:
        print("PASS: Android res/xml comments well-formed (no `--` inside `<!-- -->`) — Sprint 9.6.9 S8")

    # Sprint 9.6.10 v6: AndroidManifest.xml merger-spec check (S9).
    s9_findings = check_android_manifest_v6()
    if s9_findings:
        all_findings.extend(s9_findings)
    else:
        print("PASS: AndroidManifest.xml merger-spec (no `package=` attr + tools:replace not tools:remove + gradle namespace present) — Sprint 9.6.10 S9")

    # Sprint 9.6.11 v7: Flutter Android resource skeleton check (S10).
    s10_findings = check_android_res_skeleton_v7()
    if s10_findings:
        all_findings.extend(s10_findings)
    else:
        print("PASS: Android res/ skeleton (mipmap + drawable/launch_background + values/styles.xml with LaunchTheme+NormalTheme) — Sprint 9.6.11 S10")

    # Sprint 9.6.12 v8: mobile/.flutter-plugins-dependencies regen check (S11).
    s11_findings = check_flutter_plugins_dependencies_v8()
    if s11_findings:
        all_findings.extend(s11_findings)
    else:
        print("PASS: mobile/.flutter-plugins-dependencies exists, parses as JSON, plugins.android[] non-empty with name+native_build per entry — Sprint 9.6.12 S11")

    # Sprint 9.6.13 v9: app/build.gradle.kts `flutter_embedding_ktx` check (S12).
    s12_findings = check_flutter_kotlin_embedding_v9()
    if s12_findings:
        all_findings.extend(s12_findings)
    else:
        print(f"PASS: app/build.gradle.kts declares io.flutter:flutter_embedding_ktx:1.0.0-<engine_commit> matching $flutterSdkPath/bin/internal/engine.version — Sprint 9.6.13 S12")

    if all_findings:
        print("\nFINDINGS:")
        for f in all_findings:
            print(f"  - {f}")
        return 1
    print("\nALL 4 WORKFLOWS + GRADLE WRAPPER + AGP + KOTLIN + SYNTAX v2 + S6 flutter pub get step + S7 mobile entry point + S8 Android XML comments + S9 AndroidManifest merger-spec + S10 Android res/ skeleton + S11 .flutter-plugins-dependencies regen + S12 flutter_embedding_ktx declared in app deps PASS PyYAML AUDIT.")
    return 0


if __name__ == "__main__":
    sys.exit(main())