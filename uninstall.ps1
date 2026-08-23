#Requires -Version 5.1
<#
.SYNOPSIS
    lazyaddons - Uninstall Script for Windows

.DESCRIPTION
    Completely removes lazyaddons from a Windows system, including the binary,
    configuration, and WoW addon data.

.EXAMPLE
    # Run directly:
    irm https://raw.githubusercontent.com/pentsec/lazyaddons/main/uninstall.ps1 | iex

    # Or download and run:
    Invoke-WebRequest -Uri https://raw.githubusercontent.com/pentsec/lazyaddons/main/uninstall.ps1 -OutFile uninstall.ps1
    .\uninstall.ps1

    # Skip confirmation prompt:
    .\uninstall.ps1 -Force

    # Keep configuration:
    .\uninstall.ps1 -KeepConfig

    # Keep WoW addon data:
    .\uninstall.ps1 -KeepData
#>

[CmdletBinding()]
param(
    [switch]$Force,
    [switch]$KeepConfig,
    [switch]$KeepData
)

$ErrorActionPreference = "Stop"

# Ensure UTF-8 output so Unicode characters render correctly on all terminals.
$null = & chcp 65001 2>$null
try { [Console]::OutputEncoding = [System.Text.Encoding]::UTF8 } catch {}

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
    Write-Host "  Uninstaller" -ForegroundColor DarkGray
    Write-Host ""
}

# ============================================================================
# Detect installed components
# ============================================================================

$script:BinaryPaths = @()
$script:ConfigDir = ""
$script:ConfigFile = ""
$script:WowPaths = @()

function Find-Binary {
    Write-Step "Detecting binary"

    # Default install location
    $defaultDir = Join-Path $env:LOCALAPPDATA "Programs\lazyaddons"
    $defaultExe = Join-Path $defaultDir "$BINARY_NAME.exe"

    if (Test-Path $defaultExe) {
        $script:BinaryPaths += $defaultExe
        Write-Success "Found: $defaultExe"
        return
    }

    # Check PATH
    $pathDirs = $env:PATH -split ';' | Where-Object { $_ -ne '' }
    foreach ($dir in $pathDirs) {
        $exe = Join-Path $dir "$BINARY_NAME.exe"
        if (Test-Path $exe) {
            $script:BinaryPaths += $exe
            Write-Success "Found in PATH: $exe"
            return
        }
    }

    # Check common alternative locations
    $candidates = @(
        (Join-Path $env:LOCALAPPDATA "Programs\lazyaddons\$BINARY_NAME.exe"),
        (Join-Path $env:PROGRAMFILES "lazyaddons\$BINARY_NAME.exe"),
        (Join-Path "${env:PROGRAMFILES(x86)}" "lazyaddons\$BINARY_NAME.exe")
    )

    foreach ($path in $candidates) {
        if (Test-Path $path) {
            $script:BinaryPaths += $path
            Write-Success "Found: $path"
            return
        }
    }

    Write-Warn "Binary not found — may already be removed"
}

function Find-Config {
    Write-Step "Detecting configuration"

    $candidate = Join-Path $env:APPDATA "lazyaddons"

    if (Test-Path $candidate) {
        $script:ConfigDir = $candidate
        $script:ConfigFile = Join-Path $candidate "config.json"
        Write-Success "Config directory: $candidate"
    } else {
        Write-Warn "Config directory not found — may already be removed"
    }
}

function Find-WowData {
    Write-Step "Detecting WoW addon data"

    if (-not $script:ConfigFile -or -not (Test-Path $script:ConfigFile)) {
        Write-Warn "No config file found — cannot detect WoW paths"
        return
    }

    try {
        $config = Get-Content $script:ConfigFile -Raw | ConvertFrom-Json
    } catch {
        Write-Warn "Could not parse config.json — skipping WoW data detection"
        return
    }

    $paths = @()

    # v2 config: profiles[].wow_path
    if ($config.profiles) {
        foreach ($profile in $config.profiles) {
            if ($profile.wow_path) {
                $paths += $profile.wow_path
            }
        }
    }

    # v1 config: root-level wow_path
    if ($config.wow_path) {
        $paths += $config.wow_path
    }

    if ($paths.Count -eq 0) {
        Write-Warn "No WoW paths found in config"
        return
    }

    $seen = @{}
    foreach ($wp in $paths) {
        if ($seen.ContainsKey($wp)) { continue }
        $seen[$wp] = $true

        $lazyDir = Join-Path $wp ".lazyaddons"
        $backupDir = Join-Path $wp ".backup"

        if ((Test-Path $lazyDir) -or (Test-Path $backupDir)) {
            $script:WowPaths += $wp
            Write-Success "WoW AddOns: $wp"
        }
    }

    if ($script:WowPaths.Count -eq 0) {
        Write-Warn "No .lazyaddons or .backup directories found in WoW paths"
    }
}

