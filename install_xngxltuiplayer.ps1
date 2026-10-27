#Requires -Version 5.1
<#
.SYNOPSIS
    Installs xngxltuiplayer for the current user without administrator rights.

.DESCRIPTION
    If xngxltuiplayer.exe is next to this script, the files are installed from
    that folder (the portable release zip). Otherwise the latest Windows release
    is downloaded from GitHub, its SHA256 is verified and the player is
    installed. Existing music\ and data\ folders are never deleted.

.PARAMETER InstallDir
    Target folder. Default: %LOCALAPPDATA%\Programs\xngxltuiplayer

.PARAMETER NoShortcut
    Do not create desktop and Start Menu shortcuts.

.PARAMETER NoDeps
    Do not try to install yt-dlp/ffmpeg with winget.

.PARAMETER SourceDir
    Install from this folder instead of the script folder or GitHub.
#>
param(
    [string]$InstallDir = "",
    [switch]$NoShortcut,
    [switch]$NoDeps,
    [string]$SourceDir = ""
)

$ErrorActionPreference = "Stop"
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
$ProgressPreference = "SilentlyContinue"

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

function Fail {
    param([string]$Text)
    Write-Host ""
    Write-Host "ОШИБКА: $Text" -ForegroundColor Red
    exit 1
}

function Refresh-Path {
    $machine = [Environment]::GetEnvironmentVariable("Path", "Machine")
    $user = [Environment]::GetEnvironmentVariable("Path", "User")
    $parts = @()
    if ($machine) { $parts += $machine }
    if ($user) { $parts += $user }
    $env:Path = ($parts -join ";")

    $links = Join-Path $env:LOCALAPPDATA "Microsoft\WinGet\Links"
    if (Test-Path -LiteralPath $links) {
        $env:Path += ";" + $links
    }

    $packages = Join-Path $env:LOCALAPPDATA "Microsoft\WinGet\Packages"
    if (Test-Path -LiteralPath $packages) {
        foreach ($tool in @("ffmpeg.exe", "ffprobe.exe")) {
            $found = Get-ChildItem -LiteralPath $packages -Recurse -Filter $tool -ErrorAction SilentlyContinue | Select-Object -First 1
            if ($found) {
                $dir = $found.DirectoryName
                if (($env:Path -split ";") -notcontains $dir) {
                    $env:Path += ";" + $dir
                }
            }
        }
    }
}

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
        Write-Info "$Name уже установлен"
        return $true
    }
    if (-not (Get-Command winget -ErrorAction SilentlyContinue)) {
        Write-Warn "winget не найден — установите $Name вручную"
        return $false
    }

    $wingetArgs = @(
        "install", "--id", $PackageId, "--exact",
        "--accept-source-agreements", "--accept-package-agreements", "--silent"
    )

    Write-Step "Установка $Name ($PackageId)"
    $userArgs = $wingetArgs + @("--scope", "user")
    & winget @userArgs
    Refresh-Path
    if (Test-Tool $Name) {
        Write-Info "$Name установлен"
        return $true
    }

    Write-Info "Повторная попытка без --scope user"
    & winget @wingetArgs
    Refresh-Path
    if (Test-Tool $Name) {
        Write-Info "$Name установлен"
        return $true
    }

    Write-Warn "$Name не найден после установки; возможно, нужен новый терминал"
    return $false
}

function Find-WindowsTerminal {
    $cmd = Get-Command "wt.exe" -ErrorAction SilentlyContinue
    if ($cmd) { return $cmd.Source }
    $alias = Join-Path $env:LOCALAPPDATA "Microsoft\WindowsApps\wt.exe"
    if (Test-Path -LiteralPath $alias) { return $alias }
    return $null
}

