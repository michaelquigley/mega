# The macOS application, as Finder starts it

What `mega.app` does when Finder starts it, which a test cannot show: launchd's environment, the menu bar, the focus, and macOS's privacy questions. dfw's own fixture, `examples/dfw-example-macos` in dfw, covers the window itself (the Edit keys, the file chooser, new-window links).

`app.sh [APP] [PORT]`, from the repository's root after `make app`, starts the application the way Finder does. That means launchd's minimal environment and no `LANG`. It uses a scratch config, data and state, and port 18471 by default, never the operator's mega. Its config raises `environment.timeout` to 90 seconds, so a slow shell shows up as a time and not as a fallback. With Finder in front, it reports:

- whether the bare launch ran the daemon and opened the window;
- what `LANG` mega chose, and its own `PATH`;
- how long the operator's shell took to resolve, and where each harness was found;
- how many menu bar items appeared;
- which application is in front;
- what opening the application again did.

Then it quits the application and removes its scratch. It reads no conversation and starts no harness. It runs the operator's login shell, as every project start does. A real session's windows open on the screen while it runs.

- `menubar.c` lists the menu bar's item windows, layer 25, with owner, position and whether each is on screen. On macOS 26 Control Center owns every item window, the application's own included, so an item is found by counting before and after, never by the application's pid.

## 2026-11-20, macOS 26.6, arm64

A spike, before the application existed. A bundle whose executable was a shell script never showed its menu bar item. That held for a script that ran the login shell first and for one that only `exec`ed the binary. The process ran, and systray's `onReady` ran. With the binary as the executable, the item appeared: two more items on screen, one per display with room for it, and one hidden on the notched built-in display. The window's focus was read with `lsappinfo front` after starting it with Finder in front.

The application, run by hand through the same steps `app.sh` now takes:

- **A bare launch** ran the daemon and opened the window.
- **`LANG`** became `en_US.UTF-8`, from `AppleLocale` `en_US`. mega's own `PATH` was `/usr/bin:/bin:/usr/sbin:/sbin`.
- **The first resolution after each build** took 34 to 35 seconds. A macOS dialog, brought up by `ssh-add --apple-load-keychain` in the operator's zsh start-up, held the shell until it was answered. The next start of the same build took 1.5 to 1.8 seconds. Under the default 10-second timeout, the first start fell back to mega's own environment and found no harness.
- **Harnesses:** Claude Code 2.1.328 and `node` were found from the login shell, and the shim in `Contents/Resources/shim/claude`. Codex and pi are not installed on that machine.
- **The window** took the front from Finder, as `mega`, `Foreground`.
- **Opening the application again** left one daemon and opened a window, in front.

`app.sh` itself, run the same day on the same build once the dialog had been answered, gave the same results, with the shell resolved in 1.8 seconds.