# ============================================================================
# Display removal plan
# ============================================================================

function Show-RemovalPlan {
    Write-Step "Removal plan"

    Write-Host ""
    Write-Host "The following will be removed:" -ForegroundColor White
    Write-Host ""

    $hasItems = $false

    if ($script:BinaryPaths.Count -gt 0) {
        $hasItems = $true
        Write-Host "  Binary:" -ForegroundColor Cyan
        foreach ($path in $script:BinaryPaths) {
            Write-Host "    - $path"
        }
    }

    if ($script:ConfigDir -and -not $KeepConfig) {
        $hasItems = $true
        Write-Host "  Configuration:" -ForegroundColor Cyan
        Write-Host "    - $($script:ConfigDir)"
    }

    if ($script:WowPaths.Count -gt 0 -and -not $KeepData) {
        $hasItems = $true
        Write-Host "  WoW addon data:" -ForegroundColor Cyan
        foreach ($wp in $script:WowPaths) {
            Write-Host "    - $wp\.lazyaddons\"
            Write-Host "    - $wp\.backup\"
        }
    }

    if ($KeepConfig) {
        Write-Host "  Configuration preserved (-KeepConfig)" -ForegroundColor DarkGray
    }

    if ($KeepData) {
        Write-Host "  WoW addon data preserved (-KeepData)" -ForegroundColor DarkGray
    }

    if (-not $hasItems) {
        Write-Host "  (nothing to remove)" -ForegroundColor DarkGray
        Write-Host ""
        Write-Success "lazyaddons appears to be already uninstalled"
        exit 0
    }

    Write-Host ""
    Write-Host "  Addon folders inside AddOns\ (Details, DragonUI, etc.) will NOT be removed." -ForegroundColor Yellow
    Write-Host "  lazyaddons never modifies addon content — it only manages git repos." -ForegroundColor DarkGray
    Write-Host ""
}

# ============================================================================
# Confirm
# ============================================================================

function Confirm-Removal {
    if ($Force) { return }

    $answer = Read-Host "Proceed with uninstall? (y/N)"
    if ($answer -notmatch '^[yY]$') {
        Write-Host ""
        Write-Info "Uninstall cancelled"
        exit 0
    }
    Write-Host ""
}

# ============================================================================
# Remove binary
# ============================================================================

function Remove-Binary {
    Write-Step "Removing binary"

    if ($script:BinaryPaths.Count -eq 0) {
        Write-Warn "No binary found — skipping"
        return
    }

    foreach ($path in $script:BinaryPaths) {
        if (Test-Path $path) {
            Write-Info "Removing $path..."
            try {
                Remove-Item -Path $path -Force
                Write-Success "Removed $path"
            } catch {
                Write-Err "Failed to remove $path — $($_.Exception.Message)"
            }
        }
    }
}

# ============================================================================
# Remove configuration
# ============================================================================

function Remove-Config {
    Write-Step "Removing configuration"

    if ($KeepConfig) {
        Write-Info "Skipping — -KeepConfig specified"
        return
    }

    if (-not $script:ConfigDir) {
        Write-Warn "No config directory found — skipping"
        return
    }

    if (-not (Test-Path $script:ConfigDir)) {
        Write-Warn "Config directory does not exist — skipping"
        return
    }

    Write-Info "Removing $($script:ConfigDir)..."
    try {
        Remove-Item -Path $script:ConfigDir -Recurse -Force
        Write-Success "Removed $($script:ConfigDir)"
    } catch {
        Write-Err "Failed to remove $($script:ConfigDir) — $($_.Exception.Message)"
    }
}

# ============================================================================
# Remove WoW addon data
# ============================================================================

