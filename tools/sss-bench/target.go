package main

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// The probes are embedded so one harness can be compiled into any source tree.
//
//go:embed probes/probekit/*.go probes/engine/*.go probes/catalogops/*.go
var probeFiles embed.FS

const (
	probeRoot   = "tools/sss-bench/probes"
	appBinary   = "sss-bench-app"
	buildLDFlag = "-s -w"
)

func executableName(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

// targetSpec names what to measure: a source tree or a prebuilt binary.
type targetSpec struct {
	name   string // baseline, candidate or target
	source string
	binary string
}

// targetRun is one target's state during a session.
type targetRun struct {
	spec       targetSpec
	info       TargetInfo
	dir        string // this target's folder in the workspace
	appPath    string
	enginePath string
	opsPath    string
	col        *collector
	problems   []Problem
	warnings   []string
	suites     []SuiteResult
	fatal      bool
}

func (t *targetRun) fail(stage string, err error) {
	t.problems = append(t.problems, Problem{Stage: stage, Message: err.Error()})
}

func (t *targetRun) warn(format string, args ...any) {
	t.warnings = append(t.warnings, fmt.Sprintf(format, args...))
}

// gitState reads a commit and whether the tree has uncommitted changes.
func gitState(ctx context.Context, dir string) (commit string, dirty bool) {
	out, err := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", "HEAD").Output()
	if err != nil {
		return "", false
	}
	commit = strings.TrimSpace(string(out))
	status, err := exec.CommandContext(ctx, "git", "-C", dir, "status", "--porcelain").Output()
	return commit, err == nil && len(bytes.TrimSpace(status)) > 0
}

// runGo runs the toolchain in dir and returns its combined output on failure.
func runGo(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "go", args...)
	cmd.Dir = dir
	cmd.Env = buildEnv()
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// probeOverlay writes the embedded probes into the workspace and returns a Go
// overlay file that places them at probeRoot inside source. The source tree
// itself is never written to, and a tree that predates the harness builds them
// exactly as this one does.
func probeOverlay(dir, source string) (string, error) {
	replace := map[string]string{}
	err := fs.WalkDir(probeFiles, "probes", func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		data, err := probeFiles.ReadFile(path)
		if err != nil {
			return err
		}
		local := filepath.Join(dir, "probe-src", filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(local), 0o700); err != nil {
			return err
		}
		if err := os.WriteFile(local, data, 0o600); err != nil {
			return err
		}
		replace[filepath.Join(source, filepath.FromSlash("tools/sss-bench"), filepath.FromSlash(path))] = local
		return nil
	})
	if err != nil {
		return "", err
	}
	data, err := json.Marshal(map[string]any{"Replace": replace})
	if err != nil {
		return "", err
	}
	overlay := filepath.Join(dir, "overlay.json")
	return overlay, os.WriteFile(overlay, data, 0o600)
}

func firstLines(text string, n int) string {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	return strings.Join(lines[:min(n, len(lines))], " | ")
}

// prepareTarget builds or copies the application and, from source, the probes.
// A probe that does not compile against an older tree costs only its metrics.
func (s *session) prepareTarget(ctx context.Context, spec targetSpec) *targetRun {
	t := &targetRun{spec: spec, col: newCollector(), dir: filepath.Join(s.ws.root, "targets", spec.name)}
	t.info.EngineProbe, t.info.OpsProbe = "not built: --binary target has no source", "not built: --binary target has no source"
	binDir := filepath.Join(t.dir, "bin")
	if err := os.MkdirAll(binDir, 0o700); err != nil {
		t.fail("prepare", err)
		t.fatal = true
		return t
	}
	t.appPath = filepath.Join(binDir, executableName(appBinary))
	var err error
	if spec.binary != "" {
		t.info.Kind = "binary"
		err = copyFile(spec.binary, t.appPath)
		if err == nil {
			err = os.Chmod(t.appPath, 0o700)
		}
	} else {
		t.info.Kind = "source"
		t.info.Commit, t.info.Dirty = gitState(ctx, spec.source)
		if t.info.Dirty {
			t.warn("%s tree has uncommitted changes, so its commit does not fully identify it", spec.name)
		}
		t.info.Go, _ = runGo(ctx, spec.source, "env", "GOVERSION")
		t.info.BuildFlags = "-trimpath -ldflags=" + buildLDFlag
		s.log.printf("%s: building application", spec.name)
		var out string
		out, err = runGo(ctx, spec.source, "build", "-trimpath", "-ldflags="+buildLDFlag, "-o", t.appPath, "./cmd/smart-search")
		if err != nil {
			err = fmt.Errorf("building the application: %v: %s", err, firstLines(out, 8))
		}
	}
	if err != nil {
		t.fail("prepare", err)
		t.fatal = true
		return t
	}
	sum, size, err := fileFingerprint(t.appPath)
	if err != nil {
		t.fail("prepare", err)
		t.fatal = true
		return t
	}
	t.info.BinarySHA256, t.info.BinaryBytes = sum, size
	t.col.add("binary.app_bytes", float64(size))
	version := exec.CommandContext(ctx, t.appPath, "--version")
	version.Env = s.env
	version.Dir = s.ws.home
	if out, err := version.Output(); err == nil {
		t.info.AppVersion = strings.TrimSpace(string(out))
	} else {
		t.fail("prepare", fmt.Errorf("%s --version failed: %w", appBinary, err))
	}
	if spec.source != "" {
		s.log.printf("%s: building probes", spec.name)
		overlay, err := probeOverlay(t.dir, spec.source)
		if err != nil {
			t.fail("prepare", err)
			return t
		}
		build := func(probe string) (string, string) {
			path := filepath.Join(binDir, executableName("sss-bench-"+probe))
			out, err := runGo(ctx, spec.source, "build", "-trimpath", "-overlay", overlay, "-o", path, "./"+probeRoot+"/"+probe)
			if err != nil {
				return "", "does not compile against this tree: " + firstLines(out, 4)
			}
			return path, "built"
		}
		t.enginePath, t.info.EngineProbe = build("engine")
		t.opsPath, t.info.OpsProbe = build("catalogops")
	}
	for id, d := range t.col.defs {
		switch {
		case d.needs == needsEngine && t.enginePath == "":
			t.col.unavailable(id, "needs the engine probe: "+t.info.EngineProbe)
		case d.needs == needsOps && t.opsPath == "":
			t.col.unavailable(id, "needs the catalogue-operations probe: "+t.info.OpsProbe)
		}
	}
	return t
}

func copyFile(from, to string) error {
	in, err := os.Open(from)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(to, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// runProbe runs a probe with a JSON request and decodes its JSON response.
func (s *session) runProbe(ctx context.Context, t *targetRun, path, stage string, request, response any) error {
	data, err := json.Marshal(request)
	if err != nil {
		return err
	}
	file := filepath.Join(t.dir, stage+".request.json")
	if err := os.WriteFile(file, data, 0o600); err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, path, file)
	cmd.Env = s.env
	cmd.Dir = s.ws.home
	var stdout bytes.Buffer
	stderr := &tail{}
	cmd.Stdout = &stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%v: %s", err, stderr.String())
	}
	if err := json.Unmarshal(stdout.Bytes(), response); err != nil {
		return fmt.Errorf("probe wrote an unreadable response: %w", err)
	}
	return nil
}
