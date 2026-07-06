#!/usr/bin/env pwsh
# scripts/build-web.ps1 - Flutter web build wrapper (PR-27)
# ADR-0008 S2.4 - PowerShell native fallback (mirrors scripts/build-web.sh)
#
# Why this script exists
# ----------------------
# `flutter build web` defaults to `lib/main.dart`. OpenE2EE deliberately
# keeps a SEPARATE entry point for the web dashboard at `lib/web/main.dart`
# (see mobile/lib/web/main.dart for the rationale, HANDOFF S4.2 PR-11).
# Until the mobile-only `lib/main.dart` exists (future PR), running
# `flutter build web` without `--target` produces a confusing
# "Target of URI doesn't exist" error against `lib/main.dart`.
#
# This wrapper hard-codes the correct invocation:
#     flutter build web --target=lib/web/main.dart
# (run from inside the `mobile/` Flutter package)
#
# Usage:
#   pwsh -File scripts/build-web.ps1
#   pwsh -File scripts/build-web.ps1 -Release
#   pwsh -File scripts/build-web.ps1 -Release -Renderer canvaskit
#   pwsh -File scripts/build-web.ps1 -Check           # CI: verify entry exists, don't build
#
# Equivalent to: make build-web  (cross-platform Make target)

[CmdletBinding()]
param(
    # Pass --release to flutter. Default: omitted (flutter default = release).
    [switch]$Release,

    # --web-renderer <canvaskit|html|wasm>. Default: omitted (flutter default).
    [string]$Renderer,

    # CI preflight: verify target entry file exists + flutter on PATH, but skip the build.
    [switch]$Check
)

$ErrorActionPreference = 'Stop'

# --- Resolve repo root from this script's location -------------------------
$ScriptDir  = Split-Path -Parent $MyInvocation.MyCommand.Path
$RepoRoot   = (Resolve-Path (Join-Path $ScriptDir '..')).Path
$MobileDir  = Join-Path $RepoRoot 'mobile'
$TargetDart = Join-Path $MobileDir 'lib/web/main.dart'

# --- Friendly pre-flight: ensure flutter project + web entry exist ----------
if (-not (Test-Path $MobileDir)) {
    Write-Host ''
    Write-Error "Flutter project not found at '$MobileDir'. Did you clone the full repo?"
    exit 1
}

if (-not (Test-Path $TargetDart)) {
    Write-Host ''
    Write-Host 'ERROR: Flutter web entry not found at expected path:' -ForegroundColor Red
    Write-Host "    $TargetDart"
    Write-Host ''
    Write-Host 'Why this happens:' -ForegroundColor Yellow
    Write-Host '  OpenE2EE uses a non-standard Flutter web entry point at'
    Write-Host '  mobile/lib/web/main.dart (PR-11 web dashboard). The default'
    Write-Host '  `flutter build web` targets mobile/lib/main.dart (the future'
    Write-Host '  mobile entry point, not yet in this checkout).'
    Write-Host ''
    Write-Host 'How to fix (run from repo root):' -ForegroundColor Yellow
    Write-Host '  1. Restore the canonical web entry:'
    Write-Host '       git checkout -- mobile/lib/web/main.dart'
    Write-Host '  2. If the entry is missing because the Flutter package is uninitialised:'
    Write-Host '       cd mobile'
    Write-Host '       flutter create .            # creates web/ + lib/'
    Write-Host '       flutter pub get              # restore deps'
    Write-Host '       git checkout -- mobile/lib/web/main.dart   # restore web entry'
    Write-Host '  3. If you intentionally moved the web entry, update the'
    Write-Host '     --target=<path> flag in this wrapper (see line ~92).'
    Write-Host ''
    exit 1
}

# --- Resolve flutter --------------------------------------------------------
$flutterExe = (Get-Command flutter -ErrorAction SilentlyContinue).Source
if (-not $flutterExe) {
    Write-Error "flutter not found on PATH. Install Flutter SDK: https://docs.flutter.dev/get-started/install"
    exit 1
}

# --- Compose flutter args ---------------------------------------------------
$flutterArgs = @('build', 'web', '--target=lib/web/main.dart')
if ($Release)  { $flutterArgs += '--release' }
if ($Renderer) { $flutterArgs += @('--web-renderer', $Renderer) }

# --- Header: Sprint 4 PR-27 context ----------------------------------------
if ($Check) {
    Write-Host '==> OpenE2EE Flutter web build (PR-27 wrapper) - -Check (CI preflight)'
    Write-Host '    entry:   mobile/lib/web/main.dart (non-standard; see CONTRIBUTING.md)'
    Write-Host '    OK:      target file exists, flutter on PATH.'
    Write-Host ''
    Write-Host '==> Check passed. Skipping build (omit -Check to actually build).'
    exit 0
}

Write-Host '==> OpenE2EE Flutter web build (PR-27 wrapper)'
Write-Host '    entry:   mobile/lib/web/main.dart (non-standard; see CONTRIBUTING.md)'
Write-Host "    args:     flutter $($flutterArgs -join ' ')"
Write-Host '    output:  mobile/build/web'
Write-Host ''

# --- Run --------------------------------------------------------------------
Push-Location $MobileDir
try {
    & $flutterExe @flutterArgs
    if ($LASTEXITCODE -ne 0) {
        throw "flutter build web failed (exit $LASTEXITCODE)"
    }
} finally {
    Pop-Location
}

Write-Host ''
Write-Host '==> Done. Output: mobile/build/web'
