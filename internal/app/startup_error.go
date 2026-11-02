package app

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
	"unicode"
	"unicode/utf8"
)

// ShowStartupError tells the person why the helper did not start when there is
// no terminal to print to: the Windows build has no console, and apps opened
// from Finder or a desktop menu send their output nowhere visible. It returns
// without showing anything when the output is a terminal or a pipe (scripts
// and CI), so automated runs never wait on a dialog.
func ShowStartupError(message string) {
	if os.Getenv("CI") != "" || !outputHidden() {
		return
	}
	const title = "Stremio Smart Search couldn't start"
	message = sentence(message)
	switch runtime.GOOS {
	case "windows":
		messageBox(title, message)
	case "darwin":
		// The message travels as an argument, never as script text.
		cmd := exec.Command("osascript", "-e", "on run argv", "-e", `display alert "`+title+`" message (item 1 of argv) as critical`, "-e", "end run", message)
		_ = cmd.Run()
	default:
		if path, err := exec.LookPath("zenity"); err == nil {
			_ = exec.Command(path, "--error", "--no-markup", "--title="+title, "--text="+message).Run()
		} else if path, err := exec.LookPath("kdialog"); err == nil {
			_ = exec.Command(path, "--title", title, "--error", message).Run()
		}
	}
}

// sentence turns a Go error string into something to show a person.
func sentence(message string) string {
	message = strings.TrimSpace(message)
	first, size := utf8.DecodeRuneInString(message)
	if size == 0 {
		return "An unknown error stopped it."
	}
	message = string(unicode.ToUpper(first)) + message[size:]
	if !strings.HasSuffix(message, ".") {
		message += "."
	}
	return message
}
