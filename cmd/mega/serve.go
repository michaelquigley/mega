package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os/signal"
	"syscall"

	"github.com/michaelquigley/df/dl"
	"github.com/spf13/cobra"

	"github.com/michaelquigley/mega/internal/config"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Run the environment service (the default)",
	Args:  cobra.NoArgs,
	RunE:  runServe,
}

func init() {
	rootCmd.AddCommand(serveCmd)
}

func runServe(_ *cobra.Command, _ []string) error {
	cfg, err := config.Load(configPath, dataPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	dl.Debugf("config: listen='%s' data='%s'", cfg.Listen, cfg.Data)
	inst, err := start(cfg)
	if err != nil {
		return err
	}
	defer inst.release()

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
		case <-inst.svc.ShutdownRequested():
			dl.Infof("shutting down, as asked from the workspace once every agent was idle")
		}
		inst.stop()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := inst.server.Shutdown(shutdownCtx); err != nil {
			dl.Errorf("server shutdown: %v", err)
		}
	}()

	dl.Infof("listening on '%s'", inst.url)
	if err := inst.server.Serve(inst.listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serving: %w", err)
	}
	<-shutdownDone
	return nil
}
