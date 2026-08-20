# Superopen CLI uninstall for Windows.
#
# Runs the same `so uninstall` production users run. Locates so.exe at
# the release-installer prefix first (%USERPROFILE%\.superopen\bin).
#
# Usage:
#   powershell -File scripts/uninstall.ps1
#   powershell -File scripts/uninstall.ps1 --keep-data

$ErrorActionPreference = 'Stop'

$InstallDir = if ($env:SUPEROPEN_INSTALL_DIR) {
    $env:SUPEROPEN_INSTALL_DIR
} else {
    Join-Path $env:USERPROFILE '.superopen\bin'
}

function Write-So($msg) { Write-Host "so: $msg" }
function Stop-So($msg)  { throw "so: $msg" }

$bin = Join-Path $InstallDir 'so.exe'
if (-not (Test-Path $bin)) {
    $cmd = Get-Command so.exe -ErrorAction SilentlyContinue
    if (-not $cmd) { $cmd = Get-Command so -ErrorAction SilentlyContinue }
    if ($cmd) {
        $bin = $cmd.Source
    } else {
        Stop-So "so.exe not found in $InstallDir or PATH"
    }
}

Write-So "Running: $bin uninstall $($args -join ' ')"
& $bin uninstall @args
Write-So "Restart your coding agent so it drops in-memory hooks and MCP."
