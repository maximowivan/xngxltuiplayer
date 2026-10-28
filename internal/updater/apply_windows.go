//go:build windows

package updater

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
)

const applyScript = `param(
    [int]$WaitPid,
    [string]$Stage,
    [string]$InstallDir,
    [switch]$NoRestart
)
$ErrorActionPreference = "Stop"
$log = Join-Path $env:TEMP "xngxltuiplayer-update.log"
function Write-Log([string]$Text) {
    Add-Content -LiteralPath $log -Value ("{0:u} {1}" -f (Get-Date), $Text)
}
try {
    $p = Get-Process -Id $WaitPid -ErrorAction SilentlyContinue
    if ($p) { $p.WaitForExit() }
    Start-Sleep -Milliseconds 700

    $target = Join-Path $InstallDir "xngxltuiplayer.exe"
    $backup = "$target.old"
    $files = @(
        "xngxltuiplayer.exe",
        "README.md",
        "LICENSE",
        "THIRD_PARTY_NOTICES.md",
        "discord.example.json",
        "uninstall_xngxltuiplayer.ps1",
        "uninstall_xngxltuiplayer.bat"
    )
    if (Test-Path -LiteralPath $target) {
        Remove-Item -LiteralPath $backup -Force -ErrorAction SilentlyContinue
        Move-Item -LiteralPath $target -Destination $backup -Force
    }
    foreach ($name in $files) {
        $src = Join-Path $Stage $name
        if (Test-Path -LiteralPath $src) {
            Copy-Item -LiteralPath $src -Destination (Join-Path $InstallDir $name) -Force
        }
    }
    $logo = Join-Path $Stage "assets\logo.png"
    if (Test-Path -LiteralPath $logo) {
        $assetsDir = Join-Path $InstallDir "assets"
        New-Item -ItemType Directory -Path $assetsDir -Force | Out-Null
        Copy-Item -LiteralPath $logo -Destination (Join-Path $assetsDir "logo.png") -Force
    }
    Remove-Item -LiteralPath $backup -Force -ErrorAction SilentlyContinue
    Write-Log "updated $InstallDir"
    if (-not $NoRestart) {
        $wt = Join-Path $InstallDir "terminal\wt.exe"
        $restarted = $false
        if (Test-Path -LiteralPath $wt) {
            try {
                $wtArgs = '-d "{0}" "{1}"' -f $InstallDir, $target
                Start-Process -FilePath $wt -ArgumentList $wtArgs -WorkingDirectory $InstallDir
                $restarted = $true
            } catch {
                Write-Log ("Windows Terminal restart failed: " + $_.Exception.Message)
            }
        }
        if (-not $restarted) {
            Start-Process -FilePath $target -WorkingDirectory $InstallDir
        }
    }
} catch {
    Write-Log ("FAILED: " + $_.Exception.Message)
    $target = Join-Path $InstallDir "xngxltuiplayer.exe"
    $backup = "$target.old"
    if (Test-Path -LiteralPath $backup) {
        Copy-Item -LiteralPath $backup -Destination $target -Force -ErrorAction SilentlyContinue
    }
    exit 1
}
`

// Apply starts a helper that waits for the current process to exit, replaces
// the executable and restarts the player.
func Apply(stageDir, installDir string) error {
	script := filepath.Join(stageDir, "apply-update.ps1")
	if err := os.WriteFile(script, []byte(applyScript), 0o644); err != nil {
		return err
	}

	cmd := exec.Command("powershell.exe",
		"-NoLogo", "-NoProfile", "-ExecutionPolicy", "Bypass",
		"-WindowStyle", "Hidden",
		"-File", script,
		"-WaitPid", strconv.Itoa(os.Getpid()),
		"-Stage", stageDir,
		"-InstallDir", installDir,
	)
	cmd.Dir = installDir
	if err := startHidden(cmd); err != nil {
		return fmt.Errorf("start update helper: %w", err)
	}
	return nil
}
