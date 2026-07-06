#!/usr/bin/env pwsh
# scripts/dev.ps1 - Start dev environment (docker compose + flutter run)
# ADR-0008 S2.4 - PowerShell native fallback (mirrors scripts/dev.sh)
#
# Usage:
#   pwsh -File scripts/dev.ps1
$ErrorActionPreference = 'Stop'

# --- Resolve repo root from this script's location -----------------------------
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$RepoRoot  = (Resolve-Path (Join-Path $ScriptDir '..')).Path

Set-Location $RepoRoot

Write-Host '==> OpenE2EE dev environment'

# --- Backend via docker compose ----------------------------------------------
Write-Host ''
Write-Host '==> Starting backend stack (docker compose up)'

$dockerExe = (Get-Command docker -ErrorAction SilentlyContinue).Source
$composeFiles = @('docker-compose.yml','docker-compose.yaml','compose.yml','compose.yaml')
$hasComposeFile = $false
foreach ($f in $composeFiles) {
    if (Test-Path (Join-Path $RepoRoot $f)) { $hasComposeFile = $true; break }
}

if ($dockerExe -and $hasComposeFile) {
    Push-Location $RepoRoot
    try {
        & $dockerExe compose up -d
        if ($LASTEXITCODE -ne 0) {
            throw 'docker compose up failed'
        }
        Write-Host '    docker compose stack started (detached)'
    } finally {
        Pop-Location
    }
} elseif (-not $dockerExe) {
    Write-Host '    [SKIP] docker not on PATH - install Docker to use the dev stack'
} else {
    Write-Host '    [SKIP] no docker-compose.yml/compose.yaml found at repo root'
}

# --- Mobile via flutter run --------------------------------------------------
Write-Host ''
Write-Host '==> Launching mobile (flutter run)'

if (Test-Path (Join-Path $RepoRoot 'mobile')) {
    $flutterExe = (Get-Command flutter -ErrorAction SilentlyContinue).Source
    if ($flutterExe) {
        Push-Location (Join-Path $RepoRoot 'mobile')
        try {
            # `-d` is intentionally omitted: flutter will pick a default device.
            # Use `flutter devices` to list available targets.
            & $flutterExe run
            if ($LASTEXITCODE -ne 0) {
                throw 'flutter run failed'
            }
        } finally {
            Pop-Location
        }
    } else {
        Write-Host '    [SKIP] flutter not on PATH'
    }
} else {
    Write-Host '    [SKIP] mobile/ directory not present'
}
