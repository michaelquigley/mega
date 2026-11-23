---
title: mega on macos and windows
state: researching
created: 2026-11-13
tags: [story, spike]
---

Mega builds and runs on macOS, and runs on Windows inside WSL2 with the page opened from the Windows browser. Native Windows is not attempted here.

## why

The first external operators are mostly on Windows and macOS. Mega has only ever been built for linux/amd64, which is all CI builds. A cross-compile for macOS on 2026-11-13 stopped in the tray library, which needs cgo there, so whether mega's own code builds is not yet known.

## macos

To establish, then fix:

- The build: the tray through `dfw`, and anything in mega's own packages that assumes Linux. `mega desktop integrate` already refuses anywhere but Linux; the daemon and its tray may or may not be wanted on a Mac at first, and plain `mega` in a terminal is enough.
- The project environment: resolution runs the operator's login shell, which on a Mac is zsh, from a scrubbed base with a system `PATH`. Whether the capture and the scrub hold there.
- Paths: mega uses the XDG directories. Say where the config, workspace and log land on a Mac, and whether that is acceptable or should follow the platform.
- The harness stores: where Claude Code and Codex keep sessions and settings on a Mac, since discovery and adoption read them.
- CI: a macOS runner that runs `make test`, so the build does not rot. The browser checks there if they are cheap.

Michael has no Mac and expects to find a volunteer among them. Until then CI is the evidence, and the README says macOS is built and tested there but not yet used.

## windows

- WSL2 is a Linux machine, so the expectation is that mega works there as it does on `fortyfive`, with Claude Code and Codex installed inside WSL and the browser on Windows reaching `127.0.0.1:6342`. That is an expectation, not an observation.
- Michael has a Windows machine, which is also the cleanest clean machine there is. To establish on it: that loopback forwarding reaches the page, that the harnesses' logins work inside WSL, and that checkouts kept on the Windows side of the filesystem are usable or should be advised against.
- The README gets a short Windows section that says WSL2 is the supported way and native Windows is not.

Native Windows would mean replacing the file lock, the process-group handling, the shell-based environment resolution and the launcher. That is a separate card when someone needs it.

## the first pull request from outside, 2026-11-17

An external operator, `ekoby`, opened pull request 2 the day after the README went up: "build on macOS: per-OS files for the ctime read and the termios request". It is the first evidence from a real Mac, on darwin/arm64 with Go 1.27.4 and Node 24.

**What it found and fixed.** Two Linux-only names were all that kept mega from compiling: `syscall.Stat_t.Ctim` in `internal/service/firstrun.go`, which darwin spells `Ctimespec`, and `unix.TCGETS` in `cmd/mega/logging.go`, which darwin spells `TIOCGETA`. Each now sits behind a small per-OS file. Nine tests that named `/bin/true` find it on the `PATH` instead. On the Mac, `make` builds and installs, and `mega version` runs and finds Claude Code.

**Checked here by the planning agent.** The diff was read in full: fifteen files, ninety lines added, nothing but what it says. Applied to a scratch copy of the tree, Linux builds, vets and passes the three packages it touches. The internal packages cross-compile for darwin/arm64 with it and fail without it at exactly the line it names. The earlier note on this card, that a cross-compile stopped in the tray library, was only ever about `cmd/mega` without cgo; mega's own code had these two problems behind it.

**What it says is still broken on macOS,** which is this card's work list:

- `internal/environ`: the test package does not compile, since `tty_test.go` uses Linux-only terminal requests.
- `cmd/mega` `TestIntegrateAndRemoveTwice`: desktop integration is Linux-only by design, and the test does not skip.
- Two tests compare `/private/var/...` with `/var/...`, the same macOS temporary directory spelled two ways.
- `internal/service` `TestATimedOutActivationRefusesTheLaunch` takes twenty seconds instead of timing out: a backgrounded `sleep` in the activation is not killed.
- `internal/harness/codex` `TestDescendantsOutlivingTheirParentAreReported` finds no descendants.

