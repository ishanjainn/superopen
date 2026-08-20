# Superopen CLI (`so`) installer for Windows.
#
# Release (no checkout): downloads the latest GitHub Release zip into
# $env:USERPROFILE\.superopen\bin\so.exe and adds that directory to the
# user-scope PATH. Then run: so install
#
# Local checkout: builds from source into the same prefix, adds PATH, and
# runs `so install` (same layout as production install.ps1 users).
#
# Usage:
#   iwr -useb https://raw.githubusercontent.com/ishanjainn/superopen/main/scripts/install.ps1 | iex
#   powershell -File scripts/install.ps1    # from a git checkout
#
# Environment overrides:
#   $env:SUPEROPEN_INSTALL_DIR  Target install directory.
#                               Default: $env:USERPROFILE\.superopen\bin
#   $env:SUPEROPEN_VERSION      Release tag WITHOUT the `cli-` prefix,
#                               e.g. `1.2.0`. Default: `latest`.
#   $env:SUPEROPEN_REPO         GitHub owner/repo. Default: ishanjainn/superopen

$ErrorActionPreference = 'Stop'

$Repo = if ($env:SUPEROPEN_REPO) { $env:SUPEROPEN_REPO } else { 'ishanjainn/superopen' }
$InstallDir = if ($env:SUPEROPEN_INSTALL_DIR) {
    $env:SUPEROPEN_INSTALL_DIR
} else {
    Join-Path $env:USERPROFILE '.superopen\bin'
}
$Version = if ($env:SUPEROPEN_VERSION) { $env:SUPEROPEN_VERSION } else { 'latest' }

function Write-So($msg)     { Write-Host "so: $msg" }
function Write-SoWarn($msg) { Write-Warning "so: $msg" }
function Stop-So($msg)      { throw "so: $msg" }

function Install-SoWeb([string]$WebSrc) {
    $pkg = Join-Path $WebSrc 'package.json'
    if (-not (Test-Path $pkg)) { Stop-So "web UI sources missing at $WebSrc" }
    $webDst = Join-Path (Split-Path $InstallDir -Parent) 'share\superopen\web'
    Write-So "Installing web UI into $webDst"
    if (Test-Path $webDst) { Remove-Item -Recurse -Force $webDst }
    New-Item -ItemType Directory -Force -Path (Split-Path $webDst -Parent) | Out-Null
    Copy-Item -Recurse $WebSrc $webDst
    Remove-Item -Recurse -Force -ErrorAction SilentlyContinue (Join-Path $webDst 'node_modules'), (Join-Path $webDst '.next')
    if (-not (Get-Command npm -ErrorAction SilentlyContinue)) {
        Stop-So 'npm not found; Node.js is required to install the Superopen UI'
    }
    Write-So 'npm install --ignore-scripts (web UI)'
    Push-Location $webDst
    try {
        npm install --ignore-scripts
        Write-So 'npm run build (web UI)'
        npm run build
    } finally {
        Pop-Location
    }
}
    $currentUserPath = [Environment]::GetEnvironmentVariable('Path', 'User')
    if (-not $currentUserPath) { $currentUserPath = '' }
    $pathParts = $currentUserPath -split ';' | Where-Object { $_ -ne '' }
    if ($pathParts -notcontains $Dir) {
        $newPath = if ($currentUserPath) { "$currentUserPath;$Dir" } else { $Dir }
        [Environment]::SetEnvironmentVariable('Path', $newPath, 'User')
        Write-So "Added $Dir to your user PATH (open a new terminal to pick it up)"
    }
    $env:Path = "$Dir;$env:Path"
}

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
        Write-So "Installed: $Out"
        Install-SoWeb (Join-Path $Root 'web')
        Add-SoUserPath $InstallDir
        & $Out install
        Write-So 'Done. In a test repo: so init && so dev'
        Write-So 'Wipe with: powershell -File scripts/uninstall.ps1'
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
    Add-SoUserPath $InstallDir

    $srcUrl = if ($Version -eq 'latest') {
        "https://github.com/$Repo/archive/refs/heads/main.zip"
    } else {
        "https://github.com/$Repo/archive/refs/tags/cli-$Version.zip"
    }
    Write-So "Downloading web UI sources"
    $srcZip = Join-Path $tmpDir 'src.zip'
    Invoke-WebRequest -Uri $srcUrl -OutFile $srcZip -UseBasicParsing
    $srcOut = Join-Path $tmpDir 'src'
    Expand-Archive -Path $srcZip -DestinationPath $srcOut -Force
    $webSrc = Get-ChildItem -Path $srcOut -Directory -Filter 'web' -Recurse |
        Where-Object { Test-Path (Join-Path $_.FullName 'package.json') } |
        Select-Object -First 1
    if (-not $webSrc) { Stop-So "no web/ in $srcUrl" }
    Install-SoWeb $webSrc.FullName
    & $target install
    Write-So 'Done. In a test repo: so init && so dev'
}
finally {
    Remove-Item -Recurse -Force $tmpDir -ErrorAction SilentlyContinue
}
