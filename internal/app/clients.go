package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

type Client struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Available bool   `json:"available"`
}

var ErrClientNotFound = errors.New("selected Stremio app was not found")
var clientNames = map[string]string{"stremio": "Stremio", "dxbi": "dx.bi", "enhanced": "Stremio Enhanced"}

func validClient(id string) bool { _, ok := clientNames[id]; return ok }

func clientPaths(platform, id, home, local, programFiles, programFilesX86 string) []string {
	paths := []string{}
	if platform == "windows" {
		folders := map[string][]string{
			"stremio":  {"Stremio/stremio-shell-ng.exe", "Stremio/stremio.exe", "LNV/Stremio-5/Stremio.exe", "LNV/Stremio-4/stremio.exe", "Stremio-4/stremio.exe"},
			"dxbi":     {"dxbi/dx.bi.exe", "dx.bi/dx.bi.exe"},
			"enhanced": {"stremio-enhanced/Stremio Enhanced.exe", "Stremio Enhanced/Stremio Enhanced.exe"},
		}
		roots := []string{}
		if local != "" {
			roots = append(roots, filepath.Join(local, "Programs"))
		}
		roots = append(roots, programFiles, programFilesX86)
		for _, root := range roots {
			if root == "" {
				continue
			}
			for _, part := range folders[id] {
				paths = append(paths, filepath.Join(root, filepath.FromSlash(part)))
			}
		}
	} else if platform == "darwin" {
		bundles := map[string][]string{"stremio": {"Stremio.app"}, "dxbi": {"dx.bi.app", "dxbi.app"}, "enhanced": {"Stremio Enhanced.app", "stremio-enhanced.app"}}
		for _, root := range []string{filepath.Join(home, "Applications"), "/Applications"} {
			for _, bundle := range bundles[id] {
				paths = append(paths, filepath.Join(root, bundle))
			}
		}
	}
	return paths
}

func clientCommand(id string) (*exec.Cmd, error) {
	if !validClient(id) {
		return nil, fmt.Errorf("unknown Stremio app")
	}
	home, _ := os.UserHomeDir()
	for _, path := range clientPaths(runtime.GOOS, id, home, os.Getenv("LOCALAPPDATA"), os.Getenv("ProgramFiles"), os.Getenv("ProgramFiles(x86)")) {
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		if runtime.GOOS == "darwin" && info.IsDir() {
			return exec.Command("open", "-a", path), nil
		}
		if info.Mode().IsRegular() {
			return exec.Command(path), nil
		}
	}
	commands := map[string][]string{"stremio": {"stremio", "stremio-shell-ng"}, "dxbi": {"dxbi", "dx.bi"}, "enhanced": {"stremio-enhanced", "StremioEnhanced", "Stremio Enhanced"}}
	for _, name := range commands[id] {
		if path, err := exec.LookPath(name); err == nil {
			return exec.Command(path), nil
		}
	}
	if runtime.GOOS == "linux" && id == "stremio" {
		if flatpak, err := exec.LookPath("flatpak"); err == nil {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			probe := exec.CommandContext(ctx, flatpak, "info", "com.stremio.Stremio")
			if probe.Run() == nil {
				return exec.Command(flatpak, "run", "com.stremio.Stremio"), nil
			}
		}
	}
	return nil, ErrClientNotFound
}

func InstalledClients() []Client {
	clients := make([]Client, 0, 3)
	for _, id := range []string{"stremio", "dxbi", "enhanced"} {
		_, err := clientCommand(id)
		clients = append(clients, Client{ID: id, Name: clientNames[id], Available: err == nil})
	}
	return clients
}

// Launch the selected app directly without changing the OS protocol association.
func OpenClient(id string) error {
	if id == "" || id == "system" {
		return OpenURL("stremio:///")
	}
	cmd, err := clientCommand(id)
	if err != nil {
		return err
	}
	prepareCommand(cmd)
	if err = cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

func loadClientPreference(dataDir string) string {
	var preference struct {
		Client string `json:"client"`
	}
	data, err := os.ReadFile(filepath.Join(dataDir, "preferences.json"))
	if err == nil && json.Unmarshal(data, &preference) == nil && validClient(preference.Client) {
		return preference.Client
	}
	return "system"
}

func (a *App) rememberClient(id string) error {
	if !validClient(id) {
		return fmt.Errorf("unknown Stremio app")
	}
	data, _ := json.Marshal(map[string]string{"client": id})
	if err := os.WriteFile(filepath.Join(a.dataDir, "preferences.json"), data, 0600); err != nil {
		return err
	}
	a.mu.Lock()
	a.status.PreferredClient = id
	a.mu.Unlock()
	return nil
}
