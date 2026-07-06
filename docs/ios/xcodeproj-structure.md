# OpenE2EE iOS Xcode Project Structure (PR-25)

This document is the **single source of truth** for the layout of
`mobile/ios/Runner.xcodeproj/project.pbxproj`. The `.pbxproj` file is
machine-readable but verbose; this document explains the **why** for
each section so reviewers and future maintainers can verify the structure
without reading ~456 lines of plist.

## Project layout

```
mobile/ios/
├── Runner.xcodeproj/
│   ├── project.pbxproj                    # THIS DOCUMENT describes this file
│   └── xcshareddata/
│       └── xcschemes/
│           ├── Runner.xcscheme            # Flutter-app scheme
│           └── OpenE2eeTunnelProvider.xcscheme   # NE-target scheme
├── Runner/
│   ├── AppDelegate.swift                  # MethodChannel bridge, NETunnelProviderManager
│   ├── Info.plist                         # Runner app Info.plist
│   ├── Runner.entitlements                # Runner target entitlements
│   └── Assets.xcassets/                   # (added in Sprint 4 follow-up)
├── NetworkExtension/
│   ├── OpenE2eeTunnelProvider.swift       # NEPacketTunnelProvider implementation
│   ├── OpenE2eeTunnelProvider.entitlements # NE target entitlements (packet-tunnel + allow-vpn)
│   └── Info.plist                         # NE Info.plist with NSExtension dict
├── RunnerTests/
│   └── OpenE2eeTunnelProviderTests.swift  # 9 XCTest cases (PR-22b §6 adversarial probes)
└── .gitignore
```

## Targets

| Target                     | Product                            | Bundle ID                         | Product type                              |
| -------------------------- | ---------------------------------- | --------------------------------- | ----------------------------------------- |
| `Runner`                   | `Runner.app`                       | `com.opene2ee.opene2ee`           | `com.apple.product-type.application`      |
| `OpenE2eeTunnelProvider`   | `OpenE2eeTunnelProvider.appex`     | `com.opene2ee.opene2ee.tunnel`    | `com.apple.product-type.app-extension`    |

The `Runner.app` **embeds** the `OpenE2eeTunnelProvider.appex` via an
`Embed App Extensions` Copy Files build phase (subfolder spec `13` =
PlugIns folder). The NE target is a build dependency of the Runner target
(`PBXTargetDependency`), so `flutter build ios` automatically compiles
both.

## Build settings (per-target)

| Setting                       | Runner                                      | OpenE2eeTunnelProvider                                   |
| ----------------------------- | ------------------------------------------- | -------------------------------------------------------- |
| `PRODUCT_BUNDLE_IDENTIFIER`   | `com.opene2ee.opene2ee`                     | `com.opene2ee.opene2ee.tunnel`                           |
| `CODE_SIGN_ENTITLEMENTS`      | `Runner/Runner.entitlements`                | `NetworkExtension/OpenE2eeTunnelProvider.entitlements`  |
| `INFOPLIST_FILE`              | `Runner/Info.plist`                         | `NetworkExtension/Info.plist`                            |
| `APPLICATION_EXTENSION_API_ONLY` | (not set)                                | `YES` (required for app extensions)                      |
| `SKIP_INSTALL`                | (not set)                                   | `YES` (extensions are not standalone-installable)        |
| `IPHONEOS_DEPLOYMENT_TARGET`  | `14.0`                                      | `14.0`                                                   |
| `SWIFT_VERSION`               | `5.0`                                       | `5.0`                                                    |

## Entitlements

`NetworkExtension/OpenE2eeTunnelProvider.entitlements`:

| Key                                                         | Value                       | Purpose                                                                   |
| ----------------------------------------------------------- | --------------------------- | ------------------------------------------------------------------------- |
| `com.apple.developer.networking.networkextension`           | `[packet-tunnel-provider]`  | Right to subclass `NEPacketTunnelProvider` (Apple dev portal approval).   |
| `com.apple.developer.networking.vpn.api`                   | `[allow-vpn]`               | Right to invoke `startVPNTunnel` (Apple dev portal approval).             |
| `com.apple.security.application-groups`                     | `[group.com.opene2ee.opene2ee]` | Shared App Group with Runner (Keychain handoff in Sprint 4+).       |

