# Superopen CLI (`so`) installer for Windows.
#
# Release (no checkout): downloads the latest GitHub Release zip (CLI) and
# so-web.tar.gz (prebuilt UI) into $env:USERPROFILE\.superopen. Then: so install
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

$script:SpinPs = $null
$script:SpinHandle = $null
$script:SpinState = $null

function Stop-Spin {
    if ($script:SpinState) { $script:SpinState.Stop = $true }
    if ($script:SpinPs) {
        try { $null = $script:SpinPs.EndInvoke($script:SpinHandle) } catch {}
        $script:SpinPs.Dispose()
        $script:SpinPs = $null
        $script:SpinHandle = $null
        $script:SpinState = $null
        try { [Console]::Write("`r" + (' ' * 80) + "`r") } catch {}
    }
}

function Start-Spin([string]$Message) {
    Stop-Spin
    $redirected = $false
    try { $redirected = [Console]::IsOutputRedirected } catch { $redirected = $true }
    if ($redirected) {
        Write-Host "  …  $Message"
        return
    }
    $utf = $false
    try { $utf = [Console]::OutputEncoding.WebName -match 'utf-8' } catch {}
    $state = [hashtable]::Synchronized(@{ Stop = $false; Message = $Message; Utf = $utf })
    $script:SpinState = $state
    $script:SpinPs = [powershell]::Create().AddScript({
        param($state)
        $braille = @([char]0x280B, [char]0x2819, [char]0x2839, [char]0x2838, [char]0x283C, [char]0x2834, [char]0x2826, [char]0x2827, [char]0x2807, [char]0x280F)
        $ascii = @('|', '/', '-', '\')
        $frames = if ($state.Utf) { $braille } else { $ascii }
        $i = 0
        while (-not $state.Stop) {
            $frame = $frames[$i % $frames.Count]
            [Console]::Write("`r  $frame  $($state.Message)")
            $i++
            Start-Sleep -Milliseconds 80
        }
    }).AddArgument($state)
    $script:SpinHandle = $script:SpinPs.BeginInvoke()
}

function Write-So($msg)     { Write-Host "so: $msg" }
function Write-SoWarn($msg) { Write-Warning "so: $msg" }
function Stop-So($msg)      { Stop-Spin; throw "so: $msg" }

function Web-Dst {
    Join-Path (Split-Path $InstallDir -Parent) 'share\superopen\web'
}

function Install-SoWebFromSource([string]$WebSrc) {
    $pkg = Join-Path $WebSrc 'package.json'
    if (-not (Test-Path $pkg)) { Stop-So "web UI sources missing at $WebSrc" }
    if (-not (Get-Command npm -ErrorAction SilentlyContinue)) {
        Stop-So 'npm not found; Node.js is required to build the Superopen UI from a checkout'
    }
    Start-Spin 'Building the UI'
    $log = Join-Path $env:TEMP 'so-web-build.log'
    Push-Location $WebSrc
    try {
        cmd /c "npm install --ignore-scripts > `"$log`" 2>&1 && npm run build >> `"$log`" 2>&1"
        if ($LASTEXITCODE -ne 0) {
            Get-Content $log -ErrorAction SilentlyContinue
            Stop-So 'UI build failed'
        }
    } finally {
        Pop-Location
    }
    $standalone = Join-Path $WebSrc '.next\standalone'
    if (-not (Test-Path (Join-Path $standalone 'server.js'))) {
        Stop-So "standalone UI missing at $standalone (next build did not produce output: standalone)"
    }
    $staticSrc = Join-Path $WebSrc '.next\static'
    if (Test-Path $staticSrc) {
        $staticDst = Join-Path $standalone '.next\static'
        New-Item -ItemType Directory -Force -Path $staticDst | Out-Null
        Copy-Item -Recurse -Force (Join-Path $staticSrc '*') $staticDst
    }
    $publicSrc = Join-Path $WebSrc 'public'
    if (Test-Path $publicSrc) {
        $publicDst = Join-Path $standalone 'public'
        New-Item -ItemType Directory -Force -Path $publicDst | Out-Null
        Copy-Item -Recurse -Force (Join-Path $publicSrc '*') $publicDst
    }
    $webDst = Web-Dst
    if (Test-Path $webDst) { Remove-Item -Recurse -Force $webDst }
    Copy-Item -Recurse -Force $standalone $webDst
    Stop-Spin
    Write-Host "  ✓  UI             $webDst"
}

function Install-SoWebTarball([string]$Archive) {
    $webDst = Web-Dst
    if (Test-Path $webDst) { Remove-Item -Recurse -Force $webDst }
    New-Item -ItemType Directory -Force -Path $webDst | Out-Null
    & tar -xzf $Archive -C $webDst
    if ($LASTEXITCODE -ne 0) { Stop-So "extract failed; $Archive may be corrupt" }
    if (-not (Test-Path (Join-Path $webDst 'server.js'))) {
        Stop-So 'so-web.tar.gz is missing server.js (not a standalone UI bundle)'
    }
    Stop-Spin
    Write-Host "  ✓  UI             $webDst"
}

function Add-SoUserPath([string]$Dir) {
    $currentUserPath = [Environment]::GetEnvironmentVariable('Path', 'User')
    if (-not $currentUserPath) { $currentUserPath = '' }
    $pathParts = $currentUserPath -split ';' | Where-Object { $_ -ne '' }
    if ($pathParts -notcontains $Dir) {
        $newPath = if ($currentUserPath) { "$currentUserPath;$Dir" } else { $Dir }
        [Environment]::SetEnvironmentVariable('Path', $newPath, 'User')
        Write-Host "  ✓  PATH           open a new terminal ($Dir)"
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
        Write-Host ''
        Write-Host @'

  ____  _   _ ____  _____ ____   ___  ____  _____ _   _
 / ___|| | | |  _ \| ____|  _ \ / _ \|  _ \| ____| \ | |
 \___ \| | | | |_) |  _| | |_) | | | | |_) |  _| |  \| |
  ___) | |_| |  __/| |___|  _ <| |_| |  __/| |___| |\  |
 |____/ \___/|_|   |_____|_| \_\\___/|_|   |_____|_| \_|

