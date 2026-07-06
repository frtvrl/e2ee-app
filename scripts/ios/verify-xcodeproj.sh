#!/usr/bin/env bash
#
# verify-xcodeproj.sh — PR-25 static-validation gate.
#
# Runs the 8 grep checks documented in docs/ios/xcodeproj-structure.md
# and exits non-zero if any check fails. Used by CI on non-macOS
# runners where Xcode is unavailable; the macOS job runs the same
# checks via `xcodebuild -showBuildSettings` plus a `xcodebuild build`
# to validate the actual build, not just the structure.
#
# Usage:
#   bash scripts/ios/verify-xcodeproj.sh [PROJECT_ROOT]
#
# Default PROJECT_ROOT is the repo root (one level up from this script).

set -euo pipefail

PROJECT_ROOT="${1:-$(cd "$(dirname "$0")/../.." && pwd)}"
PBXPROJ="$PROJECT_ROOT/mobile/ios/Runner.xcodeproj/project.pbxproj"
APPDELEGATE="$PROJECT_ROOT/mobile/ios/Runner/AppDelegate.swift"
NE_ENT="$PROJECT_ROOT/mobile/ios/NetworkExtension/OpenE2eeTunnelProvider.entitlements"
NE_INFO_PLIST="$PROJECT_ROOT/mobile/ios/NetworkExtension/Info.plist"

if [[ ! -f "$PBXPROJ" ]]; then
    echo "FATAL: $PBXPROJ not found"
    exit 1
fi

fail_count=0
pass_count=0

check() {
    local name="$1"
    local pattern="$2"
    local file="$3"
    local expected="$4"
    local actual
    actual=$(grep -cE "$pattern" "$file" || true)
    if [[ "$actual" -ge "$expected" ]]; then
        echo "  PASS  $name (found $actual >= $expected)"
        pass_count=$((pass_count + 1))
    else
        echo "  FAIL  $name (found $actual < $expected)"
        fail_count=$((fail_count + 1))
    fi
}

check_one() {
    local name="$1"
    local pattern="$2"
    local file="$3"
    local expected="$4"
    local actual
    actual=$(grep -cE "$pattern" "$file" || true)
    if [[ "$actual" -eq "$expected" ]]; then
        echo "  PASS  $name (found $actual == $expected)"
        pass_count=$((pass_count + 1))
    else
        echo "  FAIL  $name (found $actual != $expected)"
        fail_count=$((fail_count + 1))
    fi
}

echo "=== PR-25 iOS Xcode project static-validation ==="
echo "  PROJECT_ROOT: $PROJECT_ROOT"
echo

echo "[1/8] NE target .appex exists in pbxproj"
check "1. NE target exists" 'OpenE2eeTunnelProvider\.appex' "$PBXPROJ" 4

echo "[2/8] NE source in Compile Sources"
# The PBXBuildFile entry + the Sources-phase listing both reference
# "in Sources" — both are required. Expect >= 2.
check "2. NE source in Compile Sources" 'OpenE2eeTunnelProvider\.swift in Sources' "$PBXPROJ" 2

echo "[3/8] Embed App Extensions build phase on Runner"
check "3. Embed App Extensions build phase" 'Embed App Extensions' "$PBXPROJ" 3

echo "[4/8] NE entitlements referenced in pbxproj"
check_one "4. NE entitlements referenced" 'CODE_SIGN_ENTITLEMENTS = NetworkExtension/OpenE2eeTunnelProvider\.entitlements' "$PBXPROJ" 2

echo "[5/8] Tunnel bundle id in pbxproj"
check_one "5. Tunnel bundle id" 'PRODUCT_BUNDLE_IDENTIFIER = com\.opene2ee\.opene2ee\.tunnel' "$PBXPROJ" 2

echo "[6/8] AppDelegate.tunnelBundleId matches NE bundle id"
check_one "6. tunnelBundleId in AppDelegate" 'tunnelBundleId = "com\.opene2ee\.opene2ee\.tunnel"' "$APPDELEGATE" 1

echo "[7/8] NE entitlements contain packet-tunnel-provider + allow-vpn"
check "7a. packet-tunnel-provider in NE entitlements" 'packet-tunnel-provider' "$NE_ENT" 1
check "7b. allow-vpn in NE entitlements" 'allow-vpn' "$NE_ENT" 1

echo "[8/8] NE Info.plist has packet-tunnel extension point"
# The string appears in comments AND in the actual plist string value.
# Match only plist string lines (inside <string>...</string>) to avoid
# comment noise. Expect exactly 1.
check_one "8. NE Info.plist extension point" '<string>com\.apple\.networkextension\.packet-tunnel</string>' "$NE_INFO_PLIST" 1

echo
echo "=== Results: $pass_count passed, $fail_count failed ==="
if [[ "$fail_count" -gt 0 ]]; then
    echo "STATIC VALIDATION FAILED — see docs/ios/xcodeproj-structure.md for the expected layout."
    exit 1
fi
echo "All static-validation checks passed."