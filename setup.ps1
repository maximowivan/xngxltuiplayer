#Requires -Version 5.1
<#
.SYNOPSIS
    Installs yt-dlp and ffmpeg with winget when needed, then builds xngxltuiplayer.

.NOTES
    winget must be available. Run this script from a normal user terminal.
#>
$ErrorActionPreference = "Stop"
Set-Location -LiteralPath $PSScriptRoot

function Test-Tool {
    param([string]$Name)
    return [bool](Get-Command $Name -ErrorAction SilentlyContinue)
}

function Install-Tool {
    param(
        [string]$Name,
        [string]$PackageId
    )
    if (Test-Tool $Name) {
        Write-Host "$Name is already installed."
        return
    }
    if (-not (Get-Command winget -ErrorAction SilentlyContinue)) {
        throw "$Name is missing and winget is not available. Install $Name manually and reopen the terminal."
    }
    Write-Host "Installing $Name ($PackageId) ..."
    & winget install --id $PackageId --exact --accept-source-agreements --accept-package-agreements
    if ($LASTEXITCODE -ne 0) {
        throw "winget failed to install $Name (exit code $LASTEXITCODE)."
    }
}

Install-Tool "yt-dlp" "yt-dlp.yt-dlp"
Install-Tool "ffmpeg" "Gyan.FFmpeg"

# Make freshly installed tools visible in this session as well.
$env:Path = [Environment]::GetEnvironmentVariable("Path", "Machine") + ";" +
    [Environment]::GetEnvironmentVariable("Path", "User")

Write-Host ""
& (Join-Path $PSScriptRoot "build.ps1")
if ($LASTEXITCODE -ne 0) {
    exit $LASTEXITCODE
}

Write-Host ""
Write-Host "Setup complete. If a new terminal still cannot find the tools, reopen it once more."
