#Requires -Version 5.1
<#
.SYNOPSIS
    Removes xngxltuiplayer for the current user.

.DESCRIPTION
    Removes the player files, desktop/Start Menu shortcuts and the
    "Programs and Features" entry. Music and data are kept unless the user
    explicitly asks to delete them.
#>
$ErrorActionPreference = "Stop"

function Write-Step {
    param([string]$Text)
    Write-Host ""
    Write-Host "==> $Text" -ForegroundColor Cyan
}

function Write-Info {
    param([string]$Text)
    Write-Host "    $Text"
}

function Write-Warn {
    param([string]$Text)
    Write-Host "    $Text" -ForegroundColor Yellow
}

$InstallDir = $PSScriptRoot
$Exe = Join-Path $InstallDir "xngxltuiplayer.exe"

Write-Host "xngxltuiplayer uninstaller" -ForegroundColor Green
Write-Info "Папка: $InstallDir"

$running = Get-Process -Name "xngxltuiplayer" -ErrorAction SilentlyContinue
if ($running) {
    Write-Warn "Плеер запущен. Закройте его и нажмите Enter, чтобы продолжить..."
    [void](Read-Host)
    if (Get-Process -Name "xngxltuiplayer" -ErrorAction SilentlyContinue) {
        Write-Host "Плеер всё ещё запущен. Удаление отменено." -ForegroundColor Red
        exit 1
    }
}

Write-Step "Ярлыки"
$shell = New-Object -ComObject WScript.Shell
$shortcutDirs = @()
try { $shortcutDirs += [string]$shell.SpecialFolders("Desktop") } catch { }
try { $shortcutDirs += [string]$shell.SpecialFolders("Programs") } catch { }
foreach ($dir in ($shortcutDirs | Where-Object { $_ } | Select-Object -Unique)) {
    $lnkPath = Join-Path $dir "xngxltuiplayer.lnk"
    if (-not (Test-Path -LiteralPath $lnkPath)) { continue }
    try {
        $lnk = $shell.CreateShortcut($lnkPath)
        $target = [string]$lnk.TargetPath
        $work = [string]$lnk.WorkingDirectory
        $args = [string]$lnk.Arguments
        $ours = ($target -like "*xngxltuiplayer*") -or ($work -eq $InstallDir) -or ($args -like "*$InstallDir*")
        if ($ours) {
            Remove-Item -LiteralPath $lnkPath -Force
            Write-Info "Удалён ярлык: $lnkPath"
        } else {
            Write-Info "Оставлен чужой ярлык: $lnkPath -> $target"
        }
    } catch {
        Write-Warn "Не удалось проверить ярлык $lnkPath`: $($_.Exception.Message)"
    }
}

Write-Step "Запись в списке программ"
$uninstallKey = "HKCU:\Software\Microsoft\Windows\CurrentVersion\Uninstall\xngxltuiplayer"
if (Test-Path -LiteralPath $uninstallKey) {
    Remove-Item -LiteralPath $uninstallKey -Recurse -Force
    Write-Info "Запись удалена"
} else {
    Write-Info "Записи нет"
}

$deleteData = $false
Write-Host ""
Write-Host "Удалить music\ и data\ (музыка, плейлисты, сессия, настройки)?" -ForegroundColor Yellow
Write-Host "По умолчанию они сохраняются. Введите y, чтобы удалить их тоже."
$answer = Read-Host "Удалить данные? [y/N]"
if ($answer -match '^(y|yes|д|да)$') {
    $deleteData = $true
}

Write-Step "Файлы программы"
$files = @(
    "xngxltuiplayer.exe",
    "xngxltuiplayer.exe.old",
    "README.md",
    "LICENSE",
    "THIRD_PARTY_NOTICES.md",
    "discord.example.json",
    "uninstall_xngxltuiplayer.ps1",
    "uninstall_xngxltuiplayer.bat"
)
foreach ($name in $files) {
    $path = Join-Path $InstallDir $name
    if (Test-Path -LiteralPath $path) {
        try {
            Remove-Item -LiteralPath $path -Force
            Write-Info "Удалён: $name"
        } catch {
            Write-Warn "Не удалось удалить $name`: $($_.Exception.Message)"
        }
    }
}

$terminalDir = Join-Path $InstallDir "terminal"
if (Test-Path -LiteralPath $terminalDir) {
    try {
        Remove-Item -LiteralPath $terminalDir -Recurse -Force
        Write-Info "Удалён портативный Windows Terminal"
    } catch {
        Write-Warn "Не удалось удалить terminal\`: $($_.Exception.Message). Закройте Windows Terminal и повторите."
    }
}

if ($deleteData) {
    Write-Step "Данные"
    foreach ($name in @("music", "data")) {
        $path = Join-Path $InstallDir $name
        if (Test-Path -LiteralPath $path) {
            Remove-Item -LiteralPath $path -Recurse -Force
            Write-Info "Удалена папка: $name"
        }
    }
} else {
    Write-Info "music\ и data\ сохранены."
}

Write-Host ""
Write-Host "Готово." -ForegroundColor Green
Write-Info "Если папка пуста, её можно удалить вручную: $InstallDir"
