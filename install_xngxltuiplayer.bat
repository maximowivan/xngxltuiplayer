@echo off
setlocal
title xngxltuiplayer installer
chcp 65001 >nul

set "PS1=%~dp0install_xngxltuiplayer.ps1"
if not exist "%PS1%" (
    echo Installer files are incomplete: install_xngxltuiplayer.ps1 not found.
    echo Download the release zip and extract all files first.
    pause
    exit /b 1
)

echo Starting xngxltuiplayer installer...
echo.
powershell -NoLogo -NoProfile -ExecutionPolicy Bypass -File "%PS1%" %*
set "RC=%ERRORLEVEL%"
echo.
if "%RC%"=="0" (
    echo Installation finished.
) else (
    echo Installation failed with code %RC%.
)
pause
exit /b %RC%
