//go:build !windows

package app

import (
	"context"
	"errors"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

const pickerTitle = "Choose where Stremio Smart Search keeps its catalogue"

// ChooseFolderNative shows the system folder picker on this computer. An empty
// result with no error means the user cancelled.
func ChooseFolderNative(start string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	var cmd *exec.Cmd
	switch {
	case runtime.GOOS == "darwin":
		// Ask the frontmost app (the browser) to show it, so it opens on top.
		// The prompt and start folder are passed as arguments, not script text.
		cmd = exec.CommandContext(ctx, "osascript",
			"-e", "on run argv",
			"-e", "tell application (path to frontmost application as text) to return POSIX path of (choose folder with prompt (item 1 of argv) default location (POSIX file (item 2 of argv)))",
			"-e", "end run", pickerTitle, start)
	case has("zenity"):
		cmd = exec.CommandContext(ctx, "zenity", "--file-selection", "--directory", "--title="+pickerTitle, "--filename="+start+"/")
	case has("kdialog"):
		cmd = exec.CommandContext(ctx, "kdialog", "--getexistingdirectory", start, "--title", pickerTitle)
	default:
		return "", ErrPickerMissing
	}
	out, err := cmd.Output()
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() == 1 {
		return "", nil // cancelled in every picker above
	}
	if err != nil {
		return "", err
	}
	return strings.TrimRight(strings.TrimSpace(string(out)), "/"), nil
}

func has(name string) bool { _, err := exec.LookPath(name); return err == nil }
