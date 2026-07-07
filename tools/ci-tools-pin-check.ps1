# tools/ci-tools-pin-check.ps1 — PowerShell twin of ci-tools-pin-check.sh.
# Mirrors the bash validator for Windows CI / ADR-0008 cross-platform.
#
# Sprint 7 / STRIDE-8-03 — same policy as the .sh twin.
# Scans .ps1/.psm1/.yml/.yaml files under scripts/, infra/scripts/, tools/,
# and .github/workflows/ for unpinned invocations of jq, curl, openssl,
# base64, sha256sum, apt-get, brew, pip, pip3, npm.
#
# Exits 0 on clean, 1 on violation, 2 on internal error.
#
# Usage:
#   pwsh tools/ci-tools-pin-check.ps1
#   pwsh tools/ci-tools-pin-check.ps1 -Verbose
#   pwsh tools/ci-tools-pin-check.ps1 -Self

[CmdletBinding()]
param(
    [switch]$Self,
    [switch]$Verbose
)

$ErrorActionPreference = 'Stop'

$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$RepoRoot = Resolve-Path (Join-Path $ScriptDir '..')
$PinsFile = Join-Path $RepoRoot 'tools/PINS.toml'

if (-not (Test-Path $PinsFile)) {
    Write-Error "ERROR: $PinsFile not found"
    exit 2
}

# ----------------------------------------------------------------------------
# Same allowlist + scan list as the bash twin.
# ----------------------------------------------------------------------------
$ScanBinaries = @(
    'jq', 'curl', 'wget', 'openssl', 'base64', 'sha256sum',
    'apt-get', 'apt', 'brew', 'pip', 'pip3', 'npm'
)

$Allowlist = @(
    'bash', 'sh', 'zsh', 'dash',
    'git', 'svn',
    'go', 'gofmt',
    'flutter', 'dart',
    'docker', 'docker-compose', 'podman',
    'python', 'python3', 'pip', 'pip3',
    'node', 'npm', 'npx',
    'awk', 'sed', 'grep', 'egrep', 'fgrep', 'cut', 'sort', 'uniq',
    'tr', 'head', 'tail', 'wc',
    'find', 'xargs', 'which', 'command',
    'test', 'echo', 'printf', 'cat', 'ls', 'cp', 'mv', 'rm',
    'mkdir', 'rmdir', 'ln', 'touch', 'chmod', 'chown',
    'tar', 'gzip', 'gunzip', 'zip', 'unzip',
    'date', 'env', 'true', 'false',
    'dirname', 'basename', 'realpath', 'readlink',
    'make',
    'protoc'
)

$Reasons = @{
    'jq'        = 'pin via tools/PINS.toml (github-release) + tools/install-pinned.sh'
    'curl'      = 'pin via tools/PINS.toml (github-release) + tools/install-pinned.sh; or use pinned apt/brew version'
    'wget'      = 'pin via tools/PINS.toml + tools/install-pinned.sh'
    'openssl'   = 'pin via apt/brew with explicit version; record entry in tools/PINS.toml'
    'base64'    = 'dist-default on runners; record entry in tools/PINS.toml (os-default) to satisfy policy'
    'sha256sum' = 'dist-default (coreutils); record entry in tools/PINS.toml (os-default) to satisfy policy'
    'apt-get'   = 'use `apt-get install -y <pkg>=<version>`; record entry in tools/PINS.toml (apt)'
    'apt'       = 'use `apt install -y <pkg>=<version>`; record entry in tools/PINS.toml (apt)'
    'brew'      = 'use `brew install <formula>@<version>`; record entry in tools/PINS.toml (brew)'
    'pip'       = 'use `pip install -r requirements.txt` with --require-hashes; never `pip install <pkg>` unpinned'
    'pip3'      = 'use `pip3 install -r requirements.txt` with --require-hashes; never `pip3 install <pkg>` unpinned'
    'npm'       = 'use `npm ci` (NOT `npm install`); record entry in tools/PINS.toml for global tool installs'
}

$ScanRoots = @('scripts', 'infra/scripts', 'tools', '.github/workflows')

# ----------------------------------------------------------------------------
# Read pinned binaries from PINS.toml.
# ----------------------------------------------------------------------------
$PinnedNames = @{}
Get-Content $PinsFile | ForEach-Object {
    if ($_ -match '^\s*name\s*=\s*"([^"]+)"') {
        $name = $Matches[1]
        if (-not $PinnedNames.ContainsKey($name)) {
            $PinnedNames[$name] = $true
        }
    }
}

