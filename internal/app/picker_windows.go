//go:build windows

package app

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

// The start folder is passed through the environment, never spliced into the
// script, so no path can change what PowerShell runs.
const pickerScript = `[Console]::OutputEncoding = [Text.Encoding]::UTF8
Add-Type -AssemblyName System.Windows.Forms
$owner = New-Object System.Windows.Forms.Form -Property @{TopMost = $true; ShowInTaskbar = $false}
$dialog = New-Object System.Windows.Forms.FolderBrowserDialog
$dialog.Description = 'Choose where Stremio Smart Search keeps its catalogue'
$dialog.ShowNewFolderButton = $true
$dialog.SelectedPath = $env:SSS_PICKER_START
if ($dialog.ShowDialog($owner) -eq [System.Windows.Forms.DialogResult]::OK) { [Console]::Out.Write($dialog.SelectedPath) }`

// ChooseFolderNative shows the system folder picker on this computer. An empty
// result with no error means the user cancelled.
func ChooseFolderNative(start string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-STA", "-Command", pickerScript)
	cmd.Env = append(os.Environ(), "SSS_PICKER_START="+start)
	// CREATE_NO_WINDOW avoids a console without HideWindow, which would also
	// hide the dialog (see OpenFolder).
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x08000000}
	out, err := cmd.Output()
	if err != nil {
		if _, lookErr := exec.LookPath("powershell.exe"); lookErr != nil {
			return "", ErrPickerMissing
		}
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
