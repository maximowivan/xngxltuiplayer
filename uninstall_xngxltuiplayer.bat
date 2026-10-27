@echo off
setlocal
title xngxltuiplayer uninstaller
chcp 65001 >nul
powershell -NoLogo -NoProfile -ExecutionPolicy Bypass -File "%~dp0uninstall_xngxltuiplayer.ps1"
set "RC=%ERRORLEVEL%"
echo.
pause
exit /b %RC%