function Test-Pinned([string]$Binary) {
    return $PinnedNames.ContainsKey($Binary)
}

function Test-Allowlisted([string]$Binary) {
    return $Allowlist -contains $Binary
}

# ----------------------------------------------------------------------------
# Run the Python scanner helper. The .ps1 twin delegates comment-strip
# and token-match to tools/lib/scanner.py (the same helper the .sh
# twin uses) — keeping one source of truth for the match logic and
# avoiding PowerShell-quoting foot-guns on the embedded code.
# ----------------------------------------------------------------------------
function Invoke-Scanner([string]$FilePath) {
    $py = $null
    if (Get-Command python3 -ErrorAction SilentlyContinue) { $py = 'python3' }
    elseif (Get-Command python -ErrorAction SilentlyContinue) { $py = 'python' }
    if (-not $py) { return @() }
    $helper = Join-Path $ScriptDir 'lib/scanner.py'
    if (-not (Test-Path $helper)) { return @() }
    $output = & $py $helper match $FilePath 2>$null
    if ($null -eq $output) { return @() }
    $result = @()
    foreach ($line in $output) {
        if ($line -match '^(\d+)\|(.+)$') {
            $result += [pscustomobject]@{ Lineno = [int]$Matches[1]; Binary = $Matches[2] }
        }
    }
    return $result
}

# ----------------------------------------------------------------------------
# Scan one file.
# ----------------------------------------------------------------------------
$Violations = 0
$Clean = 0
$ScanRoots | ForEach-Object {
    $full = Join-Path $RepoRoot $_
    if (-not (Test-Path $full)) { return }
    Get-ChildItem -Path $full -Recurse -File -Include '*.ps1', '*.psm1', '*.yml', '*.yaml' -ErrorAction SilentlyContinue | ForEach-Object {
        $file = $_.FullName
        $rel = $file.Substring($RepoRoot.Path.Length + 1)

        # Skip self unless -Self
        if (-not $Self) {
            if ($file -match 'ci-tools-pin-check\.ps1$' -or $file -match 'install-pinned\.ps1$') {
                return
            }
        }
        # Skip meta files
        if ($file -match '\\tools\\PINS\.toml$' -or $file -match '\\tools\\README\.md$' -or $file -match '\\docs\\CI-TOOLS\.md$') {
            return
        }

        # File-level skip directive
        $content = Get-Content $file -Raw -ErrorAction SilentlyContinue
        if ($content -match '(?m)^\s*#\s*tools-pin:\s*skip') {
            if ($Verbose) { Write-Host "  [skip-file] $rel (has 'tools-pin: skip' directive)" }
            return
        }

        $matches = Invoke-Scanner $file
        if ($null -eq $matches -or $matches.Count -eq 0) { return }

        foreach ($m in $matches) {
            $binary = $m.Binary
            $lineno = $m.Lineno
            if (Test-Allowlisted $binary) {
                if ($Verbose) { Write-Host "  [allow]  ${rel}:$lineno  $binary" }
                continue
            }
            if (Test-Pinned $binary) {
                if ($Verbose) { Write-Host "  [pin]    ${rel}:$lineno  $binary" }
                $script:Clean++
                continue
            }
            $reason = $Reasons[$binary]
            if (-not $reason) { $reason = 'add an entry to tools/PINS.toml' }
            Write-Host "  [FAIL]   ${rel}:$lineno  $binary  -> $reason"
            $script:Violations++
        }
    }
}

Write-Host ""
Write-Host "==> ci-tools-pin-check summary"
Write-Host "    scanned roots:        $($ScanRoots -join ' ')"
Write-Host "    pinned binaries:      $($PinnedNames.Count)"
Write-Host "    matched-and-pinned:   $Clean"
Write-Host "    violations:           $Violations"

if ($Violations -gt 0) {
    Write-Host ""
    Write-Host "FAIL: $Violations unpinned 3rd-party invocation(s) detected."
    Write-Host "      Add an entry to tools/PINS.toml or use the existing pinned binary,"
    Write-Host "      or add a `# tools-pin: skip` directive at the top of the file with"
    Write-Host "      justification in the PR description."
    Write-Host "      See docs/CI-TOOLS.md for the full policy."
    exit 1
}

Write-Host ""
Write-Host "PASS: all 3rd-party binary invocations are pinned or allowlisted."
exit 0