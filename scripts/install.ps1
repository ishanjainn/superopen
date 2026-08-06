# Superopen CLI (`so`) installer for Windows.
#
# Detects architecture, downloads the matching zip from the latest
# `cli-*.*.*` GitHub Release, extracts the binary to
# $env:USERPROFILE\.superopen\bin\so.exe, and adds that directory to
# the user-scope PATH if it is not already there.
#
# Usage:
#   iwr -useb https://raw.githubusercontent.com/superopen/so/main/scripts/install.ps1 | iex
#
# Environment overrides:
#   $env:SUPEROPEN_INSTALL_DIR  Target install directory.
#                               Default: $env:USERPROFILE\.superopen\bin
#   $env:SUPEROPEN_VERSION      Release tag WITHOUT the `cli-` prefix,
#                               e.g. `1.2.0`. Default: `latest`.
#   $env:SUPEROPEN_REPO         GitHub owner/repo. Default: superopen/so

$ErrorActionPreference = 'Stop'

$Repo = if ($env:SUPEROPEN_REPO) { $env:SUPEROPEN_REPO } else { 'superopen/so' }
$InstallDir = if ($env:SUPEROPEN_INSTALL_DIR) {
    $env:SUPEROPEN_INSTALL_DIR
} else {
    Join-Path $env:USERPROFILE '.superopen\bin'
}
$Version = if ($env:SUPEROPEN_VERSION) { $env:SUPEROPEN_VERSION } else { 'latest' }

function Write-So($msg)     { Write-Host "so: $msg" }
function Write-SoWarn($msg) { Write-Warning "so: $msg" }
function Stop-So($msg)      { throw "so: $msg" }

# --- Local source fallback --------------------------------------------------

$ScriptDir = $null
if ($MyInvocation.MyCommand.Path) {
    $ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
}
if ($ScriptDir) {
    $Root = Join-Path $ScriptDir '..'
    $MainGo = Join-Path $Root 'cmd\so\main.go'
    if ((Test-Path $MainGo) -and (Get-Command go -ErrorAction SilentlyContinue)) {
        Write-So 'Building so from local source...'
        New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
        $Out = Join-Path $InstallDir 'so.exe'
        Push-Location $Root
        try {
            go build -o $Out ./cmd/so
        } finally {
            Pop-Location
        }
        $env:Path = "$InstallDir;$env:Path"
        Write-So "Installed: $Out"
        & $Out install
        Write-So 'Done. Try /so in your coding agent, then /so init'
        return
    }
}

# --- Detect architecture ----------------------------------------------------

$archEnv = $env:PROCESSOR_ARCHITECTURE
if ($archEnv -eq 'ARM64') {
    $arch = 'arm64'
} elseif ([Environment]::Is64BitOperatingSystem) {
    $arch = 'amd64'
} else {
    Stop-So "unsupported architecture: $archEnv (the CLI is 64-bit only)"
}

# --- Resolve the asset URL --------------------------------------------------

$asset = "so-windows-$arch.zip"
$url = if ($Version -eq 'latest') {
    "https://github.com/$Repo/releases/latest/download/$asset"
} else {
    "https://github.com/$Repo/releases/download/cli-$Version/$asset"
}

Write-So "Downloading $asset"

$tmpDir = Join-Path $env:TEMP ("so-install-" + [System.Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $tmpDir | Out-Null

try {
    $zipPath = Join-Path $tmpDir $asset
    [Net.ServicePointManager]::SecurityProtocol = `
        [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12

    Invoke-WebRequest -Uri $url -OutFile $zipPath -UseBasicParsing

    Expand-Archive -Path $zipPath -DestinationPath $tmpDir -Force

    $extracted = Get-ChildItem -Path $tmpDir -Filter 'so*.exe' -Recurse |
        Select-Object -First 1
    if (-not $extracted) {
        Stop-So "no so*.exe found inside $asset"
    }

    if (-not (Test-Path $InstallDir)) {
        New-Item -ItemType Directory -Path $InstallDir | Out-Null
    }

    $target = Join-Path $InstallDir 'so.exe'
    Move-Item -Path $extracted.FullName -Destination $target -Force

    Write-So "Installed: $target"

    $currentUserPath = [Environment]::GetEnvironmentVariable('Path', 'User')
    if (-not $currentUserPath) { $currentUserPath = '' }

    $pathParts = $currentUserPath -split ';' | Where-Object { $_ -ne '' }
    if ($pathParts -notcontains $InstallDir) {
        $newPath = if ($currentUserPath) { "$currentUserPath;$InstallDir" } else { $InstallDir }
        [Environment]::SetEnvironmentVariable('Path', $newPath, 'User')
        Write-So "Added $InstallDir to your user PATH (open a new terminal to pick it up)"
    }

    Write-So ""
    Write-So "Next (use absolute path if PATH is not updated yet):"
    Write-So "  & `"$target`" install"
    Write-So "  & `"$target`" init"
}
finally {
    Remove-Item -Recurse -Force $tmpDir -ErrorAction SilentlyContinue
}
