#Requires -Version 5.1
<#
.SYNOPSIS
    Builds xngxltuiplayer.exe and optionally runs the environment check.

.EXAMPLE
    .\build.ps1
    .\build.ps1 -Output xngxltuiplayer-dev.exe -NoCheck
#>
param(
    [string]$Output = "xngxltuiplayer.exe",
    [switch]$NoCheck
)

$ErrorActionPreference = "Stop"
Set-Location -LiteralPath $PSScriptRoot

$go = Get-Command go -ErrorAction SilentlyContinue
if (-not $go) {
    throw "Go was not found on PATH. Install Go 1.27+ from https://go.dev/dl/ and reopen the terminal."
}

Write-Host "Building $Output ..."
& go build -trimpath -ldflags "-s -w" -o $Output .
if ($LASTEXITCODE -ne 0) {
    throw "go build failed with exit code $LASTEXITCODE"
}

$built = Join-Path $PSScriptRoot $Output
if (-not (Test-Path -LiteralPath $built)) {
    throw "Build finished but $Output was not created."
}

if (-not $NoCheck) {
    Write-Host ""
    & $built --check
    if ($LASTEXITCODE -ne 0) {
        Write-Warning "The build succeeded, but the environment check reported missing components."
    }
}

Write-Host ""
Write-Host "Done: $built"
