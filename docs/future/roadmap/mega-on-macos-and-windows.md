---
title: mega on macos and windows
state: researching
created: 2026-11-13
tags: [story, spike]
---

Mega builds and runs on macOS, and runs on Windows inside WSL2 with the page opened from the Windows browser. Native Windows is not attempted here.

## why

Michael's coworkers are mostly on Windows and macOS. Mega has only ever been built for linux/amd64, which is all CI builds. A cross-compile for macOS on 2026-11-13 stopped in the tray library, which needs cgo there, so whether mega's own code builds is not yet known.

## macos

To establish, then fix:

- The build: the tray through `dfw`, and anything in mega's own packages that assumes Linux. `mega desktop integrate` already refuses anywhere but Linux; the daemon and its tray may or may not be wanted on a Mac at first, and plain `mega` in a terminal is enough.
- The project environment: resolution runs the operator's login shell, which on a Mac is zsh, from a scrubbed base with a system `PATH`. Whether the capture and the scrub hold there.
- Paths: mega uses the XDG directories. Say where the config, workspace and log land on a Mac, and whether that is acceptable or should follow the platform.
- The harness stores: where Claude Code and Codex keep sessions and settings on a Mac, since discovery and adoption read them.
- CI: a macOS runner that runs `make test`, so the build does not rot. The browser checks there if they are cheap.

Michael has no Mac and expects to find a volunteer among his coworkers. Until then CI is the evidence, and the README says macOS is built and tested there but not yet used.

## windows

- WSL2 is a Linux machine, so the expectation is that mega works there as it does on `fortyfive`, with Claude Code and Codex installed inside WSL and the browser on Windows reaching `127.0.0.1:8470`. That is an expectation, not an observation.
- Michael has a Windows machine, which also serves as the cleanest clean machine there is for `clean-machine-rehearsal`. To establish on it: that loopback forwarding reaches the page, that the harnesses' logins work inside WSL, and that checkouts kept on the Windows side of the filesystem are usable or should be advised against.
- The README gets a short Windows section that says WSL2 is the supported way and native Windows is not.

Native Windows would mean replacing the file lock, the process-group handling, the shell-based environment resolution and the launcher. That is a separate card when someone needs it.