**Two of those are the product, not the tests.**

- **Descendants are found by reading `/proc`,** in the Codex adapter and the pi adapter alike. macOS has none, so on a Mac mega cannot see a child process that outlives its runtime, and would report a clean stop it has not observed. That is `honest-runtime-state`, and it needs a darwin way to list processes before macOS can be called supported.
  - **The honest half is done** (2026-11-18, `the-page-says-more-than-it-has-seen`). A listing that cannot be read is no longer taken as "nothing survived": a release reports that leftover processes could not be checked (`harness.ErrUnchecked`). It was tested by making the listing unreadable, not on a Mac.
  - **A real listing still waits** for someone with a Mac, and is still first on the work list below.
- **The activation that is not killed** is unconfirmed. The resolver starts the shell in its own session and kills the process group on a timeout. A shell that runs background jobs in their own process group would leave them alive, and an interactive zsh, the macOS default, may well do that.

Not yet tried on a Mac at all: a real conversation, the browser checks, the round trips.

So after this merges, the honest statement is: mega builds on macOS; its tests do not all pass there; it has not been used there. The README's platforms section should say that, and CI should gain a macOS job so the build cannot rot.

Pull request 2 was merged on GitHub on 2026-11-17, as `b176718`. The README's platforms section now says macOS builds, that its tests do not all pass, that it has not been used, and names the process gap.

## the plan from here, proposed 2026-11-17

Michael asked what of the remaining work cannot be done without a Mac. Very little:

- **Needs a person and a Mac:** a real conversation with each harness, which needs a login that does not belong in CI; a trust dialog; the tray in the menu bar; a person's own shell profile; daily use.
- **Can be done through CI,** on GitHub's macOS runners, which are free for a public repository: the five failing tests; a macOS way to list processes, which the descendants test already exercises; the activation that is not killed; the browser checks with the fakes, which run in CI on no platform today; and real installs of the three harnesses, without logins, to prove the first page finds them and reads their versions.

In order: a macOS job that reports without failing the run; the five tests, the process listing first; the browser checks in CI on both platforms; the harness installs; then the job made required and the README brought up to what is true. After that, one person with a Mac runs one conversation per harness and looks at the tray.

The loop means many small commits pushed to a branch, each waiting minutes for CI. That needs a decision from Michael: the implementer may commit and push to one named branch and nowhere else, or Michael pushes each step himself.

Michael chose the first: the implementer may commit and push to the branch `darwin`, the only exception to the rule that agents do not commit. `AGENTS.md` says so. One thing stands in the way and is his to settle: from an agent's shell under mega, ssh to GitHub is refused, because the ssh agent that shell reaches holds no key. Read on 2026-11-17 by the planning agent, whose own fetch failed the same way.

Settled the same day: the implementer commits to `darwin` and Michael pushes it. No key is given to an agent.

**How the implementer reads a result without a GitHub login,** checked from an agent's shell on 2026-11-17:

- Which job and which step failed is public, through GitHub's API.
- A job's log is not: it answers 403 without a token.
- A run's annotations are public. So the macOS job turns each failing test into an annotation carrying its output, and the implementer reads those. No credential is needed, and anything that does not fit is pasted by Michael.

## parked, 2026-11-17

Michael, the same afternoon: "let's park this for now and wait for a local macos user." The macOS half waits for someone with a Mac who will use mega there, not for CI.

What stands when it is picked up again:

- **The build works** on darwin/arm64, from pull request 2.
- **The work list** is the five failing tests above, with the process listing first: the Codex and pi adapters find leftover children by reading `/proc`, which macOS does not have.
- **The CI route is worked out and unused.** A macOS job can report failing tests as annotations, which an agent can read without a login; a job's log it cannot. Michael would push a `darwin` branch that the implementer commits to. That exception to "agents do not commit" was written into `AGENTS.md` and taken out again when this was parked, so no cold implementer acts on it. It is his to grant again.
- **What only a person with a Mac can do:** a real conversation with each harness, a trust dialog, the tray, their own shell, daily use.

