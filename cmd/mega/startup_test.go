package main

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/michaelquigley/mega/internal/config"
	"github.com/michaelquigley/mega/internal/registry"
)

// freePort is a loopback address nothing listens on.
func freePort(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	l.Close()
	return addr
}

func scratchConfig(t *testing.T, listen, data string) *config.Config {
	t.Helper()
	path := filepath.Join(t.TempDir(), "mega.yaml")
	body := fmt.Sprintf("listen: %s\ndata: %s\nclaude:\n  shim: /nonexistent\ncodex:\n  binary: /nonexistent\n", listen, data)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path, "")
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

// a second mega on the same port is told another mega runs there, by
// address; one on another port over the same data directory is told the
// directory is held, with the holder's address.
func TestASecondMegaNamesTheFirst(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	data := t.TempDir()
	addr := freePort(t)
	first, err := start(scratchConfig(t, addr, data))
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = first.server.Serve(first.listener) }()
	t.Cleanup(func() {
		first.stop()
		_ = first.server.Close()
		first.release()
	})

	var running *runningError
	_, err = start(scratchConfig(t, addr, t.TempDir()))
	if err == nil || !strings.Contains(err.Error(), "another mega is already running at 'http://"+addr+"'") || !errors.As(err, &running) || running.URL != "http://"+addr {
		t.Errorf("same port: %v", err)
	}
	_, err = start(scratchConfig(t, freePort(t), data))
	if err == nil || !strings.Contains(err.Error(), "another mega is running on the data directory '"+data+"', at 'http://"+addr+"'") || !errors.As(err, &running) || running.URL != "http://"+addr {
		t.Errorf("same data directory: %v", err)
	}

	// a port held by something that is not mega is said to be.
	other, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	go func() { _ = http.Serve(other, http.NotFoundHandler()) }()
	_, err = start(scratchConfig(t, other.Addr().String(), t.TempDir()))
	if err == nil || !strings.Contains(err.Error(), "already in use by another program") || errors.As(err, &running) {
		t.Errorf("another program: %v", err)
	}
}

// a holder that recorded an address nothing answers on is refused, not
// handed over to.
func TestASilentHolderIsRefusedNotNamedRunning(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	data := t.TempDir()
	cfg := scratchConfig(t, freePort(t), data)
	lock, err := registry.AcquireLock(cfg.LockPath())
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Release()
	if err := lock.Record("http://" + freePort(t)); err != nil {
		t.Fatal(err)
	}
	_, err = start(cfg)
	var running *runningError
	if err == nil || !strings.Contains(err.Error(), "another mega is running on the data directory") || errors.As(err, &running) {
		t.Errorf("silent holder: %v", err)
	}
}

// only another mega that answered is handed over to: its page is opened and
// the launch is done. anything else is left to refuse, and so is a page that
// cannot be opened.
func TestHandOverOpensTheRunningMega(t *testing.T) {
	var opened []string
	open := func(url string) error { opened = append(opened, url); return nil }
	if !handOver(&runningError{URL: "http://127.0.0.1:18470", err: errors.New("running")}, open) || len(opened) != 1 || opened[0] != "http://127.0.0.1:18470" {
		t.Errorf("a running mega: %v", opened)
	}
	if handOver(errors.New("'127.0.0.1:18470' is already in use by another program"), open) || len(opened) != 1 {
		t.Errorf("not mega: %v", opened)
	}
	failing := func(string) error { return errors.New("no opener") }
	if handOver(&runningError{URL: "http://127.0.0.1:18470", err: errors.New("running")}, failing) {
		t.Error("handed over without opening the page")
	}
}

// a log past the limit is moved to .1 at start-up, replacing an earlier one;
// one within it is left to grow.
func TestCapLog(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mega.log")
	if err := capLog(path, 10); err != nil {
		t.Errorf("no log yet: %v", err)
	}
	if err := os.WriteFile(path, []byte("small"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := capLog(path, 10); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); string(got) != "small" {
		t.Errorf("a small log was moved: %q", got)
	}
	if err := os.WriteFile(path+".1", []byte("older"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("larger than ten bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := capLog(path, 10); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("the large log is still in place: %v", err)
	}
	if got, _ := os.ReadFile(path + ".1"); string(got) != "larger than ten bytes" {
		t.Errorf(".1 holds %q", got)
	}
	if logLimit != 10<<20 {
		t.Errorf("limit = %d", logLimit)
	}
}

// serve is unchanged: it serves the workspace, and a shutdown accepted from
// the page (armed, idle) ends it with no error and the directory released.
func TestServeEndsOnAnAcceptedShutdown(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	data := t.TempDir()
	addr := freePort(t)
	cfgFile := filepath.Join(t.TempDir(), "mega.yaml")
	if err := os.WriteFile(cfgFile, []byte(fmt.Sprintf("listen: %s\ndata: %s\n", addr, data)), 0o644); err != nil {
		t.Fatal(err)
	}
	saved := configPath
	configPath = cfgFile
	t.Cleanup(func() { configPath = saved })
	done := make(chan error, 1)
	go func() { done <- runServe(nil, nil) }()

	url := "http://" + addr
	var resp *http.Response
	var err error
	for range 100 {
		if resp, err = http.Get(url + "/api/workspace"); err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), `"projects"`) {
		t.Fatalf("workspace: %d %s", resp.StatusCode, body)
	}
	for _, path := range []string{"/api/shutdown/arm", "/api/shutdown"} {
		resp, err := http.Post(url+path, "application/json", nil)
		if err != nil || resp.StatusCode != http.StatusAccepted {
			t.Fatalf("%s: %v %v", path, resp, err)
		}
		resp.Body.Close()
	}
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("serve ended with %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("serve did not end")
	}
	// the directory is free for the next start.
	inst, err := start(scratchConfig(t, freePort(t), data))
	if err != nil {
		t.Fatalf("after serve ended: %v", err)
	}
	inst.stop()
	inst.release()
	inst.listener.Close()
}

func TestLogPathFromEnv(t *testing.T) {
	if got, err := logPathFromEnv("/tmp/state", "/home/example"); err != nil || got != "/tmp/state/mega/mega.log" {
		t.Errorf("absolute XDG_STATE_HOME: %q, %v", got, err)
	}
	want := filepath.Join("/home/example", ".local", "state", "mega", "mega.log")
	for _, xdg := range []string{"", "relative"} {
		if got, err := logPathFromEnv(xdg, "/home/example"); err != nil || got != want {
			t.Errorf("XDG_STATE_HOME %q: %q, %v", xdg, got, err)
		}
	}
	if _, err := logPathFromEnv("", ""); err == nil {
		t.Error("the fallback needs HOME")
	}
}

// /dev/null is a character device but not a terminal: a daemon a launcher
// started with its output there still logs to its file.
func TestDevNullIsNotATerminal(t *testing.T) {
	f, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if onTerminal(f) {
		t.Error("/dev/null counted as a terminal")
	}
}
