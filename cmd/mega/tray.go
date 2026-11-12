package main

import (
	_ "embed"
	"fmt"

	"github.com/michaelquigley/df/dl"

	"github.com/michaelquigley/mega/internal/service"
)

// the mark's two states at tray size, rendered from ui/public by
// render-icons.mjs: the hub, and the hub inverted on the notice gold while
// something needs the operator.
//
//go:embed tray-icon.png
var trayIconPNG []byte

//go:embed tray-icon-needs-you.png
var trayIconNeedsYouPNG []byte

// the daemon's own tray items, by index in DaemonApp.TrayItems. dfw adds
// Quit after them.
const (
	itemOpen = iota
	itemGate
)

const (
	gateArmLabel    = "shut down when idle"
	gateCancelLabel = "cancel shutdown"
	trayTitle       = "mega"
)

// trayUI is what the daemon changes on its tray: tray.Control, through
// controlUI, or a fake in tests.
type trayUI interface {
	SetIcon(png []byte) error
	SetTooltip(tooltip string)
	SetItemLabel(index int, label string)
	Stop()
}

// gate is the part of the service the tray drives: the shutdown gate and the
// workspace it reads.
type gate interface {
	Workspace() service.Workspace
	ArmShutdown() error
	CancelShutdown() error
	ShutDown() error
}

// trayState is what the tray shows for a workspace: the needs-you icon and a
// count while the needs-you list is not empty, otherwise the mark and "mega";
// and the gate item's label, which cancels while the gate is armed.
type trayState struct {
	needsYou  int
	armed     bool
	tooltip   string
	gateLabel string
}

func stateOf(ws service.Workspace) trayState {
	s := trayState{needsYou: len(ws.NeedsYou), armed: ws.Gate.Armed, tooltip: trayTitle, gateLabel: gateArmLabel}
	switch {
	case s.needsYou == 1:
		s.tooltip = "mega: 1 needs you"
	case s.needsYou > 1:
		s.tooltip = fmt.Sprintf("mega: %d need you", s.needsYou)
	}
	if s.armed {
		s.gateLabel = gateCancelLabel
	}
	return s
}

func (s trayState) icon() []byte {
	if s.needsYou > 0 {
		return trayIconNeedsYouPNG
	}
	return trayIconPNG
}

// trayFollower keeps the tray in step with the workspace. show applies what
// changed since the last call, and nothing when nothing did.
type trayFollower struct {
	ui    trayUI
	shown *trayState
}

func (f *trayFollower) show(ws service.Workspace) {
	next := stateOf(ws)
	prev := f.shown
	if prev == nil || (prev.needsYou > 0) != (next.needsYou > 0) {
		if err := f.ui.SetIcon(next.icon()); err != nil {
			dl.Warnf("setting the tray icon: %v", err)
		}
	}
	if prev == nil || prev.tooltip != next.tooltip {
		f.ui.SetTooltip(next.tooltip)
	}
	if prev == nil || prev.gateLabel != next.gateLabel {
		f.ui.SetItemLabel(itemGate, next.gateLabel)
	}
	f.shown = &next
}

// follow shows the workspace now and after every change the service
// announces, until done closes. the service's own subscription is the
// source, so the tray never says more than the service has confirmed. a
// subscription the service drops for falling behind is taken again.
func (f *trayFollower) follow(svc interface {
	Workspace() service.Workspace
	Subscribe() *service.Subscription
}, done <-chan struct{}) {
	for {
		sub := svc.Subscribe()
		f.show(svc.Workspace())
		for update := range sub.Updates() {
			if update.WorkspaceChanged {
				f.show(svc.Workspace())
			}
		}
		sub.Close()
		select {
		case <-done:
			return
		default:
		}
	}
}

// trayActions are the daemon's answers to the tray: its items and Quit.
type trayActions struct {
	gate gate
	open func()
}

// onGateItem arms "shut down when idle" and opens the page, so the operator
// sees what it waits for; while the gate is armed the item cancels it.
func (a trayActions) onGateItem() {
	if a.gate.Workspace().Gate.Armed {
		if err := a.gate.CancelShutdown(); err != nil {
			dl.Warnf("cancelling the shutdown from the tray: %v", err)
		}
		return
	}
	if err := a.gate.ArmShutdown(); err != nil {
		dl.Warnf("arming the shutdown from the tray: %v", err)
		return
	}
	a.open()
}

// onQuit goes through the gate, so a stray click can never interrupt an
// agent. it arms the gate and asks to shut down, which the service accepts
// only if the workspace is idle at that moment, and then the daemon's
// orderly stop ends the tray. otherwise the gate stays armed and the page
// opens on what it waits for. either way dfw is told not to quit: the page
// keeps being served until the orderly stop is done.
func (a trayActions) onQuit() bool {
	if err := a.gate.ArmShutdown(); err != nil {
		// already shutting down; the stop under way ends the tray.
		return false
	}
	if err := a.gate.ShutDown(); err == nil {
		dl.Infof("quit from the tray: every agent is idle, shutting down")
		return false
	}
	dl.Infof("quit from the tray: work is in flight, so the shutdown waits until every agent is idle")
	a.open()
	return false
}
