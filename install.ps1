<#
.SYNOPSIS
    Installs claudius-maximus on Windows.

.DESCRIPTION
    Detects the architecture, downloads the matching release archive,
    verifies it against that release's checksums.txt, and installs the
    binary. See RELEASING.md and README.md for the reasoning behind the
    pinned commit this script is meant to be fetched at, rather than main
    or a tag.

.EXAMPLE
    powershell -ExecutionPolicy Bypass -File install.ps1

.EXAMPLE
    powershell -ExecutionPolicy Bypass -File install.ps1 -Version v0.1.0 -Dir C:\tools
#>

param(
    [string]$Version,
    [string]$Dir
)

$ErrorActionPreference = "Stop"

$Repo = "juitde/claudius-maximus"
$Binary = "claudius-maximus.exe"

function Fail($Message) {
    Write-Error "install.ps1: $Message"
    exit 1
}

# --- Refuse off Windows, on purpose -----------------------------------------
# PowerShell Core also runs on Linux/macOS; this script only supports native
# Windows, on purpose - use install.sh there instead. $IsWindows only exists
# from PowerShell 6 onward; Windows PowerShell 5.1 has no other platform to
# run on, so its absence itself means Windows.
$onWindows = $true
if (Get-Variable -Name IsWindows -ErrorAction SilentlyContinue) {
    $onWindows = $IsWindows
}
if (-not $onWindows) {
    Fail "this script only supports Windows. Use install.sh on macOS/Linux instead."
}

# --- Architecture detection ---------------------------------------------------
$archRaw = [System.Runtime.InteropServices.RuntimeInformation]::ProcessArchitecture
switch ($archRaw) {
    "X64" { $goarch = "amd64" }
    default { Fail "unsupported architecture: $archRaw (only amd64 Windows builds are published)" }
}

# --- Default install directory -------------------------------------------------
if (-not $Dir) {
    $Dir = Join-Path $env:LOCALAPPDATA "Programs\claudius-maximus"
}

# --- Resolve the version --------------------------------------------------------
# Invoke-RestMethod parses JSON natively, so - unlike install.sh, which
# avoids a jq dependency by following the releases/latest redirect instead -
# there is no reason here not to just ask the GitHub API directly.
if (-not $Version) {
    $release = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases/latest" -UseBasicParsing
    $Version = $release.tag_name
    if (-not $Version) {
        Fail "could not determine the latest release version"
    }
}

$archive = "claudius-maximus_windows_${goarch}.zip"
$baseUrl = "https://github.com/$Repo/releases/download/$Version"

$workDir = Join-Path ([System.IO.Path]::GetTempPath()) ([System.Guid]::NewGuid().ToString())
New-Item -ItemType Directory -Path $workDir | Out-Null

try {
    Write-Host "Downloading $archive ($Version)..."
    $archivePath = Join-Path $workDir $archive
    try {
        Invoke-WebRequest -Uri "$baseUrl/$archive" -OutFile $archivePath -UseBasicParsing
    } catch {
        Fail "failed to download $archive - does $Version exist for windows/$goarch?"
    }

    $checksumsPath = Join-Path $workDir "checksums.txt"
    try {
        Invoke-WebRequest -Uri "$baseUrl/checksums.txt" -OutFile $checksumsPath -UseBasicParsing
    } catch {
        Fail "failed to download checksums.txt"
    }

    Write-Host "Verifying checksum..."
    $pattern = "\s" + [regex]::Escape($archive) + '$'
    $checksumLine = Select-String -Path $checksumsPath -Pattern $pattern | Select-Object -First 1
    if (-not $checksumLine) {
        Fail "no checksum entry found for $archive in checksums.txt"
    }
    $expected = ($checksumLine.Line -split '\s+')[0].ToLower()
    $actual = (Get-FileHash -Path $archivePath -Algorithm SHA256).Hash.ToLower()
    if ($expected -ne $actual) {
        Fail "checksum mismatch for $archive (expected $expected, got $actual) - refusing to install"
    }

    Write-Host "Extracting..."
    Expand-Archive -Path $archivePath -DestinationPath $workDir -Force

    New-Item -ItemType Directory -Path $Dir -Force | Out-Null
    $installPath = Join-Path $Dir $Binary
    Move-Item -Path (Join-Path $workDir $Binary) -Destination $installPath -Force

    Write-Host "Installed $Binary $Version to $installPath"

    # --- PATH check -----------------------------------------------------------
    $pathDirs = $env:Path -split ';'
    if ($pathDirs -notcontains $Dir) {
        Write-Host ""
        Write-Host "Warning: $Dir is not in your PATH."
        Write-Host "Add it for your user account with:"
        Write-Host ""
        Write-Host "  [Environment]::SetEnvironmentVariable('Path', `$env:Path + ';$Dir', 'User')"
        Write-Host ""
        Write-Host "Then restart your terminal."
        Write-Host ""
    }
}
finally {
    Remove-Item -Recurse -Force $workDir -ErrorAction SilentlyContinue
}