function Register-UninstallEntry {
    param(
        [string]$InstallDir,
        [string]$Exe,
        [string]$Version
    )

    $uninstallPs1 = Join-Path $InstallDir "uninstall_xngxltuiplayer.ps1"
    if (-not (Test-Path -LiteralPath $uninstallPs1)) {
        Write-Warn "uninstall_xngxltuiplayer.ps1 не найден; запись в списке программ не создана"
        return
    }

    $key = "HKCU:\Software\Microsoft\Windows\CurrentVersion\Uninstall\xngxltuiplayer"
    New-Item -Path $key -Force | Out-Null
    $sizeKB = 0
    try {
        $sizeKB = [int](((Get-ChildItem -LiteralPath $InstallDir -Recurse -File -ErrorAction SilentlyContinue |
            Measure-Object -Property Length -Sum).Sum) / 1KB)
    } catch { }

    Set-ItemProperty -Path $key -Name "DisplayName" -Value "xngxltuiplayer"
    Set-ItemProperty -Path $key -Name "DisplayVersion" -Value ($Version -replace '^v', '')
    Set-ItemProperty -Path $key -Name "Publisher" -Value "xngxl"
    Set-ItemProperty -Path $key -Name "InstallLocation" -Value $InstallDir
    Set-ItemProperty -Path $key -Name "DisplayIcon" -Value "$Exe,0"
    Set-ItemProperty -Path $key -Name "UninstallString" -Value "powershell.exe -NoLogo -NoProfile -ExecutionPolicy Bypass -File `"$uninstallPs1`""
    Set-ItemProperty -Path $key -Name "NoModify" -Value 1 -Type DWord
    Set-ItemProperty -Path $key -Name "NoRepair" -Value 1 -Type DWord
    if ($sizeKB -gt 0) {
        Set-ItemProperty -Path $key -Name "EstimatedSize" -Value $sizeKB -Type DWord
    }
    Write-Info "Запись в «Программах и компонентах» создана для текущего пользователя"
}

try {
    Write-Host "xngxltuiplayer installer" -ForegroundColor Green
    Write-Host "https://github.com/maximowivan/xngxltuiplayer"

    $isAdmin = $false
    try {
        $identity = [Security.Principal.WindowsIdentity]::GetCurrent()
        $principal = New-Object Security.Principal.WindowsPrincipal($identity)
        $isAdmin = $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
    } catch { }
    if ($isAdmin) {
        Write-Warn "Установщик запущен от администратора. Плееру это не нужно; ярлыки и настройки создаются для текущего пользователя."
    }

    $defaultInstall = Join-Path $env:LOCALAPPDATA "Programs\xngxltuiplayer"
    if (-not $env:LOCALAPPDATA) {
        $defaultInstall = Join-Path $HOME "AppData\Local\Programs\xngxltuiplayer"
    }
    if (-not $PSBoundParameters.ContainsKey("InstallDir")) {
        Write-Host ""
        Write-Host "Куда установить xngxltuiplayer?"
        Write-Host "  Enter — $defaultInstall"
        Write-Host "  или введите папку, например D:\Apps\xngxltuiplayer"
        $answer = Read-Host "Папка"
        if ($answer.Trim()) {
            $InstallDir = $answer.Trim().Trim('"')
        }
    }
    if (-not $InstallDir) { $InstallDir = $defaultInstall }
    if ($InstallDir.StartsWith("~")) {
        $InstallDir = Join-Path $HOME $InstallDir.Substring(1).TrimStart("\", "/")
    }
    $InstallDir = [IO.Path]::GetFullPath($InstallDir)
    Write-Info "Папка установки: $InstallDir"

    $running = Get-Process -Name "xngxltuiplayer" -ErrorAction SilentlyContinue
    if ($running) {
        Write-Warn "Плеер запущен. Закройте его и нажмите Enter, чтобы продолжить..."
        [void](Read-Host)
        if (Get-Process -Name "xngxltuiplayer" -ErrorAction SilentlyContinue) {
            Fail "Плеер всё ещё запущен. Закройте его и запустите установщик снова."
        }
    }

    $depsOk = $true
    if (-not $NoDeps) {
        if (-not (Install-Tool "yt-dlp" "yt-dlp.yt-dlp")) { $depsOk = $false }
        if (-not (Install-Tool "ffmpeg" "Gyan.FFmpeg")) { $depsOk = $false }
        if (-not (Test-Tool "ffprobe")) {
            Write-Warn "ffprobe не найден (обычно идёт вместе с ffmpeg)"
            $depsOk = $false
        }
    }

    $source = $SourceDir
    if (-not $source) { $source = $PSScriptRoot }
    $source = [IO.Path]::GetFullPath($source)
    $localExe = Join-Path $source "xngxltuiplayer.exe"
    $version = ""

    if (Test-Path -LiteralPath $localExe) {
        Write-Step "Локальная сборка"
        $versionLine = & $localExe --version 2>$null | Select-Object -First 1
        if ($versionLine -match '(\d+\.\d+\.\d+.*)$') { $version = $Matches[1] }
        Write-Info "Версия: $version"
        Write-Info "Источник: $source"
    } else {
        Write-Step "Поиск последнего релиза"
        $headers = @{ "User-Agent" = "xngxltuiplayer-installer" }
        try {
            $release = Invoke-RestMethod -Uri "https://api.github.com/repos/maximowivan/xngxltuiplayer/releases/latest" -Headers $headers -TimeoutSec 30
        } catch {
            Fail "GitHub API недоступен: $($_.Exception.Message). Скачайте архив вручную: https://github.com/maximowivan/xngxltuiplayer/releases"
        }
        $assets = @($release.assets | Where-Object {
            $_.name -match "^xngxltuiplayer-(v?[0-9].*)-windows-amd64\.zip$"
        })
        $asset = $assets | Where-Object { $_.name -match "^xngxltuiplayer-v" } | Select-Object -First 1
        if (-not $asset) {
            $asset = $assets | Select-Object -First 1
        }
        if (-not $asset) {
            Fail "В последнем релизе нет Windows-архива. Скачайте его вручную: https://github.com/maximowivan/xngxltuiplayer/releases"
        }
        $shaAsset = $release.assets | Where-Object { $_.name -eq ($asset.name + ".sha256") } | Select-Object -First 1
        if (-not $shaAsset) {
            Fail "В релизе нет файла SHA256. Установка отменена."
        }
        $version = $release.tag_name -replace '^v', ''
        Write-Info "Версия: $version"
        Write-Info "Архив: $($asset.name)"

        $temp = Join-Path $env:TEMP "xngxltuiplayer-install"
        New-Item -ItemType Directory -Path $temp -Force | Out-Null
        $zip = Join-Path $temp $asset.name
        $shaFile = "$zip.sha256"
        $stage = Join-Path $temp "stage"
        Remove-Item -LiteralPath $zip, $shaFile -Force -ErrorAction SilentlyContinue
        Remove-Item -LiteralPath $stage -Recurse -Force -ErrorAction SilentlyContinue

        Write-Step "Скачивание"
        Invoke-WebRequest -Uri $asset.browser_download_url -OutFile $zip -UseBasicParsing -Headers $headers -TimeoutSec 180
        Invoke-WebRequest -Uri $shaAsset.browser_download_url -OutFile $shaFile -UseBasicParsing -Headers $headers -TimeoutSec 30

        Write-Step "Проверка SHA256"
        $expected = ((Get-Content -LiteralPath $shaFile -Raw) -split "\s+")[0].ToLowerInvariant()
        $actual = (Get-FileHash -LiteralPath $zip -Algorithm SHA256).Hash.ToLowerInvariant()
        if ($expected -ne $actual) {
            Fail "SHA256 не совпал. Скачанный архив повреждён или подменён."
        }
        Write-Info "SHA256 совпал: $actual"

        Write-Step "Распаковка"
        Expand-Archive -LiteralPath $zip -DestinationPath $stage -Force
        $source = $stage
        $localExe = Join-Path $source "xngxltuiplayer.exe"
        if (-not (Test-Path -LiteralPath $localExe)) {
            Fail "В архиве не найден xngxltuiplayer.exe."
        }
    }

    Write-Step "Установка файлов"
    try {
        New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
        $probe = Join-Path $InstallDir (".write-test-" + [Guid]::NewGuid().ToString("N"))
        [IO.File]::WriteAllText($probe, "test")
        Remove-Item -LiteralPath $probe -Force
    } catch {
        Fail "Нет прав на запись в $InstallDir. Выберите другую папку, например в профиле пользователя."
    }

    $sameDir = [string]::Equals(
        [IO.Path]::GetFullPath($source).TrimEnd("\"),
        [IO.Path]::GetFullPath($InstallDir).TrimEnd("\"),
        [StringComparison]::OrdinalIgnoreCase
    )
    if (-not $sameDir) {
        $sourceFiles = @(
            "xngxltuiplayer.exe",
            "README.md",
            "LICENSE",
            "THIRD_PARTY_NOTICES.md",
            "discord.example.json",
            "uninstall_xngxltuiplayer.ps1",
            "uninstall_xngxltuiplayer.bat"
        )
        foreach ($name in $sourceFiles) {
            $from = Join-Path $source $name
            if (Test-Path -LiteralPath $from) {
                Copy-Item -LiteralPath $from -Destination (Join-Path $InstallDir $name) -Force
            }
        }
        foreach ($name in @("uninstall_xngxltuiplayer.ps1", "uninstall_xngxltuiplayer.bat")) {
            $target = Join-Path $InstallDir $name
            $fallback = Join-Path $PSScriptRoot $name
            if (-not (Test-Path -LiteralPath $target) -and (Test-Path -LiteralPath $fallback)) {
                Copy-Item -LiteralPath $fallback -Destination $target -Force
            }
        }
    }

    $exe = Join-Path $InstallDir "xngxltuiplayer.exe"
    if (-not (Test-Path -LiteralPath $exe)) {
        Fail "После установки не найден xngxltuiplayer.exe."
    }
    if (-not $version) { $version = "unknown" }
    Unblock-File -LiteralPath $exe -ErrorAction SilentlyContinue
    Write-Info "Файлы установлены в $InstallDir"
    Write-Info "music\ и data\ не затрагиваются: существующие данные сохранены."

    $shortcutCount = 0
    if (-not $NoShortcut) {
        Write-Step "Ярлыки"
        $shell = $null
        try { $shell = New-Object -ComObject WScript.Shell } catch { Write-Warn "WScript.Shell недоступен: $($_.Exception.Message)" }
        if ($shell) {
        $desktop = ""
        $programs = ""
        try { $desktop = [string]$shell.SpecialFolders("Desktop") } catch { }
        try { $programs = [string]$shell.SpecialFolders("Programs") } catch { }
        if (-not $desktop) { $desktop = [Environment]::GetFolderPath("Desktop") }
        $shortcutDirs = @($desktop, $programs) | Where-Object { $_ } | Select-Object -Unique
        $wt = Find-WindowsTerminal
        if ($wt) {
            Write-Info "Ярлык откроет плеер в Windows Terminal"
        } else {
            Write-Info "Windows Terminal не найден — ярлык запустит exe напрямую"
        }

        foreach ($dir in $shortcutDirs) {
            try {
                if (-not (Test-Path -LiteralPath $dir)) {
                    New-Item -ItemType Directory -Path $dir -Force | Out-Null
                }
                $lnkPath = Join-Path $dir "xngxltuiplayer.lnk"
                $lnk = $shell.CreateShortcut($lnkPath)
                if ($wt) {
                    $lnk.TargetPath = $wt
                    $lnk.Arguments = "-d `"$InstallDir`" `"$exe`""
                } else {
                    $lnk.TargetPath = $exe
                }
                $lnk.WorkingDirectory = $InstallDir
                $lnk.IconLocation = "$exe,0"
                $lnk.Description = "xngxltuiplayer"
                $lnk.Save()
                $shortcutCount++
                Write-Info "Ярлык для текущего пользователя: $lnkPath"
            } catch {
                Write-Warn "Не удалось создать ярлык в $dir`: $($_.Exception.Message)"
            }
        }
        }
    }

    try {
        Register-UninstallEntry -InstallDir $InstallDir -Exe $exe -Version $version
    } catch {
        Write-Warn "Не удалось создать запись в «Программах и компонентах»: $($_.Exception.Message)"
    }

    Write-Step "Проверка"
    Push-Location -LiteralPath $InstallDir
    try {
        & $exe --version
        & $exe --check
        $checkExit = $LASTEXITCODE
    } finally {
        Pop-Location
    }
    if ($checkExit -ne 0) {
        Write-Warn "Проверка сообщила о нехватке компонентов. Плеер установлен, но для скачивания могут понадобиться зависимости."
    }

    Write-Host ""
    Write-Host "Готово." -ForegroundColor Green
    Write-Info "Версия: $version"
    Write-Info "Папка: $InstallDir"
    if ($shortcutCount -gt 0) { Write-Info "Ярлыков создано: $shortcutCount" }
    if (-not $depsOk) {
        Write-Warn "Не все зависимости найдены. Проверьте вывод --check выше."
    }
    Write-Info "Запуск: $exe"
    Write-Info "Удаление: $InstallDir\uninstall_xngxltuiplayer.bat"
    exit 0
} catch {
    Fail $_.Exception.Message
}
