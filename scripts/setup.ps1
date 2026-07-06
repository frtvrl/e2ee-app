#!/usr/bin/env pwsh
# scripts/setup.ps1 - OpenE2EE development environment setup
# ADR-0008 S2.4 - PowerShell native fallback (mirrors scripts/setup.sh)
#
# Usage:
#   pwsh -File scripts/setup.ps1
#
# Exits non-zero on the first failed toolchain check.
$ErrorActionPreference = 'Stop'

# --- Resolve repo root from this script's location -----------------------------
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$RepoRoot  = (Resolve-Path (Join-Path $ScriptDir '..')).Path

Set-Location $RepoRoot

Write-Host '==> OpenE2EE setup'
Write-Host "    repo root: $RepoRoot"

# Helper: print a [OK]/[MISSING] line for a given executable.
function Test-Toolchain {
    param(
        [Parameter(Mandatory)] [string] $Name,
        [Parameter(Mandatory)] [string] $ExpectedHint,
        [scriptblock]               $VersionExtractor
    )

    $exe = Get-Command $Name -ErrorAction SilentlyContinue
    if ($null -eq $exe) {
        Write-Host "    [MISSING] $Name ($ExpectedHint)"
        return
    }

    $version = if ($VersionExtractor) {
        try { & $VersionExtractor } catch { 'unknown' }
    } else {
        'unknown'
    }
    if ([string]::IsNullOrWhiteSpace($version)) { $version = 'unknown' }

    Write-Host "    [OK] $Name $version"
}

# --- Toolchain version checks -------------------------------------------------
Write-Host ''
Write-Host '==> Checking toolchain versions'

# Go (>= 1.26 per ADR-0008 S2.6) - invokes via the resolved full path of go.exe.
Test-Toolchain -Name 'go' -ExpectedHint 'expected Go 1.26+' -VersionExtractor {
    $goExe = (Get-Command go -ErrorAction SilentlyContinue).Source
    if (-not $goExe) { return '' }
    (& $goExe version) -replace '^go version ',''
}

# Flutter (stable channel)
Test-Toolchain -Name 'flutter' -ExpectedHint 'stable channel' -VersionExtractor {
    $flutterExe = (Get-Command flutter -ErrorAction SilentlyContinue).Source
    if (-not $flutterExe) { return '' }
    # flutter --version first line: "Flutter 3.x.y ... channel ..."
    $line = (& $flutterExe --version 2>$null | Select-Object -First 1)
    if ($line -match 'Flutter\s+([0-9.]+)') { $Matches[1] } else { '' }
}

# protoc (Protocol Buffers compiler)
Test-Toolchain -Name 'protoc' -ExpectedHint 'Protocol Buffers compiler' -VersionExtractor {
    $protocExe = (Get-Command protoc -ErrorAction SilentlyContinue).Source
    if (-not $protocExe) { return '' }
    (& $protocExe --version 2>&1) -replace '^libprotoc ',''
}

# Docker (required for dev.ps1)
Test-Toolchain -Name 'docker' -ExpectedHint "required for 'make dev'" -VersionExtractor {
    $dockerExe = (Get-Command docker -ErrorAction SilentlyContinue).Source
    if (-not $dockerExe) { return '' }
    $line = (& $dockerExe --version 2>&1 | Select-Object -First 1)
    if ($line -match 'Docker\s+version\s+([0-9.]+)') { $Matches[1] } else { '' }
}

# --- Dependency install (placeholder) ----------------------------------------
Write-Host ''
Write-Host '==> Installing backend dependencies (placeholder)'
Write-Host '    -> cd backend && go mod download    (skipped - backend/ not present yet)'

Write-Host ''
Write-Host '==> Installing mobile dependencies (placeholder)'

if (Test-Path (Join-Path $RepoRoot 'mobile/pubspec.yaml')) {
    $flutterExe = (Get-Command flutter -ErrorAction SilentlyContinue).Source
    if ($flutterExe) {
        Push-Location (Join-Path $RepoRoot 'mobile')
        try {
            & $flutterExe pub get
            if ($LASTEXITCODE -ne 0) {
                throw 'flutter pub get failed'
            }
        } finally {
            Pop-Location
        }
    } else {
        Write-Host '    (skipped - flutter not on PATH)'
    }
} else {
    Write-Host '    (skipped - mobile/pubspec.yaml not present)'
}

Write-Host ''
Write-Host '==> Setup complete.'
Write-Host "    Next: make dev    # start backend (docker compose) + mobile (flutter run)"