'@
        Start-Spin 'Building the CLI'
        New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
        $Out = Join-Path $InstallDir 'so.exe'
        Push-Location $Root
        try {
            $hasCc = (Get-Command gcc -ErrorAction SilentlyContinue) -or (Get-Command clang -ErrorAction SilentlyContinue)
            if ($hasCc) {
                $env:CGO_ENABLED = '1'
                go build -tags tsnative,sqlite_fts5 -o $Out ./cmd/so
            } else {
                go build -o $Out ./cmd/so
            }
        } finally {
            Pop-Location
        }
        Stop-Spin
        Write-Host "  ✓  CLI            $Out"
        Install-SoWebFromSource (Join-Path $Root 'web')
        Add-SoUserPath $InstallDir
        $env:SUPEROPEN_INSTALLER = '1'
        & $Out install
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

Write-Host ''
Write-Host @'

  ____  _   _ ____  _____ ____   ___  ____  _____ _   _
 / ___|| | | |  _ \| ____|  _ \ / _ \|  _ \| ____| \ | |
 \___ \| | | | |_) |  _| | |_) | | | | |_) |  _| |  \| |
  ___) | |_| |  __/| |___|  _ <| |_| |  __/| |___| |\  |
 |____/ \___/|_|   |_____|_| \_\\___/|_|   |_____|_| \_|

'@
Start-Spin 'Downloading the CLI'

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

    Stop-Spin
    Write-Host "  ✓  CLI            $target"
    Add-SoUserPath $InstallDir

    $webAsset = 'so-web.tar.gz'
    $webUrl = if ($Version -eq 'latest') {
        "https://github.com/$Repo/releases/latest/download/$webAsset"
    } else {
        "https://github.com/$Repo/releases/download/cli-$Version/$webAsset"
    }
    Start-Spin 'Downloading the UI'
    $webTar = Join-Path $tmpDir $webAsset
    Invoke-WebRequest -Uri $webUrl -OutFile $webTar -UseBasicParsing
    Install-SoWebTarball $webTar
    $env:SUPEROPEN_INSTALLER = '1'
    & $target install
}
finally {
    Remove-Item -Recurse -Force $tmpDir -ErrorAction SilentlyContinue
}
