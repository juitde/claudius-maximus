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

# A relative -Dir would otherwise be created relative to wherever this script
# happens to be invoked from, and neither the final "installed to" message
# nor the PATH check below would mean much against a relative path.
if (-not [System.IO.Path]::IsPathRooted($Dir)) {
    $Dir = [System.IO.Path]::GetFullPath($Dir)
}

# --- Resolve the version --------------------------------------------------------
# Follows the releases/latest redirect rather than calling the GitHub API
# directly, same as install.sh - and for the same reason: the unauthenticated
# API's low per-IP rate limit. Consistent behavior between the two scripts
# matters more here than using Invoke-RestMethod's native JSON parsing.
if (-not $Version) {
    $resp = Invoke-WebRequest -Uri "https://github.com/$Repo/releases/latest" -UseBasicParsing
    # PowerShell 7's BaseResponse (System.Net.Http.HttpResponseMessage) leaves
    # .ResponseUri empty; the resolved URL is at .RequestMessage.RequestUri
    # instead. Windows PowerShell 5.1's BaseResponse (System.Net.HttpWebResponse)
    # has .ResponseUri populated instead, so both are tried.
    $finalUri = $resp.BaseResponse.ResponseUri
    if (-not $finalUri) {
        $finalUri = $resp.BaseResponse.RequestMessage.RequestUri
    }
    if ($finalUri -and ($finalUri.ToString() -match '/tag/([^/]+)$')) {
        $Version = $Matches[1]
    } else {
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
    # Trim trailing backslashes on both sides - Windows treats "C:\tools" and
    # "C:\tools\" as the same directory, but an exact string comparison would not.
    $normalizedDir = $Dir.TrimEnd('\')
    $pathDirs = $env:Path -split ';' | ForEach-Object { $_.TrimEnd('\') }
    if ($pathDirs -notcontains $normalizedDir) {
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
