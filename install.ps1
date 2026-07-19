#Requires -Version 5.1
<#
.SYNOPSIS
    lazyaddons - Install Script for Windows

.DESCRIPTION
    Downloads and installs the lazyaddons binary for Windows from GitHub Releases.

.EXAMPLE
    # Run directly:
    irm https://raw.githubusercontent.com/pentsec/lazyaddons/main/install.ps1 | iex

    # Or download and run:
    Invoke-WebRequest -Uri https://raw.githubusercontent.com/pentsec/lazyaddons/main/install.ps1 -OutFile install.ps1
    .\install.ps1

    # Install a specific version:
    .\install.ps1 -Version v0.1.0

    # Custom install directory:
    .\install.ps1 -InstallDir "C:\Tools\bin"

    # Skip checksum verification (not recommended):
    .\install.ps1 -Insecure
#>

[CmdletBinding()]
param(
    [string]$Version = "",
    [string]$InstallDir = "",
    [switch]$Insecure
)

$ErrorActionPreference = "Stop"

# Ensure UTF-8 output so Unicode characters render correctly on all terminals.
$null = & chcp 65001 2>$null
try { [Console]::OutputEncoding = [System.Text.Encoding]::UTF8 } catch {}

$GITHUB_OWNER = "pentsec"
$GITHUB_REPO = "lazyaddons"
$BINARY_NAME = "lazyaddons"

# ============================================================================
# Logging helpers
# ============================================================================

function Write-Info    { param([string]$Message) Write-Host "[info]    $Message" -ForegroundColor Blue }
function Write-Success { param([string]$Message) Write-Host "[ok]      $Message" -ForegroundColor Green }
function Write-Warn    { param([string]$Message) Write-Host "[warn]    $Message" -ForegroundColor Yellow }
function Write-Err     { param([string]$Message) Write-Host "[error]   $Message" -ForegroundColor Red }
function Write-Step    { param([string]$Message) Write-Host "`n==> $Message" -ForegroundColor Cyan }

function Stop-WithError {
    param([string]$Message)
    Write-Err $Message
    exit 1
}

# ============================================================================
# Banner
# ============================================================================

function Show-Banner {
    Write-Host ""
    Write-Host "                    _                   ___    ____" -ForegroundColor Cyan
    Write-Host "  _     _  _  _ _| |  ___  ___  ___  |   \  |  _ \" -ForegroundColor Cyan
    Write-Host " | |   | || || ' \| | / _ \/ _ \/ __| | |) | | | | |" -ForegroundColor Cyan
    Write-Host " | |___| \/ | || || |/\__/\__ /\__ \ |  __/| |_| |" -ForegroundColor Cyan
    Write-Host " |_____|\__/|_._/|_| /\__,_|___/|___/ |_|   |____/" -ForegroundColor Cyan
    Write-Host "               |__/" -ForegroundColor Cyan
    Write-Host ""
    Write-Host "  WoW Addon Manager - Git-powered" -ForegroundColor DarkGray
    Write-Host ""
}

# ============================================================================
# Platform detection
# ============================================================================

function Get-Platform {
    Write-Step "Detecting platform"

    $arch = if ([Environment]::Is64BitOperatingSystem) {
        if ($env:PROCESSOR_ARCHITECTURE -eq "ARM64") { "arm64" } else { "amd64" }
    } else {
        Stop-WithError "32-bit Windows is not supported."
    }

    Write-Success "Platform: Windows ($arch)"
    return $arch
}

# ============================================================================
# Prerequisites
# ============================================================================

function Test-Prerequisites {
    Write-Step "Checking prerequisites"

    $missing = @()
    if (-not (Get-Command "curl" -ErrorAction SilentlyContinue)) { $missing += "curl" }

    if ($missing.Count -gt 0) {
        Stop-WithError "Missing required tools: $($missing -join ', '). Please install them and try again."
    }

    Write-Success "curl is available"
}

# ============================================================================
# Version resolution
# ============================================================================