The README says what is true meanwhile: macOS builds, its tests do not all pass, it has not been used, and mega cannot yet see a process that outlives its runtime there.


## the clean-machine rehearsal, closed into this card on 2026-11-18

The rehearsal had its own card. It did what it was for and is closed:

- **The agent's pass,** 2026-11-13, followed the README in a scratch home with no harnesses. It found that a missing Claude Code showed as "internal error" and that the first page explained nothing. Both were fixed the same week.
- **A second machine.** Michael installed mega on his laptop, another Ubuntu 24 system, from the rewritten README, and it is in daily use there. External operators have built it from source since.

What it never reached is the pass under a fresh WSL2 on Michael's Windows machine, with real logins through to a working conversation. That is the Windows half of this card, and it is still to do.

## the application, 2026-11-20

The local macOS user this card waited for came: Jens, on macOS 26.6, arm64, who asked for mega as a standalone Mac application built with `dfw`. It is built, and `docs/current/mega.md` describes it ("The macOS application"): `make app` makes `build/mega.app`, a menu bar daemon with the Claude shim inside it and a window of its own, dfw's WKWebView. The spec gained a paragraph, "The window", that says so. On macOS this is what the inbox card `dfw-window-mode` asks for: mega in a dfw window. That card stays open for Linux.

**dfw changed for it,** in `michaelquigley/dfw` pull request 1, from Jens's fork, released in dfw v0.1.7. Its journal of the same day has the why:

- A window process is a regular application, with the standard menus, so Copy and Paste reach the page.
- It takes the focus when it opens, once more if the tray's menu hands it back as it closes.
- It holds its own UI delegate, because webview_go's was freed at once. That brought back the file chooser and new-window links.
- The tray shows no title beside its icon.
- `DaemonApp.OnReopen` answers the application being opened again.
- Mega requires v0.1.7. The release passes `OnReopen` the daemon's address.

**Two things only a Mac showed:**

- A shell script as the bundle's executable leaves the menu bar item unshown. So the application runs no shell first, unlike the Linux launcher, and relies on project environments.
- A shell start-up file that reaches protected data asks the operator on the application's behalf, once per build, and holds the shell meanwhile: about 30 seconds the first time on Jens's machine, past the 10-second default.

**The work list above, as `go test ./...` found it on Jens's Mac the same day.** Every failure below also fails at the commit before the application, so none comes from it:

- `internal/environ` does not build.
- `internal/harness` `TestReadVersion`.
- `internal/harness/claude` `TestAShimThatLeavesAChildRunningSaysSo`.
- `internal/harness/codex` `TestDescendantsOutlivingTheirParentAreReported` and `TestAMissingHelperIsNamed`.
- `internal/service` `TestATimedOutActivationRefusesTheLaunch` (40 seconds) and `TestAnActivationCannotChooseMegasPrograms`.

`TestIntegrateAndRemoveTwice` now skips off Linux. Four of the failures were not on the list before; why they fail was not looked at.

**Still to do on macOS,** beyond the work list above, which this did not touch:

- Signing and notarization, so a copy can be handed to someone else: `macos-application-signing-and-notarization`.
- Whether the page's notifications work in the window.
- The window keeping its position, which is dfw's (its card `macos-window-position`).
- Ctrl+Space, which macOS keeps for switching input sources unless the operator changes it, and which the page uses to reach the reply box.
- A real conversation through the application with each harness.
- A logout, or `quit app "mega"`, ends the daemon without the orderly stop. It waits on dfw's card `macos-daemon-quit-apple-event`. Then mega decides whether Quit's gate may decline a logout, since declining cancels it.
