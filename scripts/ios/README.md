# scripts/ios/

iOS-specific developer scripts for OpenE2EE mobile. These scripts are
**macOS-only** (require Xcode + xcodebuild); they are NOT executed in CI
on Linux/Windows runners because Apple's toolchain is not available
there.

The iOS Xcode project (`mobile/ios/Runner.xcodeproj/project.pbxproj`)
is committed as a hand-written scaffold (PR-25). The scripts in this
directory are for **future maintenance** — e.g. regenerating the pbxproj
after adding files, or running Flutter's standard `flutter create` to
reconcile against an upstream template.

## Available scripts

| Script                          | Purpose                                                                   |
| ------------------------------- | ------------------------------------------------------------------------- |
| `verify-xcodeproj.sh`           | Run the 8 static-validation checks from `docs/ios/xcodeproj-structure.md` on a freshly-checked-out tree. Exits non-zero if any check fails. |

## Future scripts (planned)

| Script                          | Purpose                                                                   |
| ------------------------------- | ------------------------------------------------------------------------- |
| `regenerate-pbxproj.sh`         | Run XcodeGen with `project.yml` spec to re-emit `project.pbxproj` after file moves. Tracked as Sprint 4+ follow-up. |
| `sign-and-build.sh`             | One-shot build with adhoc signing for local dev installs.                 |

## Why no Windows PowerShell counterparts?

The iOS build toolchain is Apple-only. A Windows runner cannot
build/test iOS targets. CI runs iOS jobs on macOS-latest GitHub-hosted
runners (`docs/CI-PIPELINE.md` describes the matrix); Windows runners
only execute the static-validation grep checks listed in
`docs/ios/xcodeproj-structure.md`.