`Runner/Runner.entitlements` mirrors the same three keys. Apple convention
allows either target to declare the NE entitlements, but the canonical
location is the NE target's own file. Keeping the Runner-target copy is a
safety net so a dev who only opens the Runner scheme still sees the
required keys.

## NetworkExtension Info.plist

`NSExtension`:

| Key                              | Value                                                          |
| -------------------------------- | -------------------------------------------------------------- |
| `NSExtensionPointIdentifier`     | `com.apple.networkextension.packet-tunnel`                    |
| `NSExtensionPrincipalClass`      | `$(PRODUCT_MODULE_NAME).OpenE2eeTunnelProvider`                |

The `$(PRODUCT_MODULE_NAME)` token is replaced with the NE target's
module name (`OpenE2eeTunnelProvider`) at build time. iOS instantiates
that class on `startVPNTunnel()`.

## UUIDs

All UUIDs in `project.pbxproj` follow the pattern `A1XXXXXXXXXXXXXXXXXXX`
(24 hex chars). They are deterministic in the sense that running the
`xcodegen` rewriter (or hand-editing + `git commit`) yields identical
strings for the same semantic entity (target name, file path, build
phase label). This keeps the diff focused on real changes, not UUID
churn.

The pbxproj was committed as a **hand-written minimal scaffold**
(Xcode 14+ objectVersion 56) because the environment that produced
PR-25 has no Xcode installed. A macOS dev should verify the file
opens cleanly with `xed Runner.xcodeproj`; if Xcode re-rolls any
UUIDs on first save, the change is benign (UUID churn is normal) but
the file structure must match the table above.

## Static-validation checklist

For environments where Xcode cannot be invoked, the following `grep`
checks confirm the structure is correct:

```bash
# 1. NE target exists
grep -c 'OpenE2eeTunnelProvider.appex' mobile/ios/Runner.xcodeproj/project.pbxproj
# expect: >= 4 (file ref + product ref + build file + group children)

# 2. NE source is in Compile Sources
grep -c 'OpenE2eeTunnelProvider.swift in Sources' mobile/ios/Runner.xcodeproj/project.pbxproj
# expect: 1

# 3. Embed App Extensions build phase on Runner
grep -c 'Embed App Extensions' mobile/ios/Runner.xcodeproj/project.pbxproj
# expect: >= 3 (phase def + phase name + build file name)

# 4. NE entitlements referenced
grep -c 'CODE_SIGN_ENTITLEMENTS = NetworkExtension/OpenE2eeTunnelProvider.entitlements' mobile/ios/Runner.xcodeproj/project.pbxproj
# expect: 2 (Debug + Release configurations)

# 5. Tunnel bundle id
grep -c 'PRODUCT_BUNDLE_IDENTIFIER = com.opene2ee.opene2ee.tunnel' mobile/ios/Runner.xcodeproj/project.pbxproj
# expect: 2

# 6. AppDelegate tunnelBundleId matches NE bundle id
grep 'tunnelBundleId = ' mobile/ios/Runner/AppDelegate.swift
# expect: tunnelBundleId = "com.opene2ee.opene2ee.tunnel"

# 7. NE entitlements contain required keys
grep -E 'packet-tunnel-provider|allow-vpn' mobile/ios/NetworkExtension/OpenE2eeTunnelProvider.entitlements
# expect: 2 lines

# 8. NE Info.plist has the right extension point
grep 'com.apple.networkextension.packet-tunnel' mobile/ios/NetworkExtension/Info.plist
# expect: 1
```

If all eight checks pass, the pbxproj is structurally correct. The
final gate (signing + provisioning profile assignment) requires a
macOS dev machine with Xcode and an Apple Developer account.

## Why not regenerate via `flutter create`?

Sprint 3 chose not to commit `project.pbxproj` because every dev's
`flutter create` re-rolls UUIDs, creating churn. PR-25's solution is to
**commit a deterministic, hand-written scaffold**. Future regeneration
should use a generator (e.g. XcodeGen with a `project.yml` spec) — that's
tracked as Sprint 4+ follow-up.

## See also

- `docs/ADR-0003-vpn-layer.md` — VPN architecture
- `docs/ADR-0006-anonimlik.md` — Privacy invariants
- `docs/SPRINT-3-SCOPE.md` §7 — PR-22b iOS VPN
- `docs/SPRINT-4-SCOPE.md` (when published) — PR-25 follow-ups