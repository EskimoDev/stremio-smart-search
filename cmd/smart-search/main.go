package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/EskimoDev/stremio-smart-search/internal/app"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"syscall"
	"time"
)

var version = "0.1.0-dev"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		app.ShowStartupError(err.Error())
		os.Exit(1)
	}
}
func run() error {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return err
	}
	dataDir := flag.String("data-dir", filepath.Join(configDir, "StremioSmartSearch"), "Local catalogue and settings folder")
	listen := flag.String("listen", "127.0.0.1:"+app.DefaultPort, "Listen address (loopback by default)")
	noBrowser := flag.Bool("no-browser", false, "Run without opening settings")
	launch := flag.Bool("launch", strings.Contains(filepath.Base(os.Args[0]), "Launcher"), "Start the local addon and open Stremio when ready")
	quit := flag.Bool("quit", false, "Stop the running local companion")
	importCatalog := flag.String("catalog", "", "Load a custom catalogue JSON or JSON.gz")
	showVersion := flag.Bool("version", false, "Print version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Println(version)
		return nil
	}
	_, port, err := net.SplitHostPort(*listen)
	if err != nil {
		return fmt.Errorf("invalid listen address: %w", err)
	}
	baseURL := "http://127.0.0.1:" + port
	if *quit {
		if _, err := os.Stat(filepath.Join(*dataDir, "control.key")); os.IsNotExist(err) {
			return nil
		}
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	runtime.GOMAXPROCS(min(2, runtime.NumCPU()))
	debug.SetMemoryLimit(256 << 20)
	a, err := app.New(ctx, *dataDir, baseURL, version)
	if err != nil {
		return err
	}
	if exe, err := os.Executable(); err == nil {
		// Start at sign-in keeps any non-default folder or address this copy uses.
		var keep []string
		flag.Visit(func(f *flag.Flag) {
			if f.Name == "data-dir" || f.Name == "listen" {
				keep = append(keep, "--"+f.Name+"="+f.Value.String())
			}
		})
		if resolved, err := filepath.EvalSymlinks(exe); err == nil {
			exe = resolved
		}
		a.Autostart = &app.Autostart{Exe: exe, Args: keep}
	}
	// Reuse the existing process only after authenticating it with this data folder's
	// key. A different app occupying the port must not receive launch/control calls.
	client := &http.Client{Timeout: 2 * time.Second}
	req, _ := http.NewRequest(http.MethodGet, baseURL+"/api/status", nil)
	req.Header.Set("X-Smart-Search-Key", a.Token)
	if response, e := client.Do(req); e == nil {
		valid := response.StatusCode == 200
		var existing struct {
			Ready           bool
			PreferredClient string
		}
		if valid {
			valid = json.NewDecoder(response.Body).Decode(&existing) == nil
		}
		response.Body.Close()
		if valid {
			if *quit {
				q, _ := http.NewRequest(http.MethodPost, baseURL+"/api/quit", nil)
				q.Header.Set("X-Smart-Search-Key", a.Token)
				r, e := client.Do(q)
				if e != nil {
					return e
				}
				r.Body.Close()
				return nil
			}
			if *launch && existing.Ready {
				if err := app.OpenClient(existing.PreferredClient); err == nil {
					return nil
				}
				return app.OpenURL(baseURL)
			}
			if !*noBrowser {
				return app.OpenURL(baseURL)
			}
			return nil
		}
	}
	if *quit {
		return nil
	}
	if *importCatalog != "" {
		if err := a.Load(*importCatalog); err != nil {
			return err
		}
	} else if err := a.Restore(); err != nil && !os.IsNotExist(err) {
		fmt.Fprintln(os.Stderr, "Stored catalogue could not be loaded; rebuild it from settings.")
	}
	listener, err := net.Listen("tcp", *listen)
	if addressInUse(err) {
		return fmt.Errorf("another program is already using port %s, which Smart Search needs. If another copy of Smart Search is running from a different folder, quit that copy first. Otherwise close the other program and open Smart Search again", port)
	}
	if err != nil {
		return fmt.Errorf("could not start Smart Search: %w", err)
	}
	server := &http.Server{Handler: a.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16 << 10}
	a.Quit = cancel
	if *importCatalog == "" {
		a.StartUpdates(ctx)
	} else {
		// Automatic updates replace the managed catalogue, never a --catalog file.
		a.DisableUpdates()
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, done := context.WithTimeout(context.Background(), 3*time.Second)
		defer done()
		_ = server.Shutdown(shutdownCtx)
	}()
	if !*noBrowser {
		if *launch && a.Status().Ready {
			if err := app.OpenClient(a.Status().PreferredClient); err != nil {
				_ = app.OpenURL(baseURL)
			}
		} else {
			_ = app.OpenURL(baseURL)
		}
	}
	fmt.Printf("%s %s — %s\n", app.Product, version, baseURL)
	err = server.Serve(listener)
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

// addressInUse covers both the Unix error and Windows' WSAEADDRINUSE (10048).
func addressInUse(err error) bool {
	var errno syscall.Errno
	return errors.As(err, &errno) && (errno == syscall.EADDRINUSE || errno == 10048)
}
