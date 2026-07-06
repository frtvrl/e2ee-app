# verify-xcodeproj.ps1 — PR-25 static-validation gate (PowerShell).
#
# Windows / non-bash counterpart of verify-xcodeproj.sh. Runs the same
# 8 checks documented in docs/ios/xcodeproj-structure.md. Used by CI on
# Windows runners where bash is unavailable.
#
# Usage:
#   powershell -ExecutionPolicy Bypass -File scripts/ios/verify-xcodeproj.ps1 [-ProjectRoot <path>]

[CmdletBinding()]
param(
    [string]$ProjectRoot
)

if (-not $ProjectRoot) {
    $scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
    $ProjectRoot = (Resolve-Path "$scriptDir/../..").Path
}

$ErrorActionPreference = 'Stop'

$Pbxproj = Join-Path $ProjectRoot 'mobile/ios/Runner.xcodeproj/project.pbxproj'
$AppDelegate = Join-Path $ProjectRoot 'mobile/ios/Runner/AppDelegate.swift'
$NeEntitlements = Join-Path $ProjectRoot 'mobile/ios/NetworkExtension/OpenE2eeTunnelProvider.entitlements'
$NeInfoPlist = Join-Path $ProjectRoot 'mobile/ios/NetworkExtension/Info.plist'

if (-not (Test-Path $Pbxproj)) {
    Write-Host "FATAL: $Pbxproj not found"
    exit 1
}

$failCount = 0
$passCount = 0

function Test-GrepAtLeast {
    param(
        [string]$Name,
        [string]$Pattern,
        [string]$Path,
        [int]$Expected
    )
    $count = (Select-String -Path $Path -Pattern $Pattern -AllMatches | Measure-Object).Count
    if ($count -ge $Expected) {
        Write-Host "  PASS  $Name (found $count >= $Expected)"
        $script:passCount++
    } else {
        Write-Host "  FAIL  $Name (found $count < $Expected)"
        $script:failCount++
    }
}

function Test-GrepExactly {
    param(
        [string]$Name,
        [string]$Pattern,
        [string]$Path,
        [int]$Expected
    )
    $count = (Select-String -Path $Path -Pattern $Pattern -AllMatches | Measure-Object).Count
    if ($count -eq $Expected) {
        Write-Host "  PASS  $Name (found $count == $Expected)"
        $script:passCount++
    } else {
        Write-Host "  FAIL  $Name (found $count != $Expected)"
        $script:failCount++
    }
}

Write-Host '=== PR-25 iOS Xcode project static-validation ==='
Write-Host "  PROJECT_ROOT: $ProjectRoot"
Write-Host ''

Write-Host '[1/8] NE target .appex exists in pbxproj'
Test-GrepAtLeast -Name '1. NE target exists' -Pattern 'OpenE2eeTunnelProvider\.appex' -Path $Pbxproj -Expected 4

Write-Host '[2/8] NE source in Compile Sources'
# The PBXBuildFile entry + the Sources-phase listing both reference
# "in Sources" — both are required. Expect >= 2.
Test-GrepAtLeast -Name '2. NE source in Compile Sources' -Pattern 'OpenE2eeTunnelProvider\.swift in Sources' -Path $Pbxproj -Expected 2

Write-Host '[3/8] Embed App Extensions build phase on Runner'
Test-GrepAtLeast -Name '3. Embed App Extensions build phase' -Pattern 'Embed App Extensions' -Path $Pbxproj -Expected 3

Write-Host '[4/8] NE entitlements referenced in pbxproj'
Test-GrepExactly -Name '4. NE entitlements referenced' -Pattern 'CODE_SIGN_ENTITLEMENTS = NetworkExtension/OpenE2eeTunnelProvider\.entitlements' -Path $Pbxproj -Expected 2

Write-Host '[5/8] Tunnel bundle id in pbxproj'
Test-GrepExactly -Name '5. Tunnel bundle id' -Pattern 'PRODUCT_BUNDLE_IDENTIFIER = com\.opene2ee\.opene2ee\.tunnel' -Path $Pbxproj -Expected 2

Write-Host '[6/8] AppDelegate.tunnelBundleId matches NE bundle id'
Test-GrepExactly -Name '6. tunnelBundleId in AppDelegate' -Pattern 'tunnelBundleId = "com\.opene2ee\.opene2ee\.tunnel"' -Path $AppDelegate -Expected 1

Write-Host '[7/8] NE entitlements contain packet-tunnel-provider + allow-vpn'
Test-GrepAtLeast -Name '7a. packet-tunnel-provider in NE entitlements' -Pattern 'packet-tunnel-provider' -Path $NeEntitlements -Expected 1
Test-GrepAtLeast -Name '7b. allow-vpn in NE entitlements' -Pattern 'allow-vpn' -Path $NeEntitlements -Expected 1

Write-Host '[8/8] NE Info.plist has packet-tunnel extension point'
Test-GrepExactly -Name '8. NE Info.plist extension point' -Pattern '<string>com\.apple\.networkextension\.packet-tunnel</string>' -Path $NeInfoPlist -Expected 1

Write-Host ''
Write-Host "=== Results: $passCount passed, $failCount failed ==="
if ($failCount -gt 0) {
    Write-Host 'STATIC VALIDATION FAILED — see docs/ios/xcodeproj-structure.md for the expected layout.'
    exit 1
}
Write-Host 'All static-validation checks passed.'
exit 0