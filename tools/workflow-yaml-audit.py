"""
PyYAML audit for 4 GH Actions workflows + Gradle wrapper + AGP + Kotlin
+ app/build.gradle.kts syntax invariants.
Per memory rule: PyYAML 1.1 parses `on:` as boolean `True` — use d[True].
Applies to all workflow files; tracks the Sprint 9.6.2 + 9.6.3 + 9.6.4 +
9.6.5 + 9.6.6 fix invariants (added 2026-07-08 after Sprint 9.6.1 PR #13
push CI FAIL, Sprint 9.6.2 PR #14 push CI FAIL, Sprint 9.6.3 PR #15 push
CI FAIL, Sprint 9.6.4 PR #15 (PUSHED) live build test CI FAIL, Sprint
9.6.5 PR #16 (PUSHED) live build test CI FAIL).

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
     with 4 sub-checks:
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
"""
import re
import sys
from pathlib import Path

try:
    import yaml
except ImportError:
    print("PyYAML not installed; install via `pip install pyyaml`", file=sys.stderr)
    sys.exit(1)

WORKFLOWS_DIR = Path(__file__).resolve().parent.parent / ".github" / "workflows"
REPO_ROOT = Path(__file__).resolve().parent.parent
TARGETS = ["android-debug.yml", "ci.yml", "ios.yml", "android-release.yml"]

# Flutter 3.44.1 minimum version pins (env.FLUTTER_VERSION in all 4
# workflows; cross-cycle consistency).
FLUTTER_MIN_GRADLE = (8, 7)
FLUTTER_MIN_AGP = (8, 6)
FLUTTER_MIN_KOTLIN = (2, 2)


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

    if all_findings:
        print("\nFINDINGS:")
        for f in all_findings:
            print(f"  - {f}")
        return 1
    print("\nALL 4 WORKFLOWS + GRADLE WRAPPER + AGP + KOTLIN + SYNTAX v2 PASS PyYAML AUDIT.")
    return 0


if __name__ == "__main__":
    sys.exit(main())