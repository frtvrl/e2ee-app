# tools/install-pinned.ps1 — PowerShell twin of install-pinned.sh.
# Mirrors the bash installer for Windows CI / ADR-0008 cross-platform.
#
# Sprint 7 / STRIDE-8-03 — same policy as the .sh twin.
#
# Usage:
#   pwsh tools/install-pinned.ps1                 # install everything
#   pwsh tools/install-pinned.ps1 -Only jq        # install one tool
#   pwsh tools/install-pinned.ps1 -DryRun         # print plan, no downloads

[CmdletBinding()]
param(
    [string]$Only = '',
    [switch]$DryRun
)

$ErrorActionPreference = 'Stop'

$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$RepoRoot = Resolve-Path (Join-Path $ScriptDir '..')
$PinsFile = Join-Path $RepoRoot 'tools/PINS.toml'
$BinDir = Join-Path $RepoRoot 'tools/bin'

if (-not (Test-Path $PinsFile)) {
    Write-Error "ERROR: $PinsFile not found"
    exit 1
}

if (-not (Test-Path $BinDir)) {
    New-Item -ItemType Directory -Path $BinDir | Out-Null
}

# ----------------------------------------------------------------------------
# Tiny TOML parser — emits one block at a time.
# ----------------------------------------------------------------------------
function Parse-Toml([string]$Path) {
    $content = Get-Content $Path -Encoding UTF8
    $inBlock = $false
    $current = @{}
    foreach ($line in $content) {
        if ($line -match '^\s*#') { continue }
        if ($line -match '^\s*\[\[tool\]\]') {
            if ($inBlock -and $current.Count -gt 0) {
                , $current
            }
            $inBlock = $true
            $current = @{}
            continue
        }
        if ($inBlock -and $line -match '^\s*$') { continue }
        if ($inBlock -and $line -match '^\s*([a-z_]+)\s*=\s*"?([^"]*)"?\s*$') {
            $key = $Matches[1]
            $val = $Matches[2]
            $current[$key] = $val
        }
    }
    if ($inBlock -and $current.Count -gt 0) {
        , $current
    }
}

