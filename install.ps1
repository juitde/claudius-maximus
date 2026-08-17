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

.EXAMPLE
    powershell -ExecutionPolicy Bypass -File install.ps1 -Postinstall
#>

param(
    [string]$Version,
    [string]$Dir,
    [switch]$Postinstall
)

$ErrorActionPreference = "Stop"

$Repo = "juitde/claudius-maximus"
$Binary = "claudius-maximus.exe"

function Fail($Message) {
    # $ErrorActionPreference = "Stop" (set above) makes Write-Error terminate
    # the script on its own; no separate exit is needed or reachable.
    Write-Error "install.ps1: $Message"
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
# OSArchitecture, not ProcessArchitecture: the latter reports the bitness of
# the running PowerShell host, not the machine - a 32-bit powershell.exe on a
# 64-bit Windows install would otherwise be misreported as unsupported.
$archRaw = [System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture
switch ($archRaw) {
    "X64" { $goarch = "amd64" }
    default { Fail "unsupported architecture: $archRaw (only amd64 Windows builds are published)" }
}

# --- Default install directory -------------------------------------------------
if (-not $Dir) {
    $Dir = Join-Path $env:LOCALAPPDATA "Programs\claudius-maximus"
}

# A leading ~ is a shell/PowerShell-provider convention, not something plain
# [System.IO.Path] methods understand - expand it by hand the same way
# Resolve-Path would, before treating the result as a plain filesystem path.
if ($Dir -eq '~' -or $Dir.StartsWith('~/') -or $Dir.StartsWith('~\')) {
    $Dir = Join-Path $HOME $Dir.Substring(1).TrimStart('/', '\')
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
# API's low per-IP rate limit. Reads the Location header off a raw HttpClient
# response (AllowAutoRedirect disabled) rather than Invoke-WebRequest's own
# response object, whose shape differs between PowerShell 5.1 and 7 and would
# otherwise couple this script to that internal, version-dependent detail.
if (-not $Version) {
    Add-Type -AssemblyName System.Net.Http
    $handler = New-Object System.Net.Http.HttpClientHandler
    $handler.AllowAutoRedirect = $false
    $client = New-Object System.Net.Http.HttpClient($handler)
    try {
        $response = $client.GetAsync("https://github.com/$Repo/releases/latest").GetAwaiter().GetResult()
    } finally {
        $client.Dispose()
    }
    $location = $response.Headers.Location
    if ($location -and ($location.ToString() -match '/tag/([^/]+)$')) {
        $Version = $Matches[1]
    } else {
        Fail "could not determine the latest release version"
    }
}

$archive = "claudius-maximus_windows_${goarch}.zip"
$baseUrl = "https://github.com/$Repo/releases/download/$Version"

$workDir = Join-Path ([System.IO.Path]::GetTempPath()) ([System.Guid]::NewGuid().ToString())
New-Item -ItemType Directory -Path $workDir | Out-Null

function Get-ReleaseFile($Url, $OutFile, $FailContext) {
    try {
        Invoke-WebRequest -Uri $Url -OutFile $OutFile -UseBasicParsing
    } catch {
        Fail "failed to download $FailContext"
    }
}

try {
    Write-Host "Downloading $archive ($Version)..."
    $archivePath = Join-Path $workDir $archive
    Get-ReleaseFile "$baseUrl/$archive" $archivePath "$archive - does $Version exist for windows/$goarch?"

    $checksumsPath = Join-Path $workDir "checksums.txt"
    Get-ReleaseFile "$baseUrl/checksums.txt" $checksumsPath "checksums.txt"

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

    # --- Postinstall registration ----------------------------------------------
    # Placing the binary and registering it with Claude Code are two different
    # actions with two different blast radii (the second mutates the caller's
    # Claude Code configuration and requires `claude` on PATH) - opt-in via
    # -Postinstall, not automatic. See issue #14.
    if ($Postinstall) {
        Write-Host ""
        Write-Host "Running $installPath install..."
        & $installPath install
        if ($LASTEXITCODE -ne 0) {
            Fail "postinstall failed - $Binary is installed at $installPath, but registering it with Claude Code did not succeed. Run '$installPath install' yourself to retry."
        }
    } else {
        Write-Host ""
        Write-Host "Next: run $installPath install to register $Binary with Claude Code."
    }
}
finally {
    Remove-Item -Recurse -Force $workDir -ErrorAction SilentlyContinue
}
