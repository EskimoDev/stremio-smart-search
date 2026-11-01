package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func copyFile(from, to string, mode fs.FileMode) error {
	b, err := os.ReadFile(from)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(to), 0755); err != nil {
		return err
	}
	return os.WriteFile(to, b, mode)
}
func put(path, text string, mode fs.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(text), mode)
}

// Windows does not preserve POSIX executable bits on staged files. Apply package
// permissions by role so cross-built app bundles and .deb archives remain runnable.
func executablePath(path string) bool {
	base := filepath.Base(path)
	return base == "stremio-smart-search" || strings.HasSuffix(base, ".exe") || strings.Contains(filepath.ToSlash(path), "/Contents/MacOS/")
}
func archiveZip(root, path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	z := zip.NewWriter(f)
	err = filepath.WalkDir(root, func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		h, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		relative, _ := filepath.Rel(root, name)
		h.Name = filepath.ToSlash(relative)
		h.Method = zip.Deflate
		if executablePath(name) {
			h.SetMode(0755)
		} else {
			h.SetMode(0644)
		}
		w, err := z.CreateHeader(h)
		if err != nil {
			return err
		}
		r, err := os.Open(name)
		if err != nil {
			return err
		}
		defer r.Close()
		_, err = io.Copy(w, r)
		return err
	})
	if e := z.Close(); err == nil {
		err = e
	}
	if e := f.Close(); err == nil {
		err = e
	}
	return err
}
func tarGzip(root string) ([]byte, error) {
	var b bytes.Buffer
	gz := gzip.NewWriter(&b)
	tw := tar.NewWriter(gz)
	err := filepath.WalkDir(root, func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, _ := filepath.Rel(root, name)
		if relative == "." {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		h, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		h.Name = filepath.ToSlash(relative)
		h.Mode = 0644
		if d.IsDir() || executablePath(name) {
			h.Mode = 0755
		}
		h.Uid = 0
		h.Gid = 0
		h.Uname = "root"
		h.Gname = "root"
		if err = tw.WriteHeader(h); err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		r, err := os.Open(name)
		if err != nil {
			return err
		}
		defer r.Close()
		_, err = io.Copy(tw, r)
		return err
	})
	if e := tw.Close(); err == nil {
		err = e
	}
	if e := gz.Close(); err == nil {
		err = e
	}
	return b.Bytes(), err
}
func deb(path, controlDir, dataDir string) error {
	control, err := tarGzip(controlDir)
	if err != nil {
		return err
	}
	data, err := tarGzip(dataDir)
	if err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err = f.WriteString("!<arch>\n"); err != nil {
		return err
	}
	for _, entry := range []struct {
		name string
		data []byte
	}{{"debian-binary", []byte("2.0\n")}, {"control.tar.gz", control}, {"data.tar.gz", data}} {
		header := fmt.Sprintf("%-16s%-12d%-6d%-6d%-8s%-10d`\n", entry.name+"/", time.Now().Unix(), 0, 0, "100644", len(entry.data))
		if _, err = f.WriteString(header); err != nil {
			return err
		}
		if _, err = f.Write(entry.data); err != nil {
			return err
		}
		if len(entry.data)%2 != 0 {
			if _, err = f.Write([]byte{'\n'}); err != nil {
				return err
			}
		}
	}
	return nil
}

