package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os/signal"
	"syscall"

	"github.com/michaelquigley/df/dl"
	"github.com/michaelquigley/dfw/tray"
	"github.com/spf13/cobra"

	"github.com/michaelquigley/mega/internal/config"
)

// appID is mega's desktop identity: the tray, the launcher and its icons
// share it, so the desktop ties them together.
const appID = "com.michaelquigley.mega"

var daemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "Run the service with a tray icon",
	Long: "Run the service as serve does, with a tray icon: open the page, arm or cancel " +
		"\"shut down when idle\", and Quit, which goes through the shutdown gate.",
	Args: cobra.NoArgs,
	RunE: runDaemon,
}

func init() {
	rootCmd.AddCommand(daemonCmd)
}

// handOver is a second launch meeting a mega already running, as the desktop
// launcher does when mega is up: it opens that mega's page and reports the
// launch done. anything else, including a port held by something that is not
// mega or a recorded holder that does not answer, is left to refuse.
func handOver(err error, open func(url string) error) bool {
	var running *runningError
	if !errors.As(err, &running) {
		return false
	}
	if openErr := open(running.URL); openErr != nil {
		dl.Errorf("another mega is already running at '%s', but its page could not be opened: %v", running.URL, openErr)
		return false
	}
	dl.Infof("another mega is already running at '%s'; opened its page instead of starting a second", running.URL)
	return true
}

// controlUI is tray.Control as trayUI.
type controlUI struct{ control *tray.Control }

func (c controlUI) SetIcon(png []byte) error     { return c.control.SetIcon(png) }
func (c controlUI) SetTooltip(tooltip string)    { c.control.SetTooltip(tooltip) }
func (c controlUI) SetItemLabel(i int, l string) { c.control.Item(i).SetLabel(l) }
func (c controlUI) Stop()                        { c.control.Stop() }

// runDaemon starts mega as serve does and hands it to dfw's tray. every way
// out is the orderly stop serve runs on a signal, followed by the tray going:
// a shutdown accepted from the page, Quit on an idle workspace (through the
// same acceptance), or SIGINT or SIGTERM. dfw serves the page until the tray
// is stopped, so the page that asked keeps its answer.
func runDaemon(_ *cobra.Command, _ []string) error {
	cfg, err := config.Load(configPath, dataPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	inst, err := start(cfg)
	if err != nil {
		if handOver(err, func(url string) error { return openBrowserAndWait(url, handOverWait) }) {
			return nil
		}
		return err
	}
	defer inst.release()

	control := tray.NewControl()
	ui := controlUI{control: control}
	open := func() {
		if err := openBrowser(inst.url); err != nil {
			dl.Errorf("opening the page: %v", err)
		}
	}
	actions := trayActions{gate: inst.svc, open: open}

	ctx, stopSignals := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stopSignals()
	trayDone := make(chan struct{})
	ending := make(chan struct{})
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		select {
		case <-ctx.Done():
			dl.Infof("shutting down")
		case <-inst.svc.ShutdownRequested():
			dl.Infof("shutting down, as asked once every agent was idle")
		case <-trayDone:
			dl.Infof("the tray ended; shutting down")
		}
		close(ending)
		inst.stop()
		ui.Stop()
	}()
	follower := &trayFollower{ui: ui}
	go follower.follow(inst.svc, ending)

	dl.Infof("daemon serving at '%s'", inst.url)
	err = tray.Daemon(tray.DaemonApp{
		AppID:   appID,
		Title:   trayTitle,
		IconPNG: trayIconPNG,
		Listen: func() (*http.Server, net.Listener, error) {
			return inst.server, inst.listener, nil
		},
		TrayItems: []tray.TrayMenuItem{
			itemOpen: {Label: "open mega", Tooltip: "open mega in your browser", OnClick: open},
			itemGate: {Label: gateArmLabel, Tooltip: "stop once every agent is idle", OnClick: actions.onGateItem},
		},
		OnQuit:  actions.onQuit,
		Control: control,
	})
	// the tray is gone: by the orderly stop, or because it failed. either way
	// nothing exits before the stop has run.
	close(trayDone)
	<-stopped
	if err != nil {
		return fmt.Errorf("tray: %w", err)
	}
	return nil
}
