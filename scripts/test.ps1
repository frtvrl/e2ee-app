#!/usr/bin/env pwsh
# scripts/test.ps1 - Run all test suites (Go + Flutter)
# ADR-0008 S2.4 - PowerShell native fallback (mirrors scripts/test.sh)
#
# Usage:
#   pwsh -File scripts/test.ps1
#
# First failing suite throws; the script never proceeds to a second suite so
# you don't get a wall of cascading errors after the real failure.
$ErrorActionPreference = 'Stop'

# --- Resolve repo root from this script's location -----------------------------
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$RepoRoot  = (Resolve-Path (Join-Path $ScriptDir '..')).Path

Set-Location $RepoRoot

Write-Host '==> OpenE2EE test suite'

# --- Backend: go test --------------------------------------------------------
Write-Host ''
Write-Host '==> Backend: go test ./...'

if (Test-Path (Join-Path $RepoRoot 'backend')) {
    $goExe = (Get-Command go -ErrorAction SilentlyContinue).Source
    if ($goExe) {
        Push-Location (Join-Path $RepoRoot 'backend')
        try {
            & $goExe test ./...
            if ($LASTEXITCODE -ne 0) {
                throw 'go test ./... failed'
            }
        } finally {
            Pop-Location
        }
    } else {
        Write-Host '    [SKIP] go not on PATH'
    }
} else {
    Write-Host '    [SKIP] backend/ directory not present (Go service not scaffolded yet)'
}

# --- Mobile: flutter test ----------------------------------------------------
Write-Host ''
Write-Host '==> Mobile: flutter test'

if (Test-Path (Join-Path $RepoRoot 'mobile')) {
    $flutterExe = (Get-Command flutter -ErrorAction SilentlyContinue).Source
    if ($flutterExe) {
        Push-Location (Join-Path $RepoRoot 'mobile')
        try {
            & $flutterExe test
            if ($LASTEXITCODE -ne 0) {
                throw 'flutter test failed'
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
Write-Host '==> Test run complete.'
