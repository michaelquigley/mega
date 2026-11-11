package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/michaelquigley/df/dl"
	"github.com/spf13/cobra"

	"github.com/michaelquigley/mega/internal/config"
	"github.com/michaelquigley/mega/internal/harness"
	"github.com/michaelquigley/mega/internal/harness/claude"
	"github.com/michaelquigley/mega/internal/harness/codex"
	"github.com/michaelquigley/mega/internal/registry"
	"github.com/michaelquigley/mega/internal/server"
	"github.com/michaelquigley/mega/internal/service"
)

const (
	// releaseTimeout bounds how long shutdown waits for runtimes to be seen to
	// exit; any still running stay recorded running and come back unsupervised.
	releaseTimeout  = 30 * time.Second
	shutdownTimeout = 10 * time.Second
)

func init() {
	rootCmd.AddCommand(&cobra.Command{
		Use:   "serve",
		Short: "Run the environment service (the default)",
		Args:  cobra.NoArgs,
		RunE:  runServe,
	})
}

func runServe(_ *cobra.Command, _ []string) error {
	cfg, err := config.Load(configPath, dataPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	dl.Debugf("config: listen='%s' data='%s'", cfg.Listen, cfg.Data)

	// bind before touching the data directory, so a second instance on the
	// same port fails without writing anything the running service owns.
	listener, err := net.Listen("tcp", cfg.Listen)
	if err != nil {
		return fmt.Errorf("binding: %w", err)
	}
	defer listener.Close()

	// then hold the data directory, so an instance on another port that
	// shares it cannot become a second writer of the registry.
	lock, err := registry.AcquireLock(cfg.LockPath())
	if err != nil {
		return err
	}
	defer lock.Release()

	reg, err := registry.Load(cfg.RegistryPath())
	if err != nil {
		return fmt.Errorf("loading registry: %w", err)
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
		return fmt.Errorf("starting service: %w", err)
	}
	dl.Debugf("claude: node='%s' shim='%s' executable='%s'", cfg.Claude.Node, cfg.Claude.Shim, cfg.Claude.Executable)
	dl.Debugf("codex: binary='%s'", cfg.Codex.Binary)
	httpServer := &http.Server{
		Handler:           server.New(svc).Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)
		// a signal, or a shutdown the operator asked for from the gate once
		// every agent was idle: the same orderly stop either way.
		select {
		case <-ctx.Done():
			dl.Infof("shutting down")
		case <-svc.ShutdownRequested():
			dl.Infof("shutting down, as asked from the workspace once every agent was idle")
		}
		releaseCtx, cancelRelease := context.WithTimeout(context.Background(), releaseTimeout)
		svc.Shutdown(releaseCtx)
		cancelRelease()
		svc.Close()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			dl.Errorf("server shutdown: %v", err)
		}
	}()

	dl.Infof("listening on 'http://%s'", listener.Addr())
	if err := httpServer.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serving: %w", err)
	}
	<-shutdownDone
	return nil
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
