package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"syscall"
	"time"

	"github.com/michaelquigley/df/dd"
	"github.com/michaelquigley/df/dl"

	"github.com/michaelquigley/mega/internal/config"
	"github.com/michaelquigley/mega/internal/harness"
	"github.com/michaelquigley/mega/internal/harness/claude"
	"github.com/michaelquigley/mega/internal/harness/codex"
	"github.com/michaelquigley/mega/internal/registry"
	"github.com/michaelquigley/mega/internal/server"
	"github.com/michaelquigley/mega/internal/service"
)

const (
	// releaseTimeout bounds how long the orderly stop waits for runtimes to be
	// seen to exit; any still running stay recorded running and come back
	// unsupervised.
	releaseTimeout  = 30 * time.Second
	shutdownTimeout = 10 * time.Second
	// probeTimeout bounds asking whatever holds the port what it is.
	probeTimeout = time.Second
)

// instance is a started mega: its bound listener, its hold on the data
// directory, the service, and the guarded HTTP server not yet serving. serve
// and the daemon share it, and differ only in who serves and how they end.
type instance struct {
	cfg      *config.Config
	listener net.Listener
	lock     *registry.Lock
	svc      *service.Service
	server   *http.Server
	url      string
}

// start brings mega up to the point of serving: the data directory guard (in
// config.Load), the port, the lock, the registry and the service. it binds
// before touching the data directory, so a second instance on the same port
// fails without writing anything the running one owns, then holds the
// directory, so one on another port cannot become a second writer. either
// refusal names the mega already running where it can.
func start(cfg *config.Config) (*instance, error) {
	listener, err := net.Listen("tcp", cfg.Listen)
	if err != nil {
		return nil, bindError(cfg.Listen, err)
	}
	url := "http://" + listener.Addr().String()

	lock, err := registry.AcquireLock(cfg.LockPath())
	if err != nil {
		listener.Close()
		var held *registry.LockedError
		if errors.As(err, &held) {
			if held.Holder != "" {
				refusal := fmt.Errorf("another mega is running on the data directory '%s', at '%s'; open it there, or stop it before starting this one", held.Dir, held.Holder)
				// the record is only a claim; the holder is named as running
				// only if it answers as mega.
				if isMega(strings.TrimPrefix(held.Holder, "http://")) {
					return nil, &runningError{URL: held.Holder, err: refusal}
				}
				return nil, refusal
			}
			return nil, fmt.Errorf("another mega is running on the data directory '%s'; stop it before starting this one", held.Dir)
		}
		return nil, err
	}
	if err := lock.Record(url); err != nil {
		dl.Warnf("recording this instance in '%s': %v", cfg.LockPath(), err)
	}

	reg, err := registry.Load(cfg.RegistryPath())
	if err != nil {
		lock.Release()
		listener.Close()
		return nil, fmt.Errorf("loading registry: %w", err)
	}
	claudeHarness := claude.New(claude.Config{Node: cfg.Claude.Node, Shim: cfg.Claude.Shim, Executable: cfg.Claude.Executable})
	svc, err := service.New(reg, service.Options{
		IsUntrusted:   untrustedChecker(),
		InheritedMode: claudeHarness.InheritedPermissionMode,
		EventLogPath:  cfg.EventLogPath,
		Harnesses: []harness.Harness{
			claudeHarness,
			codex.New(codex.Config{Binary: cfg.Codex.Binary}),
		},
	})
	if err != nil {
		lock.Release()
		listener.Close()
		return nil, fmt.Errorf("starting service: %w", err)
	}
	dl.Debugf("claude: node='%s' shim='%s' executable='%s'", cfg.Claude.Node, cfg.Claude.Shim, cfg.Claude.Executable)
	dl.Debugf("codex: binary='%s'", cfg.Codex.Binary)
	return &instance{
		cfg:      cfg,
		listener: listener,
		lock:     lock,
		svc:      svc,
		server:   &http.Server{Handler: server.New(svc).Handler(), ReadHeaderTimeout: 10 * time.Second},
		url:      url,
	}, nil
}

// stop is the orderly stop a signal, a shutdown from the page, or the tray's
// Quit on an idle workspace all run: every runtime released and waited for,
// each conversation recorded with no runtime, subscriptions closed. the HTTP
// server is left to the caller, so a page asking keeps its answer.
func (i *instance) stop() {
	ctx, cancel := context.WithTimeout(context.Background(), releaseTimeout)
	defer cancel()
	i.svc.Shutdown(ctx)
	i.svc.Close()
}

// release lets the data directory go once nothing more will be written.
func (i *instance) release() {
	if err := i.lock.Release(); err != nil {
		dl.Warnf("releasing the data directory: %v", err)
	}
}

// bindError explains a refused bind. a port already taken is asked what it
// is: another mega answers its workspace read, and is named with its
// address; anything else is only said to hold the port.
func bindError(addr string, err error) error {
	if !errors.Is(err, syscall.EADDRINUSE) {
		return fmt.Errorf("binding '%s': %w", addr, err)
	}
	if isMega(addr) {
		return &runningError{
			URL: "http://" + addr,
			err: fmt.Errorf("another mega is already running at 'http://%s'; open it there, or stop it (Quit in its tray, or Ctrl+C where it runs) before starting this one", addr),
		}
	}
	return fmt.Errorf("'%s' is already in use by another program; stop it, or set listen in the config", addr)
}

// runningError is a start refused because another mega, which answered as
// mega, already runs on the port or the data directory. serve reports it;
// the daemon hands over to it instead, opening its page.
type runningError struct {
	URL string
	err error
}

func (e *runningError) Error() string { return e.err.Error() }

// isMega reports whether what listens on addr answers as mega does: its
// workspace read, with a projects list.
func isMega(addr string) bool {
	client := &http.Client{Timeout: probeTimeout}
	resp, err := client.Get("http://" + addr + "/api/workspace")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false
	}
	probe := &megaProbe{}
	return dd.BindJSONReader(probe, resp.Body) == nil && probe.Shutdown != nil
}

// megaProbe is the part of mega's workspace read that tells it apart: the
// shutdown gate, present on every read.
type megaProbe struct {
	Shutdown *megaProbeGate
}

type megaProbeGate struct {
	Armed bool
}

// untrustedChecker binds the project trust notice to the operator's claude
// config. the notice is informational, so a config that cannot be read shows
// no notice rather than a false one.
func untrustedChecker() service.UntrustedChecker {
	claudeConfig, err := claude.DefaultConfigPath()
	if err != nil {
		dl.Warnf("trust notices disabled: %v", err)
		return func(string) bool { return false }
	}
	return func(checkout string) bool {
		trusted, err := claude.CheckoutTrusted(claudeConfig, checkout)
		if err != nil {
			dl.Warnf("reading trust state for '%s': %v", checkout, err)
			return false
		}
		return !trusted
	}
}
