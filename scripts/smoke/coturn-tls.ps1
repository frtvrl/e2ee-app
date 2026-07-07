# ============================================================================
# Coturn TLS 1.2+ Smoke Test — Sprint 7 SCA-22 (PowerShell variant)
# ----------------------------------------------------------------------------
# Windows companion to scripts/smoke/coturn-tls.sh — same five assertions,
# uses openssl.exe (Git Bash / native Windows OpenSSL) + Test-NetConnection
# for reachability, and PSCustomObject for the result tally.
#
# Run from repo root (PowerShell 5.1 compatible):
#     powershell -ExecutionPolicy Bypass -File scripts\smoke\coturn-tls.ps1 `
#         -Host turn.opene2ee.com -Port 5349
#
# Or with defaults:
#     powershell -ExecutionPolicy Bypass -File scripts\smoke\coturn-tls.ps1
#
# Exit codes:
#   0 = all assertions passed
#   1 = one or more assertions failed
#   2 = openssl missing / network unreachable
#
# NOTE on UTF-8 (WinPS 5.1): this script writes only ASCII to stdout.
# Pipe to Out-File with -Encoding ASCII if redirecting to a log.
# ============================================================================
[CmdletBinding()]
param(
    [string] $Host = 'turn.opene2ee.com',
    [int]    $Port = 5349
)

$ErrorActionPreference = 'Stop'

# ---- Pre-flight ------------------------------------------------------------
$openssl = (Get-Command openssl.exe -ErrorAction SilentlyContinue)
if (-not $openssl) {
    Write-Error 'FAIL: openssl.exe not found in PATH (install Git for Windows or OpenSSL).'
    exit 2
}

Write-Host '=== Coturn TLS 1.2+ smoke test (PowerShell) ==='
Write-Host "Target: ${Host}:${Port}"
Write-Host ''

# TCP reachability probe (skip - TURN-over-TCP listener).
try {
    $tcp = Test-NetConnection -ComputerName $Host -Port $Port -WarningAction SilentlyContinue -InformationLevel Quiet
    if (-not $tcp) {
        Write-Error "FAIL: TCP connect to ${Host}:${Port} refused or timed out"
        exit 2
    }
} catch {
    Write-Error "FAIL: Test-NetConnection error: $_"
    exit 2
}

# ---- Helpers ---------------------------------------------------------------
$probeOut = [System.IO.Path]::GetTempFileName()

function Invoke-TlsProbe {
    param([string] $Version)
    # openssl s_client: echo "" via stdin forces handshake then EOF.
    # Exit code: 0 = handshake succeeded, non-zero = refused/errored.
    $psi = New-Object System.Diagnostics.ProcessStartInfo
    $psi.FileName               = $openssl.Source
    $psi.Arguments              = @(
        's_client',
        "-connect", "${Host}:${Port}",
        "-$Version",
        '-no_ign_eof',
        '-servername', $Host,
        '-verify_return_error'
    ) -join ' '
    $psi.RedirectStandardInput  = $true
    $psi.RedirectStandardOutput = $true
    $psi.RedirectStandardError  = $true
    $psi.UseShellExecute        = $false
    $psi.CreateNoWindow         = $true

    $p = [System.Diagnostics.Process]::Start($psi)
    $p.StandardInput.Close()    # send EOF immediately
    $out = $p.StandardOutput.ReadToEnd()
    $err = $p.StandardError.ReadToEnd()
    $p.WaitForExit()
    Set-Content -Path $probeOut -Value ($out + "`n" + $err) -Encoding ASCII -NoNewline
    return $p.ExitCode
}

function Get-ProbeField {
    param([string] $Pattern)
    if (Test-Path $probeOut) {
        Select-String -Path $probeOut -Pattern $Pattern | Select-Object -First 1 | ForEach-Object { $_.Matches[0].Value }
    }
}

function Test-ProbeMatch {
    param([string] $Pattern)
    if (Test-Path $probeOut) {
        return [bool] (Select-String -Path $probeOut -Pattern $Pattern -Quiet)
    }
    return $false
}

# ---- Assertions ------------------------------------------------------------
$script:pass = 0
$script:fail = 0

function Assert-Equal {
    param([string] $Label, [string] $Expected, [string] $Actual)
    if ($Expected -eq $Actual) {
        Write-Host "  [PASS] ${Label}: ${Actual}"
        $script:pass++
    } else {
        Write-Error "  [FAIL] ${Label}: expected='${Expected}' actual='${Actual}'"
        $script:fail++
    }
}

