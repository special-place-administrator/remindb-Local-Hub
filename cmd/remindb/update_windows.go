//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	"golang.org/x/sys/windows"
)

func platformUpdate() error {
	shell, err := windowsPowerShell()
	if err != nil {
		return err
	}

	logPath := filepath.Join(os.TempDir(), "remindb-update.log")
	scriptPath := filepath.Join(os.TempDir(), fmt.Sprintf("remindb-update-%d.ps1", os.Getpid()))

	script := fmt.Sprintf(`$ErrorActionPreference = 'Continue'
$ProgressPreference = 'SilentlyContinue'
$logPath = '%s'
$installURL = '%s'
$parentPid = %d

function Log-Line {
    param([string]$Line)
    "$(Get-Date -Format o) $Line" | Out-File -FilePath $logPath -Append -Encoding utf8
}

try {
    Wait-Process -Id $parentPid -ErrorAction SilentlyContinue
} catch {}
Start-Sleep -Milliseconds 500

Log-Line "update: starting install (parent pid $parentPid)"
try {
    $script = (Invoke-WebRequest -Uri $installURL -UseBasicParsing).Content
    Invoke-Expression $script 2>&1 | ForEach-Object { Log-Line "install: $_" }
    Log-Line "update: install OK"
} catch {
    Log-Line "update: install FAILED: $_"
    exit 1
}
`, logPath, installPSURL, os.Getpid())

	if err := os.WriteFile(scriptPath, []byte(script), 0o644); err != nil {
		return fmt.Errorf("failed to stage update script: %w", err)
	}

	cmd := exec.Command(shell, "-NoProfile", "-NonInteractive", "-File", scriptPath)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: windows.CREATE_NO_WINDOW | windows.CREATE_NEW_PROCESS_GROUP,
		HideWindow:    true,
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start installer: %w", err)
	}

	fmt.Fprintf(os.Stderr, "update: installer running detached (PID %d) via %s\n", cmd.Process.Pid, shell)
	fmt.Fprintf(os.Stderr, "update: this process exiting so the binary can be replaced\n")
	fmt.Fprintf(os.Stderr, "update: log %s\n", logPath)
	return nil
}

func windowsPowerShell() (string, error) {
	if path, err := exec.LookPath("pwsh.exe"); err == nil {
		return path, nil
	}
	if path, err := exec.LookPath("powershell.exe"); err == nil {
		return path, nil
	}
	return "", fmt.Errorf("no PowerShell found on PATH (expected pwsh.exe or powershell.exe)")
}
