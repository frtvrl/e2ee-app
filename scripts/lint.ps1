#!/usr/bin/env pwsh
# scripts/lint.ps1 - Run linters (go vet + flutter analyze)
# ADR-0008 S2.4 - PowerShell native fallback (mirrors scripts/lint.sh)
#
# Usage:
#   pwsh -File scripts/lint.ps1
$ErrorActionPreference = 'Stop'

# --- Resolve repo root from this script's location -----------------------------
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$RepoRoot  = (Resolve-Path (Join-Path $ScriptDir '..')).Path

Set-Location $RepoRoot

Write-Host '==> OpenE2EE lint'

# --- Backend: go vet ---------------------------------------------------------
Write-Host ''
Write-Host '==> Backend: go vet ./...'

if (Test-Path (Join-Path $RepoRoot 'backend')) {
    $goExe = (Get-Command go -ErrorAction SilentlyContinue).Source
    if ($goExe) {
        Push-Location (Join-Path $RepoRoot 'backend')
        try {
            & $goExe vet ./...
            if ($LASTEXITCODE -ne 0) {
                throw 'go vet ./... failed'
            }
        } finally {
            Pop-Location
        }
    } else {
        Write-Host '    [SKIP] go not on PATH'
    }
} else {
    Write-Host '    [SKIP] backend/ directory not present'
}

# --- Mobile: flutter analyze -------------------------------------------------
Write-Host ''
Write-Host '==> Mobile: flutter analyze'

if (Test-Path (Join-Path $RepoRoot 'mobile')) {
    $flutterExe = (Get-Command flutter -ErrorAction SilentlyContinue).Source
    if ($flutterExe) {
        Push-Location (Join-Path $RepoRoot 'mobile')
        try {
            & $flutterExe analyze
            if ($LASTEXITCODE -ne 0) {
                throw 'flutter analyze failed'
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

Write-Host ''
Write-Host '==> Lint complete.'
