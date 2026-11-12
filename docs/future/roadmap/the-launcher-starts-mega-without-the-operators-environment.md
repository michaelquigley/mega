---
title: the launcher starts mega without the operator's environment
state: evaluating
created: 2026-11-11
tags: [defect]
---

Started from the desktop launcher, `mega daemon` runs with the desktop session's environment, not the operator's shell environment. Make the launcher start mega through the operator's interactive shell, and make a missing harness executable say so plainly instead of failing a launch with a raw exec error.

## why

Michael's first launch from the tray failed with `exec: "node": executable file not found in $PATH`, followed by "internal error". His PATH is assembled in `~/.bashrc`, which only an interactive shell reads; the desktop session's PATH has none of `~/local/node/bin`, `~/local/go/bin` or his tools. Every agent inherits mega's environment, so this is not only the Claude shim: a tray-launched mega hands every harness, and every command an agent runs, an environment without the operator's toolchain.

## shape

- **`mega desktop integrate` writes an entry that starts mega through the operator's shell**: the shell named by `SHELL` when integrate runs, as an interactive shell, executing the binary with `daemon`. The launcher then has the environment a new terminal has. `integrate` prints the exact `Exec` line and says why, and `docs/current` and the README explain it. A flag keeps the plain form for anyone who wants it.
- **A preflight on the workspace read**, computed and never stored, like the trust notice and the missing checkout: for each harness, whether its executable can be found (`node` and the shim for Claude, the configured `codex`). When one cannot, the workspace says which and what mega's PATH is, the launch dialog disables that harness with the reason, and a launch, prompt or Start is refused with a distinct code naming the missing executable.
- **The error path.** A failure to read native history because an executable is missing surfaces as that named condition, not as "internal error".
- The log records the PATH mega started with, once, at start-up.

## checks

A unit test for the entry's `Exec` line across bash, zsh and fish, and for the plain form. A service test with a harness whose executable is absent: the preflight flag, the refusals, the launch dialog's reason. On the live session with scratch XDG directories: a daemon started through the generated entry's command line sees an executable that exists only on the interactive shell's PATH.