function Assert-CommandExitCode {
    param([string] $Label, [int] $Expected, [int] $Actual)
    $expectedNorm = if ($Expected -eq 0) { 'zero' } else { 'nonzero' }
    $actualNorm   = if ($Actual   -eq 0) { 'zero' } else { 'nonzero' }
    if ($expectedNorm -eq $actualNorm) {
        Write-Host "  [PASS] ${Label}: rc=${Actual}"
        $script:pass++
    } else {
        Write-Error "  [FAIL] ${Label}: expected rc=${Expected} got rc=${Actual}"
        $script:fail++
    }
}

function Assert-Match {
    param([string] $Label, [string] $Pattern, [string] $Actual)
    if ([regex]::IsMatch($Actual, $Pattern)) {
        Write-Host "  [PASS] ${Label}: matches /$Pattern/"
        $script:pass++
    } else {
        Write-Error "  [FAIL] ${Label}: '${Actual}' does not match /$Pattern/"
        $script:fail++
    }
}

# ---- 1. TLS 1.0 must be refused --------------------------------------------
Write-Host '[1/5] Probe TLS 1.0 (must FAIL handshake)'
$rcTls10 = Invoke-TlsProbe -Version 'tls1'
Assert-CommandExitCode -Label 'TLS 1.0 refused' -Expected 1 -Actual $rcTls10

# ---- 2. TLS 1.1 must be refused --------------------------------------------
Write-Host '[2/5] Probe TLS 1.1 (must FAIL handshake)'
$rcTls11 = Invoke-TlsProbe -Version 'tls1_1'
Assert-CommandExitCode -Label 'TLS 1.1 refused' -Expected 1 -Actual $rcTls11

# ---- 3. TLS 1.2 must succeed -----------------------------------------------
Write-Host '[3/5] Probe TLS 1.2 (must SUCCEED handshake)'
$rcTls12 = Invoke-TlsProbe -Version 'tls1_2'
Assert-CommandExitCode -Label 'TLS 1.2 accepted' -Expected 0 -Actual $rcTls12

if ($rcTls12 -ne 0) {
    Write-Host '  ---- openssl output ----'
    Get-Content $probeOut | ForEach-Object { Write-Host "    $_" }
    Write-Host '  -----------------------'
}

# ---- 4. Cert chain validates + subject sane --------------------------------
Write-Host '[4/5] Validate TLS 1.2 cert chain'
$chainOk = Test-ProbeMatch -Pattern '^\s*Verify return code:\s*0\s*\(ok\)'
if ($chainOk) {
    Write-Host '  [PASS] cert chain verifies against system trust'
    $script:pass++
} else {
    Write-Error '  [FAIL] cert chain did NOT verify (see openssl output above)'
    $script:fail++
}

$subject = Get-ProbeField -Pattern '^\s*subject=\S.*$'
if ([string]::IsNullOrEmpty($subject)) {
    Write-Error '  [FAIL] could not read subject from TLS 1.2 probe'
    $script:fail++
} else {
    Write-Host "  subject: $subject"
    # subject=CN=... — strip the leading "subject=" for the regex.
    $subjectBody = ($subject -replace '^\s*subject=', '').Trim()
    Assert-Match -Label 'subject includes host' -Pattern "^CN\s*=\s*([a-z0-9-]+\.)*${($Host -replace '\.', '\.')}" -Actual $subjectBody
}

# ---- 5. Cipher is forward-secret -------------------------------------------
Write-Host '[5/5] Negotiated cipher must be forward-secret'
$cipher = Get-ProbeField -Pattern '^\s*Cipher\s*:\s*\S.*$'
if ([string]::IsNullOrEmpty($cipher)) {
    Write-Error '  [FAIL] could not read cipher from TLS 1.2 probe'
    $script:fail++
} else {
    Write-Host "  cipher : $cipher"
    $cipherBody = ($cipher -replace '^\s*Cipher\s*:\s*', '').Trim()
    Assert-Match -Label 'cipher uses PFS (ECDHE/DHE + AESGCM/CHACHA20)' `
        -Pattern '(ECDHE-(AESGCM|CHACHA20)|DHE-(AESGCM|CHACHA20))' `
        -Actual $cipherBody
}

# ---- Summary ---------------------------------------------------------------
Write-Host ''
Write-Host '=== Summary ==='
Write-Host "PASS=$($script:pass)  FAIL=$($script:fail)  total=5"

Remove-Item -Path $probeOut -Force -ErrorAction SilentlyContinue

if ($script:fail -eq 0) {
    Write-Host 'OK: 5/5 coturn-tls assertions passed'
    exit 0
}

Write-Error "FAIL: $($script:fail)/5 coturn-tls assertions failed"
Write-Host 'Hint: check COTURN_TLS_ENABLED=true in infra\.env and cert paths in' `
    'COTURN_TLS_CERTDIR\live\${COTURN_TLS_DOMAIN}\.'
exit 1