function Get-LatestVersion {
    Write-Info "Fetching latest release from GitHub..."

    $url = "https://api.github.com/repos/$GITHUB_OWNER/$GITHUB_REPO/releases/latest"

    try {
        $response = Invoke-RestMethod -Uri $url -Headers @{ "User-Agent" = "lazyaddons-installer" }
    } catch {
        Stop-WithError "Failed to fetch latest release. Rate limited? Try again later."
    }

    $version = $response.tag_name
    if (-not $version) {
        Stop-WithError "Could not determine latest version from GitHub API response"
    }

    Write-Success "Latest version: $version"
    return $version
}

# ============================================================================
# Install binary
# ============================================================================

function Install-Binary {
    param([string]$Arch)

    Write-Step "Installing pre-built binary"

    # Resolve version
    if ($Version) {
        $versionTag = $Version
    } else {
        $versionTag = Get-LatestVersion
    }

    $versionNumber = $versionTag.TrimStart("v")
    $archiveName = "${BINARY_NAME}_${versionNumber}_windows_${Arch}.zip"
    $downloadUrl = "https://github.com/$GITHUB_OWNER/$GITHUB_REPO/releases/download/$versionTag/$archiveName"
    $checksumsUrl = "https://github.com/$GITHUB_OWNER/$GITHUB_REPO/releases/download/$versionTag/checksums.txt"

    $tmpDir = Join-Path $env:TEMP "lazyaddons-install-$(Get-Random)"
    New-Item -ItemType Directory -Path $tmpDir -Force | Out-Null

    try {
        # Download archive
        Write-Info "Downloading $archiveName..."
        $archivePath = Join-Path $tmpDir $archiveName
        Invoke-WebRequest -Uri $downloadUrl -OutFile $archivePath -UseBasicParsing

        $fileSize = (Get-Item $archivePath).Length
        if ($fileSize -lt 1000) {
            Stop-WithError ("Downloaded file is suspiciously small ({0} bytes). Archive may not exist for this platform." -f $fileSize)
        }
        Write-Success ("Downloaded {0} ({1} bytes)" -f $archiveName, $fileSize)

        # Verify checksum
        if (-not $Insecure) {
            Write-Info "Verifying checksum..."
            try {
                $checksumsPath = Join-Path $tmpDir "checksums.txt"
                Invoke-WebRequest -Uri $checksumsUrl -OutFile $checksumsPath -UseBasicParsing

                $checksums = Get-Content $checksumsPath
                $expectedLine = $checksums | Where-Object { $_ -match $archiveName }
                if ($expectedLine) {
                    $expectedChecksum = (($expectedLine -split "\s+")[0]).ToLowerInvariant()

                    if (Get-Command Get-FileHash -ErrorAction SilentlyContinue) {
                        $actualChecksum = (Get-FileHash -Path $archivePath -Algorithm SHA256).Hash.ToLowerInvariant()
                    } else {
                        $sha256 = [System.Security.Cryptography.SHA256]::Create()
                        $fileStream = [System.IO.File]::OpenRead($archivePath)
                        try {
                            $hashBytes = $sha256.ComputeHash($fileStream)
                            $actualChecksum = [System.BitConverter]::ToString($hashBytes).Replace("-", "").ToLowerInvariant()
                        } finally {
                            $fileStream.Close()
                            $sha256.Dispose()
                        }
                    }

                    if ($actualChecksum -ne $expectedChecksum) {
                        Stop-WithError "Checksum mismatch!`n  Expected: $expectedChecksum`n  Got:      $actualChecksum"
                    }
                    Write-Success "Checksum verified"
                } else {
                    Stop-WithError "Archive '$archiveName' not found in checksums.txt. Refusing to install unverified binary.`nUse -Insecure to skip (not recommended)."
                }
            } catch {
                $reason = $_.Exception.Message
                Stop-WithError ("Could not download checksums.txt from: {0}`nError: {1}`nRefusing to install without integrity verification.`nUse -Insecure to skip (not recommended)." -f $checksumsUrl, $reason)
            }
        } else {
            Write-Warn "Checksum verification skipped (-Insecure)"
        }

        # Extract binary
        Write-Info "Extracting $BINARY_NAME..."
        Expand-Archive -Path $archivePath -DestinationPath $tmpDir -Force

        $binaryPath = Join-Path $tmpDir "$BINARY_NAME.exe"
        if (-not (Test-Path $binaryPath)) {
            Stop-WithError "Binary '$BINARY_NAME.exe' not found in archive"
        }

        # Determine install directory
        $installDir = $InstallDir
        if (-not $installDir) {
            $installDir = Join-Path $env:LOCALAPPDATA "Programs\lazyaddons"
        }

        if (-not (Test-Path $installDir)) {
            New-Item -ItemType Directory -Path $installDir -Force | Out-Null
        }

        # Install binary
        $destPath = Join-Path $installDir "$BINARY_NAME.exe"
        Write-Info "Installing to $destPath..."
        Copy-Item -Path $binaryPath -Destination $destPath -Force

        Write-Success "Installed $BINARY_NAME to $destPath"

        # Persist install dir to the User PATH if not already present.
        $userPath = [Environment]::GetEnvironmentVariable("PATH", "User")
        $pathEntries = if ($userPath) { $userPath -split ';' | Where-Object { $_ -ne '' } } else { @() }
        $alreadyPresent = $pathEntries | Where-Object { $_.TrimEnd('\') -ieq $installDir.TrimEnd('\') }
        if (-not $alreadyPresent) {
            $newUserPath = if ($userPath) { "$userPath;$installDir" } else { $installDir }
            [Environment]::SetEnvironmentVariable("PATH", $newUserPath, "User")
            Write-Success "Added $installDir to your PATH (takes effect in new shells)"
        }

        # Also update the current session's PATH so Test-Installation can find the binary.
        $sessionEntries = $env:PATH -split ';' | Where-Object { $_ -ne '' }
        $sessionPresent = $sessionEntries | Where-Object { $_.TrimEnd('\') -ieq $installDir.TrimEnd('\') }
        if (-not $sessionPresent) {
            $env:PATH = "$env:PATH;$installDir"
        }
    } finally {
        Remove-Item -Path $tmpDir -Recurse -Force -ErrorAction SilentlyContinue
    }
}

# ============================================================================
# Verify installation
# ============================================================================

function Test-Installation {
    Write-Step "Verifying installation"

    $locations = @(
        (Join-Path $env:LOCALAPPDATA "Programs\lazyaddons\$BINARY_NAME.exe")
    )

    # Also check custom install dir if provided
    if ($InstallDir) {
        $locations = @((Join-Path $InstallDir "$BINARY_NAME.exe")) + $locations
    }

    foreach ($loc in $locations) {
        if (-not ($loc -and (Test-Path $loc))) { continue }

        $versionOutput = & $loc --version 2>&1
        Write-Success "$BINARY_NAME installed at $loc`: $versionOutput"

        # Inform the user if the binary is not yet reachable by name.
        $userPath = [Environment]::GetEnvironmentVariable("PATH", "User")
        $binaryDir = [System.IO.Path]::GetDirectoryName($loc)
        if ($userPath -notlike "*$binaryDir*") {
            Write-Warn "Binary location is not in your PATH. Open a new shell or add it manually."
        }
        return
    }

    Write-Warn "Could not verify installation. You may need to restart your terminal."
}

# ============================================================================
# Next steps
# ============================================================================

function Show-NextSteps {
    Write-Host ""
    Write-Host "Installation complete!" -ForegroundColor Green
    Write-Host ""
    Write-Host "Next steps:" -ForegroundColor White
    Write-Host "  1. Run '$BINARY_NAME' to set up your WoW addon manager" -ForegroundColor Cyan
    Write-Host "  2. Follow the interactive prompts to configure your WoW path" -ForegroundColor Cyan
    Write-Host ""
    Write-Host "For help: $BINARY_NAME --help" -ForegroundColor DarkGray
    Write-Host "Docs:     https://github.com/$GITHUB_OWNER/$GITHUB_REPO" -ForegroundColor DarkGray
    Write-Host ""
}

# ============================================================================
# Main
# ============================================================================

function Main {
    Show-Banner

    $arch = Get-Platform
    Test-Prerequisites
    Install-Binary -Arch $arch
    Test-Installation
    Show-NextSteps
}

Main @args
