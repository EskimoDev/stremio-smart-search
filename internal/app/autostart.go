package app

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Autostart starts this copy of the helper quietly when the user signs in, so
// the Smart Search rows are there when Stremio is opened through any shortcut.
// It only ever writes the current user's own sign-in entry:
//   - Windows: the HKCU Run value the installer's optional component also uses
//   - macOS: a LaunchAgent in ~/Library/LaunchAgents
//   - Linux: an XDG autostart entry in ~/.config/autostart
type Autostart struct {
	Exe  string   // this copy of the helper
	Args []string // extra flags to keep, such as a custom --data-dir
	goos string   // tests override the platform
	dir  string   // tests override the per-user folder
}

const (
	autostartName  = "StremioSmartSearch" // Windows Run value, shared with the installer
	launchAgentID  = "community.eskimodev.smartsearch"
	desktopEntryID = "stremio-smart-search"
)

var errAutostartUnsupported = errors.New("starting at sign-in is not available on this system")

// AutostartStatus is what the settings page shows.
type AutostartStatus struct {
	Supported bool   `json:"supported"`
	Enabled   bool   `json:"enabled"`
	Path      string `json:"path,omitempty"`
}

func (s *Autostart) platform() string {
	if s.goos != "" {
		return s.goos
	}
	return runtime.GOOS
}

// command is the helper started at sign-in: no settings page and no Stremio.
func (s *Autostart) command() []string {
	return append([]string{s.Exe, "--no-browser", "--launch=false"}, s.Args...)
}

// file is the sign-in entry on macOS and Linux.
func (s *Autostart) file() (string, error) {
	dir := s.dir
	switch s.platform() {
	case "darwin":
		if dir == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return "", err
			}
			dir = filepath.Join(home, "Library", "LaunchAgents")
		}
		return filepath.Join(dir, launchAgentID+".plist"), nil
	case "linux", "freebsd", "openbsd", "netbsd":
		if dir == "" {
			config := os.Getenv("XDG_CONFIG_HOME")
			if config == "" {
				home, err := os.UserHomeDir()
				if err != nil {
					return "", err
				}
				config = filepath.Join(home, ".config")
			}
			dir = filepath.Join(config, "autostart")
		}
		return filepath.Join(dir, desktopEntryID+".desktop"), nil
	}
	return "", errAutostartUnsupported
}

func (s *Autostart) Status() AutostartStatus {
	if s == nil || s.Exe == "" {
		return AutostartStatus{}
	}
	status := AutostartStatus{Supported: true, Path: s.Exe}
	if s.platform() == "windows" {
		on, err := runValueExists(autostartName)
		status.Supported, status.Enabled = err == nil, on
		return status
	}
	path, err := s.file()
	if err != nil {
		return AutostartStatus{}
	}
	_, err = os.Stat(path)
	status.Enabled = err == nil
	return status
}

func (s *Autostart) Set(on bool) error {
	if s == nil || s.Exe == "" {
		return errAutostartUnsupported
	}
	if s.platform() == "windows" {
		if !on {
			return deleteRunValue(autostartName)
		}
		return setRunValue(autostartName, windowsCommandLine(s.command()))
	}
	path, err := s.file()
	if err != nil {
		return err
	}
	if !on {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	content := desktopEntry(s.command())
	if s.platform() == "darwin" {
		content = launchAgent(s.command())
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0644)
}

func launchAgent(command []string) string {
	var args strings.Builder
	for _, arg := range command {
		args.WriteString("\t\t<string>" + xmlEscape(arg) + "</string>\n")
	}
	return `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>` + launchAgentID + `</string>
	<key>ProgramArguments</key>
	<array>
` + args.String() + `	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>ProcessType</key>
	<string>Background</string>
</dict>
</plist>
`
}

func xmlEscape(value string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&apos;").Replace(value)
}

// desktopEntry follows the XDG Desktop Entry quoting rules for Exec.
func desktopEntry(command []string) string {
	quoted := make([]string, len(command))
	for i, arg := range command {
		arg = strings.NewReplacer(`\`, `\\`, `"`, `\"`, "`", "\\`", "$", `\$`, "%", "%%").Replace(arg)
		quoted[i] = `"` + arg + `"`
	}
	exec := strings.NewReplacer(`\`, `\\`).Replace(strings.Join(quoted, " "))
	return "[Desktop Entry]\nType=Application\nName=Stremio Smart Search\nComment=Starts the Smart Search helper for Stremio\nExec=" + exec + "\nIcon=stremio-smart-search\nTerminal=false\nNoDisplay=true\nX-GNOME-Autostart-enabled=true\n"
}

// windowsCommandLine quotes each argument the way CommandLineToArgvW reads it.
func windowsCommandLine(command []string) string {
	quoted := make([]string, len(command))
	for i, arg := range command {
		if arg != "" && !strings.ContainsAny(arg, " \t\"") {
			quoted[i] = arg
			continue
		}
		var b strings.Builder
		b.WriteByte('"')
		slashes := 0
		for _, c := range arg {
			switch c {
			case '\\':
				slashes++
				continue
			case '"':
				b.WriteString(strings.Repeat(`\`, slashes*2+1))
			default:
				b.WriteString(strings.Repeat(`\`, slashes))
			}
			slashes = 0
			b.WriteRune(c)
		}
		b.WriteString(strings.Repeat(`\`, slashes*2))
		b.WriteByte('"')
		quoted[i] = b.String()
	}
	return strings.Join(quoted, " ")
}
