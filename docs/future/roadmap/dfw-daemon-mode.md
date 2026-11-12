---
title: dfw daemon mode
state: evaluating
created: 2026-11-09
tags: [feature, spike]
---

Let mega live on the GNOME tray as a daemon, the way ranger does: `mega daemon` runs the service with a tray icon, and `mega desktop integrate` installs the launcher. `mega serve` stays as it is and stays the default.

## the house pattern

ranger is the reference (`products/ranger/cmd/ranger/daemon.go` and `desktop.go`). `tray.Daemon` from `github.com/michaelquigley/dfw/tray` takes an app id, a title, a PNG icon, a `Listen` function returning the server and its bound listener, and a list of tray items; it returns when Quit is chosen. The tray subpackage needs no webview toolchain. `ranger desktop integrate` and `remove` manage a `.desktop` entry and hicolor icons whose basename matches the app id, launching `ranger daemon`. Mega opens no webview; the browser is the window.

## shape

- **`mega daemon`**: the same start-up as `serve` (the data directory guard, the lock, the registry, the orderly-stop path), handed to `tray.Daemon` with app id `com.michaelquigley.mega`, title `mega`, and the tray icon rendered from the favicon.
- **Tray items**: "open mega", which opens the page in the browser, and "shut down when idle", which arms the gate and opens the page so the operator sees what it is waiting for.
- **When the daemon stops, it is the orderly stop.** Whether through Quit on an idle workspace or a shutdown accepted from the page, mega runs exactly what a SIGINT runs: every runtime released, every conversation recorded at rest, exit 0, and the tray icon goes with it.
- **`mega desktop integrate` and `mega desktop remove`**, following `products/ranger/cmd/ranger/desktop.go` closely, because the details are what make the desktop treat it as an application:
  - the desktop id is the reverse-DNS app id, `com.michaelquigley.mega`, the same string the tray uses, so GNOME ties the launcher, the dock icon and the tray together;
  - `integrate` writes `com.michaelquigley.mega.desktop` under the user's applications directory (honouring an absolute `XDG_DATA_HOME`, else `~/.local/share`) with `Type=Application`, `Name=mega`, a one-line comment, `Exec` as the quoted absolute path of the running executable followed by `daemon`, `Icon` as the app id, `Terminal=false` and `Categories=Utility;Development;`. No `StartupWMClass`: the browser is the window;
  - it installs the plain mark at 32, 48, 192 and 512 pixels into the hicolor theme under the same basename, from the PNGs the favicon batch checked in under `cmd/mega/`, embedded in the binary;
  - `remove` deletes exactly the files `integrate` writes and nothing else; both are safe to run twice and print what they did;
  - no autostart entry. If the binary moves, running `integrate` again repoints the launcher.
- **Logs**: with no terminal, `dl` output goes to a file under the state directory, with its path shown by `mega version` or a tray tooltip.

## decisions (2026-11-11)

- **The dfw change comes first.** It landed as dfw `v0.1.6`: `DaemonApp.OnQuit func() bool`, and a `tray.Control` made with `tray.NewControl()` and passed in `DaemonApp.Control`, offering `SetIcon`, `SetTooltip`, `Item(i).SetLabel/Enable/Disable` and `Stop`. Changes made before the tray exists are recorded and applied when it appears.
- **Quit goes through the gate.** If the workspace is idle, Quit is the orderly stop. If anything is in flight, Quit arms "shut down when idle", opens the page so the operator sees what it waits for, and the daemon keeps running. A stray click can no longer interrupt an agent.
- **The tray icon is live.** It shows the mark's needs-you state from the favicon card while the needs-you list is not empty, and the plain mark otherwise. A working state can follow once this has been lived with.
- **The daemon is how Michael normally runs mega.** It does not start at login; `mega desktop integrate` installs the launcher and no autostart entry.

## still to settle while building

- What a second `mega daemon`, or a `mega serve` beside a running daemon, reports. The port and the data directory lock already refuse it; the message should name the running daemon.
- Where the log file lives and how the operator finds it.

## not in this card

A webview window; Windows; packaging; restarting the daemon from the page.