function Install-GithubRelease([hashtable]$Tool) {
    $name = $Tool['name']
    $version = $Tool['version']
    $upstream = $Tool['upstream']
    $expectedSha = $Tool['sha256']
    $sumsUrl = $Tool['sha256_url']

    if ($DryRun) {
        Write-Host "    [dry-run] would download $upstream"
        Write-Host "    [dry-run] would verify SHA-256 = $($expectedSha.Substring(0, [Math]::Min(16, $expectedSha.Length)))..."
        Write-Host "    [dry-run] would install to $BinDir/$name"
        return
    }

    if ([string]::IsNullOrWhiteSpace($expectedSha)) {
        Write-Host "    [error] PINS.toml: $name@$version has empty sha256 for source=github-release"
        return
    }

    $artefact = Join-Path $BinDir ".$name-$version.artefact"
    $sumsFile = Join-Path $BinDir ".$name-$version.SHA256SUMS"

    Write-Host "    -> downloading $(Split-Path $upstream -Leaf)"
    try {
        Invoke-WebRequest -Uri $upstream -OutFile $artefact -UseBasicParsing -ErrorAction Stop
    }
    catch {
        Write-Host "    [error] download failed for $upstream"
        Remove-Item -Force $artefact -ErrorAction SilentlyContinue
        return
    }

    Write-Host "    -> downloading SHA256SUMS"
    try {
        Invoke-WebRequest -Uri $sumsUrl -OutFile $sumsFile -UseBasicParsing -ErrorAction Stop
    }
    catch {
        Write-Host "    [warn] SHA256SUMS download failed ($sumsUrl); relying on PINS.toml expected_sha only"
        Remove-Item -Force $sumsFile -ErrorAction SilentlyContinue
    }

    Write-Host "    -> verifying SHA-256"
    if (Test-Path $sumsFile) {
        $line = Get-Content $sumsFile | Where-Object { $_ -match ([regex]::Escape((Split-Path $upstream -Leaf))) -or $_ -match $version } | Select-Object -First 1
        if ($line) {
            $upstreamSha = ($line -split '\s+')[0]
            if ($upstreamSha -ne $expectedSha) {
                Write-Host "    [error] upstream SHA mismatch: expected $($expectedSha.Substring(0,16))..., got $($upstreamSha.Substring(0,16))..."
                Remove-Item -Force $artefact, $sumsFile -ErrorAction SilentlyContinue
                return
            }
            Write-Host "    [OK] upstream SHA256SUMS verified"
        }
        else {
            Write-Host "    [warn] no matching line in SHA256SUMS for $(Split-Path $upstream -Leaf)"
        }
    }

    $actualSha = (Get-FileHash $artefact -Algorithm SHA256).Hash.ToLower()
    if ($actualSha -ne $expectedSha.ToLower()) {
        Write-Host "    [error] SHA-256 mismatch:"
        Write-Host "             expected: $expectedSha"
        Write-Host "             actual:   $actualSha"
        Remove-Item -Force $artefact, $sumsFile -ErrorAction SilentlyContinue
        return
    }
    Write-Host "    [OK] SHA-256 verified: $($actualSha.Substring(0,16))..."

    $final = Join-Path $BinDir $name
    if ($upstream -match '\.tar\.gz$|\.tgz$') {
        $extractDir = Join-Path $BinDir ".extract-$name-$version"
        New-Item -ItemType Directory -Path $extractDir | Out-Null
        tar -xzf $artefact -C $extractDir
        $extracted = Get-ChildItem -Path $extractDir -Recurse -File -Filter $name | Where-Object { -not $_.PSIsContainer } | Select-Object -First 1
        if (-not $extracted) {
            Write-Host "    [error] could not locate $name inside the tarball"
            Remove-Item -Recurse -Force $artefact, $sumsFile, $extractDir -ErrorAction SilentlyContinue
            return
        }
        Move-Item -Force $extracted.FullName $final
        Remove-Item -Recurse -Force $extractDir, $sumsFile -ErrorAction SilentlyContinue
    }
    else {
        Move-Item -Force $artefact $final
        Remove-Item -Force $sumsFile -ErrorAction SilentlyContinue
    }

    Write-Host "    [installed] $final"
}

# ----------------------------------------------------------------------------
# Main.
# ----------------------------------------------------------------------------

$tools = Parse-Toml $PinsFile
foreach ($tool in $tools) {
    $name = $tool['name']
    if (-not [string]::IsNullOrWhiteSpace($Only) -and $Only -ne $name) {
        continue
    }
    Write-Host "==> $name $($tool['version']) (source=$($tool['source']))"
    switch ($tool['source']) {
        'github-release' {
            Install-GithubRelease $tool
        }
        'apt' {
            Write-Host "    [manual] operator: sudo apt-get install -y $name=$($tool['version'])"
            Write-Host "             (see docs/CI-TOOLS.md §Rotation procedure)"
        }
        'brew' {
            Write-Host "    [manual] operator: brew install $($tool['upstream'])"
            Write-Host "             (see docs/CI-TOOLS.md §Rotation procedure)"
        }
        'go-install' {
            Write-Host "    [manual] operator: go install $($tool['upstream'])@$($tool['version'])"
            Write-Host "             (see docs/CI-TOOLS.md §Rotation procedure)"
        }
        'os-default' {
            Write-Host "    [skip] OS-default; integrity owned by the runner image"
        }
        default {
            Write-Host "    [warn] unknown source '$($tool['source'])' - skipping"
        }
    }
}

Write-Host ""
Write-Host "Done. Pinned binaries (if any) are under $BinDir/."
Write-Host "Run tools/ci-tools-pin-check.ps1 to verify everything still complies."