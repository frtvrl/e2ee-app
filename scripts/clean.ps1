#!/usr/bin/env pwsh
# scripts/clean.ps1 - Remove untracked + ignored files from the working tree
# ADR-0008 S2.4 - PowerShell native fallback (mirrors scripts/clean.sh)
#
# `git clean -fdx` removes:
#   - untracked files (-f = force, -d = directories)
#   - files matched by .gitignore (-x)
#
# It will NOT touch:
#   - the .git/ directory            (always protected)
#   - the .gitignore file itself     (tracked, not ignored)
#   - any other tracked file         (only untracked/ignored are removed)
#
# Usage:
#   pwsh -File scripts/clean.ps1           # interactive prompt
#   pwsh -File scripts/clean.ps1 -Yes      # non-interactive (skip prompt)
[CmdletBinding()]
param(
    [switch]$Yes
)

$ErrorActionPreference = 'Stop'

# --- Resolve repo root from this script's location -----------------------------
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$RepoRoot  = (Resolve-Path (Join-Path $ScriptDir '..')).Path

Set-Location $RepoRoot

Write-Host '==> OpenE2EE clean'
Write-Host "    repo root: $RepoRoot"
Write-Host ''
Write-Host '    This will run: git clean -fdx'
Write-Host '      - untracked files: REMOVED'
Write-Host '      - .gitignore''d files (e.g. build/, .dart_tool/, dist/): REMOVED'
Write-Host '      - .git/ directory: PRESERVED'
Write-Host '      - .gitignore file: PRESERVED (tracked)'
Write-Host '      - other tracked files: PRESERVED'
Write-Host ''

# Dry-run first so the user can see what would happen
Write-Host '==> Dry-run (git clean -fdx -n):'
& git.exe clean -fdx -n
if ($LASTEXITCODE -ne 0) {
    throw 'git clean -fdx -n failed'
}
Write-Host ''

# Confirm unless -Yes is supplied.
if (-not $Yes) {
    $reply = Read-Host 'Proceed with actual clean? [y/N]'
    if ($reply -notmatch '^[yY]([eE][sS])?$') {
        Write-Host 'Aborted.'
        exit 0
    }
}

Write-Host ''
Write-Host '==> Running: git clean -fdx'
& git.exe clean -fdx
if ($LASTEXITCODE -ne 0) {
    throw 'git clean -fdx failed'
}
Write-Host ''
Write-Host '==> Clean complete.'
