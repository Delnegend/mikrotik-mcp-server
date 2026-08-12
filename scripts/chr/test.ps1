<#
scripts/chr/test.ps1 - run the full test suite against the RouterOS CHR VM.

Boots the CHR (idempotent via up.sh), provisions it if fresh, then runs
`go test ./...` with the VM environment set. The behavioral integration
suite (internal/integration) runs against the live router; the pure-logic
suite stays green without a VM.

Usage:
  ./scripts/chr/test.ps1               # boot + provision + full suite (VM stays up)
  ./scripts/chr/test.ps1 -Fresh        # wipe CHR state and start clean
  ./scripts/chr/test.ps1 -Down         # stop the VM after the suite
  ./scripts/chr/test.ps1 -Tag CHR      # pass -run to go test

Requires: Go on Windows, WSL2. up.sh auto-installs qemu and joins the kvm group.
#>
param(
    [switch]$Fresh,
    [switch]$Down,
    [string]$Tag = "",
    [string]$Password = "admin"
)

$ErrorActionPreference = "Continue"

$here = $PSScriptRoot
$repo = Split-Path (Split-Path $here -Parent) -Parent   # repo root (scripts/chr -> repo)

function ConvertTo-WslPath([string]$p) {
    $drive = $p.Substring(0, 1).ToLowerInvariant()
    return "/mnt/$drive" + $p.Substring(2).Replace('\', '/')
}

$wslUp   = ConvertTo-WslPath (Join-Path $here "up.sh")
$wslDown = ConvertTo-WslPath (Join-Path $here "down.sh")

$upArgs = @("--no-install")
if ($Fresh) { $upArgs += "--fresh" }

Write-Host "==> booting CHR (idempotent)"
& wsl -e bash $wslUp @upArgs
if ($LASTEXITCODE -ne 0) { throw "up.sh failed (exit $LASTEXITCODE)" }

Push-Location $repo
try {
    Write-Host "==> ensuring provisioned (best effort; 'already provisioned' is fine)"
    & go run ./cmd/chrprovision -password $Password 2>$null | Out-Null

    Write-Host "==> running full test suite against the VM"
    $env:MIKROTIK_TEST_HOST = "127.0.0.1"
    $env:MIKROTIK_TEST_USER = "admin"
    $env:MIKROTIK_TEST_PASSWORD = $Password
    $env:MIKROTIK_TEST_PORT = "8728"

    if ($Tag) { & go test ./... -count=1 -run $Tag } else { & go test ./... -count=1 }
    $code = $LASTEXITCODE
}
finally {
    Pop-Location
}

if ($Down) {
    Write-Host "==> stopping CHR"
    & wsl -e bash $wslDown
}

exit $code
