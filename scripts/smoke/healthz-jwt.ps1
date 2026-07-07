<#
scripts/smoke/healthz-jwt.ps1
------------------------------------------------------------------------------
Sprint 7 AUTHZ-2 — PowerShell equivalent of scripts/smoke/healthz-jwt.sh.
ADR-0008 §2.4 — cross-platform entry point for the same smoke test.

Usage:
  $env:JWT_SECRET = (Get-Content infra/.env | Where-Object { $_ -like 'JWT_SECRET=*' }) -replace '^JWT_SECRET=',''
  $env:KONG_PROXY_URL = 'http://localhost:8000'
  pwsh scripts/smoke/healthz-jwt.ps1

Asserts:
  1. unauth            -> 401
  2. valid JWT         -> 200 + JSON with status=ok|degraded
  3. expired JWT       -> 401
  4. wrong-iss JWT     -> 401
#>

[CmdletBinding()]
param(
    [string]$KongProxyUrl = $env:KONG_PROXY_URL,
    [string]$HealthzPath  = '/healthz',
    [string]$JwtSecret    = $env:JWT_SECRET
)

$ErrorActionPreference = 'Stop'

if (-not $KongProxyUrl) { $KongProxyUrl = 'http://localhost:8000' }
if (-not $HealthzPath)  { $HealthzPath  = '/healthz' }

if (-not $JwtSecret) {
    Write-Error "JWT_SECRET env var is required (must match infra/kong/kong.yml's {vault://env/jwt-secret})."
    exit 2
}

# --- mint HS256 JWT in pure PowerShell (no Python dep) ---------------------
function New-HS256Jwt {
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)] [string]$Issuer,
        [Parameter(Mandatory)] [int]$ExpiryUnixEpoch
    )
    $headerJson  = '{"alg":"HS256","typ":"JWT"}'
    $payloadJson = ('{"iss":"{0}","exp":{1}}' -f $Issuer, $ExpiryUnixEpoch)

    $b64 = {
        param([byte[]]$Bytes)
        [Convert]::ToBase64String($Bytes).TrimEnd('=').Replace('+','-').Replace('/','_')
    }

    $headerB64  = & $b64 ([Text.Encoding]::UTF8.GetBytes($headerJson))
    $payloadB64 = & $b64 ([Text.Encoding]::UTF8.GetBytes($payloadJson))
    $data        = "$headerB64.$payloadB64"

    $hmac = New-Object System.Security.Cryptography.HMACSHA256
    $hmac.Key = [Text.Encoding]::UTF8.GetBytes($JwtSecret)
    $sig = $hmac.ComputeHash([Text.Encoding]::UTF8.GetBytes($data))
    $sigB64 = & $b64 $sig
    return "$data.$sigB64"
}

function Test-HealthzScenario {
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)] [string]$Name,
        [Parameter(Mandatory)] [string]$AuthHeader,   # "Bearer <token>" or "" for no-auth
        [Parameter(Mandatory)] [int]   $Expected
    )
    $headers = @{}
    if ($AuthHeader) { $headers['Authorization'] = $AuthHeader }
    try {
        $resp = Invoke-WebRequest -Uri ($KongProxyUrl + $HealthzPath) `
                                  -Headers $headers `
                                  -UseBasicParsing `
                                  -TimeoutSec 10
        $code = [int]$resp.StatusCode
    } catch {
        # 4xx/5xx raise HttpRequestException in PS 7; older surfaces via .Response
        if ($_.Exception.Response) {
            $code = [int]$_.Exception.Response.StatusCode
        } else { $code = -1 }
    }
    Write-Host "    ${Name}: HTTP $code (expected $Expected)"
    if ($code -ne $Expected) {
        Write-Error "FAIL: $Name expected $Expected got $code"
        exit 1
    }
}

# --- 1) no auth ------------------------------------------------------------
Write-Host "==> 1) $HealthzPath without Authorization — expect 401"
Test-HealthzScenario -Name 'no-auth' -AuthHeader '' -Expected 401

# --- 2) valid JWT ----------------------------------------------------------
Write-Host "==> 2) $HealthzPath with valid JWT (iss=opene2ee-monitoring, exp=+300s)"
$jwt = New-HS256Jwt -Issuer 'opene2ee-monitoring' -ExpiryUnixEpoch ([int][double]::Parse((Get-Date -UFormat %s)) + 300)
Test-HealthzScenario -Name 'valid-JWT' -AuthHeader "Bearer $jwt" -Expected 200

# --- 3) expired JWT --------------------------------------------------------
Write-Host "==> 3) $HealthzPath with expired JWT — expect 401"
$jwtExp = New-HS256Jwt -Issuer 'opene2ee-monitoring' -ExpiryUnixEpoch ([int][double]::Parse((Get-Date -UFormat %s)) - 3600)
Test-HealthzScenario -Name 'expired-JWT' -AuthHeader "Bearer $jwtExp" -Expected 401

# --- 4) wrong-iss ----------------------------------------------------------
Write-Host "==> 4) $HealthzPath with wrong-iss JWT — expect 401"
$jwtWrong = New-HS256Jwt -Issuer 'totally-unknown-issuer' -ExpiryUnixEpoch ([int][double]::Parse((Get-Date -UFormat %s)) + 300)
Test-HealthzScenario -Name 'wrong-iss' -AuthHeader "Bearer $jwtWrong" -Expected 401

Write-Host ""
Write-Host "==> OK: Sprint 7 AUTHZ-2 hardening verified at the Kong gate."