// debVersion turns a semantic pre-release such as 0.1.0-preview.3 into
// 0.1.0~preview.3. dpkg reads a hyphen as a Debian revision, which sorts
// "0.1.0-preview.3" after "0.1.0", so apt would never upgrade a preview to the
// final release. A tilde sorts before the release instead.
func debVersion(version string) string {
	return strings.Replace(version, "-", "~", 1)
}
func appBundle(root, binary, name, executable, identifier, version string) error {
	dir := filepath.Join(root, name+".app", "Contents")
	if err := copyFile(binary, filepath.Join(dir, "MacOS", executable), 0755); err != nil {
		return err
	}
	if err := copyFile("assets/icon.icns", filepath.Join(dir, "Resources", "AppIcon.icns"), 0644); err != nil {
		return err
	}
	short := strings.SplitN(version, "-", 2)[0]
	plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?><!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd"><plist version="1.0"><dict><key>CFBundleName</key><string>%s</string><key>CFBundleDisplayName</key><string>%s</string><key>CFBundleExecutable</key><string>%s</string><key>CFBundleIdentifier</key><string>%s</string><key>CFBundleIconFile</key><string>AppIcon</string><key>CFBundlePackageType</key><string>APPL</string><key>CFBundleShortVersionString</key><string>%s</string><key>CFBundleVersion</key><string>%s</string><key>LSMinimumSystemVersion</key><string>11.0</string><key>LSUIElement</key><true/></dict></plist>`, name, name, executable, identifier, short, short)
	return put(filepath.Join(dir, "Info.plist"), plist, 0644)
}
func run() error {
	osName := flag.String("os", "", "Target OS")
	arch := flag.String("arch", "", "Target architecture")
	binary := flag.String("binary", "", "Compiled binary path")
	version := flag.String("version", "0.1.0-preview.3", "Release version")
	out := flag.String("out", "dist", "Artifact folder")
	flag.Parse()
	if *osName == "" || *arch == "" || *binary == "" {
		return fmt.Errorf("os, arch and binary are required")
	}
	if strings.ContainsAny(*version, "<>\"&/\\ \r\n") {
		return fmt.Errorf("invalid version")
	}
	if err := os.MkdirAll(*out, 0755); err != nil {
		return err
	}
	stage, err := os.MkdirTemp(*out, "package-")
	if err != nil {
		return err
	}
	// The staging directory was created under the explicitly selected artifact folder.
	// Verify the absolute relationship before recursive cleanup on Windows as well.
	absRoot, _ := filepath.Abs(*out)
	absStage, _ := filepath.Abs(stage)
	rel, _ := filepath.Rel(absRoot, absStage)
	if rel == "." || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
		return fmt.Errorf("invalid staging location")
	}
	defer os.RemoveAll(absStage)
	portable := filepath.Join(stage, "portable")
	for _, file := range []string{"README.md", "LICENSE", "THIRD-PARTY-NOTICES.txt", "DATA.md", "START-HERE.txt"} {
		if err := copyFile(file, filepath.Join(portable, file), 0644); err != nil {
			return err
		}
	}
	if err := copyFile("docs/INSTALLATION.md", filepath.Join(portable, "INSTALLATION.md"), 0644); err != nil {
		return err
	}
	platformName := map[string]string{"windows": "Windows", "darwin": "macOS", "linux": "Linux"}[*osName]
	architecture := map[string]string{"amd64": "x64", "arm64": "ARM64"}[*arch]
	if *osName == "darwin" {
		architecture = map[string]string{"amd64": "Intel", "arm64": "AppleSilicon"}[*arch]
	}
	if platformName == "" || architecture == "" {
		return fmt.Errorf("unsupported package platform")
	}
	base := fmt.Sprintf("StremioSmartSearch-%s-%s-%s", *version, platformName, architecture)
	switch *osName {
	case "windows":
		if err := copyFile(*binary, filepath.Join(portable, "StremioSmartSearch.exe"), 0755); err != nil {
			return err
		}
		return archiveZip(portable, filepath.Join(*out, base+"-Portable.zip"))
	case "darwin":
		if err := appBundle(portable, *binary, "Stremio Smart Search", "SmartSearch", "io.github.EskimoDev.StremioSmartSearch", *version); err != nil {
			return err
		}
		if err := appBundle(portable, *binary, "Stremio + Smart Search", "SmartSearchLauncher", "io.github.EskimoDev.StremioSmartSearch.Launcher", *version); err != nil {
			return err
		}
		if err := archiveZip(portable, filepath.Join(*out, base+".zip")); err != nil {
			return err
		}
		// Preserve an app staging directory for native macOS signing and DMG creation.
		macStage := filepath.Join(*out, "macos-"+*arch)
		if err := os.MkdirAll(macStage, 0755); err != nil {
			return err
		}
		return filepath.WalkDir(portable, func(path string, d fs.DirEntry, e error) error {
			if e != nil {
				return e
			}
			if d.IsDir() {
				return nil
			}
			rel, _ := filepath.Rel(portable, path)
			mode := fs.FileMode(0644)
			if strings.Contains(rel, string(filepath.Separator)+"MacOS"+string(filepath.Separator)) {
				mode = 0755
			}
			return copyFile(path, filepath.Join(macStage, rel), mode)
		})
	case "linux":
		if err := copyFile(*binary, filepath.Join(portable, "stremio-smart-search"), 0755); err != nil {
			return err
		}
		blob, err := tarGzip(portable)
		if err != nil {
			return err
		}
		if err = os.WriteFile(filepath.Join(*out, base+".tar.gz"), blob, 0644); err != nil {
			return err
		}
		data, control := filepath.Join(stage, "data"), filepath.Join(stage, "control")
		if err = copyFile(*binary, filepath.Join(data, "usr/bin/stremio-smart-search"), 0755); err != nil {
			return err
		}
		if err = copyFile("internal/app/web/icon.svg", filepath.Join(data, "usr/share/icons/hicolor/scalable/apps/stremio-smart-search.svg"), 0644); err != nil {
			return err
		}
		for _, file := range []string{"LICENSE", "THIRD-PARTY-NOTICES.txt", "DATA.md", "START-HERE.txt"} {
			if err = copyFile(file, filepath.Join(data, "usr/share/doc/stremio-smart-search", file), 0644); err != nil {
				return err
			}
		}
		for _, entry := range []struct{ name, title, args string }{{"stremio-smart-search", "Stremio + Smart Search", " --launch"}, {"stremio-smart-search-settings", "Smart Search Settings", ""}} {
			desktop := fmt.Sprintf("[Desktop Entry]\nType=Application\nName=%s\nExec=/usr/bin/stremio-smart-search%s\nIcon=stremio-smart-search\nTerminal=false\nCategories=AudioVideo;Network;\n", entry.title, entry.args)
			if err = put(filepath.Join(data, "usr/share/applications", entry.name+".desktop"), desktop, 0644); err != nil {
				return err
			}
		}
		info := fmt.Sprintf("Package: stremio-smart-search\nVersion: %s\nSection: video\nPriority: optional\nArchitecture: %s\nMaintainer: EskimoDev <80299457+EskimoDev@users.noreply.github.com>\nDepends: xdg-utils\nDescription: Local typo-tolerant movie and series search for Stremio\n Includes a self-contained companion, setup page and Stremio launcher.\n", debVersion(*version), *arch)
		if err = put(filepath.Join(control, "control"), info, 0644); err != nil {
			return err
		}
		return deb(filepath.Join(*out, base+".deb"), control, data)
	default:
		return fmt.Errorf("unsupported target OS")
	}
}
