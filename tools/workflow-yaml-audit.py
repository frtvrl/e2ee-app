"""
PyYAML audit for 4 GH Actions workflows + Gradle wrapper version invariant.
Per memory rule: PyYAML 1.1 parses `on:` as boolean `True` — use d[True].
Applies to all workflow files; tracks the Sprint 9.6.2 + 9.6.3 fix invariants
(added 2026-07-08 after Sprint 9.6.1 PR #13 push CI FAIL and Sprint 9.6.2
PR #14 push CI FAIL).

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
     >= 8.7.0 (Flutter 3.44.1 minimum). Sprint 9.6.2 fix made
     `flutter_tools/gradle` resolve correctly, but a live workflow_dispatch
     run after Sprint 9.6.2 PR #14 push failed at app/build.gradle.kts:80
     with "Your project's Gradle version (8.5.0) is lower than Flutter's
     minimum supported version of 8.7.0".
"""
import re
import yaml
import sys
from pathlib import Path

WORKFLOWS_DIR = Path(__file__).resolve().parent.parent / ".github" / "workflows"
REPO_ROOT = Path(__file__).resolve().parent.parent
TARGETS = ["android-debug.yml", "ci.yml", "ios.yml", "android-release.yml"]

# Flutter 3.44.1 minimum Gradle version (set in env.FLUTTER_VERSION in all
# 4 workflows; cross-cycle consistency).
FLUTTER_MIN_GRADLE = (8, 7)


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

    if all_findings:
        print("\nFINDINGS:")
        for f in all_findings:
            print(f"  - {f}")
        return 1
    print("\nALL 4 WORKFLOWS + GRADLE WRAPPER PASS PyYAML AUDIT.")
    return 0


if __name__ == "__main__":
    sys.exit(main())