function Remove-WowData {
    Write-Step "Removing WoW addon data"

    if ($KeepData) {
        Write-Info "Skipping — -KeepData specified"
        return
    }

    if ($script:WowPaths.Count -eq 0) {
        Write-Warn "No WoW addon data found — skipping"
        return
    }

    foreach ($wp in $script:WowPaths) {
        $lazyDir = Join-Path $wp ".lazyaddons"
        $backupDir = Join-Path $wp ".backup"

        if (Test-Path $lazyDir) {
            Write-Info "Removing $lazyDir..."
            try {
                Remove-Item -Path $lazyDir -Recurse -Force
                Write-Success "Removed $lazyDir"
            } catch {
                Write-Err "Failed to remove $lazyDir — $($_.Exception.Message)"
            }
        }

        if (Test-Path $backupDir) {
            Write-Info "Removing $backupDir..."
            try {
                Remove-Item -Path $backupDir -Recurse -Force
                Write-Success "Removed $backupDir"
            } catch {
                Write-Err "Failed to remove $backupDir — $($_.Exception.Message)"
            }
        }
    }
}

# ============================================================================
# Clean PATH entries
# ============================================================================

function Remove-PathEntries {
    Write-Step "Checking PATH entries"

    $userPath = [Environment]::GetEnvironmentVariable("PATH", "User")
    if (-not $userPath) {
        Write-Info "No User PATH set — nothing to clean"
        return
    }

    $pathEntries = $userPath -split ';' | Where-Object { $_ -ne '' }
    $filteredEntries = $pathEntries | Where-Object { $_ -notmatch 'lazyaddons' }

    if ($pathEntries.Count -eq $filteredEntries.Count) {
        Write-Info "No lazyaddons entries found in User PATH"
        return
    }

    $removedCount = $pathEntries.Count - $filteredEntries.Count
    Write-Info "Found $removedCount lazyaddons PATH entry/entries"

    if (-not $Force) {
        $answer = Read-Host "Remove lazyaddons entries from User PATH? (y/N)"
        if ($answer -notmatch '^[yY]$') {
            Write-Info "Skipping PATH cleanup"
            return
        }
    }

    $newPath = $filteredEntries -join ';'
    [Environment]::SetEnvironmentVariable("PATH", $newPath, "User")
    $env:PATH = ($env:PATH -split ';' | Where-Object { $_ -ne '' -and $_ -notmatch 'lazyaddons' }) -join ';'
    Write-Success "Cleaned User PATH (removed $removedCount entry/entries)"
}

# ============================================================================
# Verify uninstall
# ============================================================================

function Test-Uninstall {
    Write-Step "Verifying uninstall"

    $issues = 0

    # Check binary
    foreach ($path in $script:BinaryPaths) {
        if (Test-Path $path) {
            Write-Err "Binary still exists: $path"
            $issues++
        }
    }

    # Check config
    if (-not $KeepConfig -and $script:ConfigDir -and (Test-Path $script:ConfigDir)) {
        Write-Err "Config directory still exists: $($script:ConfigDir)"
        $issues++
    }

    # Check WoW data
    if (-not $KeepData) {
        foreach ($wp in $script:WowPaths) {
            $lazyDir = Join-Path $wp ".lazyaddons"
            $backupDir = Join-Path $wp ".backup"
            if (Test-Path $lazyDir) {
                Write-Err ".lazyaddons still exists: $lazyDir"
                $issues++
            }
            if (Test-Path $backupDir) {
                Write-Err ".backup still exists: $backupDir"
                $issues++
            }
        }
    }

    if ($issues -eq 0) {
        Write-Success "All components removed successfully"
    } else {
        Write-Warn "$issues item(s) could not be removed — check permissions"
    }
}

# ============================================================================
# Print summary
# ============================================================================

function Show-Summary {
    Write-Host ""
    Write-Host "Uninstall complete!" -ForegroundColor Green
    Write-Host ""
    Write-Host "Notes:" -ForegroundColor White
    Write-Host "  - Addon folders in AddOns\ were " -NoNewline
    Write-Host "not removed" -ForegroundColor Cyan -NoNewline
    Write-Host " (managed independently)"
    Write-Host "  - Restart your terminal to clear the cached binary path"
    if ($script:ConfigDir -and $KeepConfig) {
        Write-Host "  - Config preserved at: $($script:ConfigDir)" -ForegroundColor DarkGray
    }
    Write-Host ""
    Write-Host "To reinstall: https://github.com/pentsec/lazyaddons" -ForegroundColor DarkGray
    Write-Host ""
}

# ============================================================================
# Main
# ============================================================================

function Main {
    Show-Banner

    Find-Binary
    Find-Config
    Find-WowData
    Show-RemovalPlan
    Confirm-Removal

    Remove-Binary
    Remove-Config
    Remove-WowData
    Remove-PathEntries
    Test-Uninstall
    Show-Summary
}

Main @args
