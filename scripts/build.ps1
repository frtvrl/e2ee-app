#!/usr/bin/env pwsh
# scripts/build.ps1 - Production build (Go server + Flutter web)
# ADR-0008 S2.4 - PowerShell native fallback (mirrors scripts/build.sh)
#
# Usage:
#   pwsh -File scripts/build.ps1
$ErrorActionPreference = 'Stop'

# --- Resolve repo root from this script's location -----------------------------
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$RepoRoot  = (Resolve-Path (Join-Path $ScriptDir '..')).Path

Set-Location $RepoRoot

Write-Host '==> OpenE2EE production build'

# --- Backend: go build -------------------------------------------------------
Write-Host ''
Write-Host '==> Backend: go build -o dist/server ./cmd/server'

if ((Test-Path (Join-Path $RepoRoot 'backend')) -and (Test-Path (Join-Path $RepoRoot 'backend/cmd/server'))) {
    $goExe = (Get-Command go -ErrorAction SilentlyContinue).Source
    if ($goExe) {
        $env:CGO_ENABLED = '0'  # -> statically linked, runs on every Linux distro.
        Push-Location (Join-Path $RepoRoot 'backend')
        try {
            New-Item -ItemType Directory -Path 'dist' -Force | Out-Null
            & $goExe build -o dist/server ./cmd/server
            if ($LASTEXITCODE -ne 0) {
                throw 'go build ./cmd/server failed'
            }
            Write-Host '    -> backend/dist/server'
        } finally {
            Pop-Location
        }
    } else {
        Write-Host '    [SKIP] go not on PATH'
    }
} else {
    Write-Host '    [SKIP] backend/cmd/server not present (Go service not scaffolded yet)'
}

# --- Mobile: flutter build web -----------------------------------------------
Write-Host ''
Write-Host '==> Mobile: flutter build web'

if (Test-Path (Join-Path $RepoRoot 'mobile')) {
    $flutterExe = (Get-Command flutter -ErrorAction SilentlyContinue).Source
    if ($flutterExe) {
        Push-Location (Join-Path $RepoRoot 'mobile')
        try {
            & $flutterExe build web
            if ($LASTEXITCODE -ne 0) {
                throw 'flutter build web failed'
            }
            Write-Host '    -> mobile/build/web'
        } finally {
            Pop-Location
        }
    } else {
        Write-Host '    [SKIP] flutter not on PATH'
    }
} else {
    Write-Host '    [SKIP] mobile/ directory not present'
}

Write-Host ''
Write-Host '==> Build complete.'
