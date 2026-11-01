package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestCrossPlatformArchiveExecutablePermissions(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"stremio-smart-search", "Stremio Smart Search.app/Contents/MacOS/SmartSearch", "README.md"} {
		if err := put(filepath.Join(root, filepath.FromSlash(name)), "fixture", 0644); err != nil {
			t.Fatal(err)
		}
	}
	archivePath := filepath.Join(t.TempDir(), "preview.zip")
	if err := archiveZip(root, archivePath); err != nil {
		t.Fatal(err)
	}
	z, err := zip.OpenReader(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()
	for _, entry := range z.File {
		want := os.FileMode(0644)
		if executablePath(entry.Name) {
			want = 0755
		}
		if entry.Mode().Perm() != want {
			t.Fatalf("ZIP %s has mode %o, want %o", entry.Name, entry.Mode().Perm(), want)
		}
	}
	data, err := tarGzip(root)
	if err != nil {
		t.Fatal(err)
	}
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	defer gz.Close()
	reader := tar.NewReader(gz)
	for {
		entry, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		want := int64(0644)
		if entry.Typeflag == tar.TypeDir || executablePath(entry.Name) {
			want = 0755
		}
		if entry.Mode != want {
			t.Fatalf("tar %s has mode %o, want %o", entry.Name, entry.Mode, want)
		}
	}
}

func TestDebianVersionOrdersPreviewsBeforeRelease(t *testing.T) {
	for in, want := range map[string]string{"0.1.0-preview.3": "0.1.0~preview.3", "0.1.0": "0.1.0", "1.2.3-rc.1": "1.2.3~rc.1"} {
		if got := debVersion(in); got != want {
			t.Fatalf("debVersion(%q) = %q, want %q", in, got, want)
		}
	}
}
