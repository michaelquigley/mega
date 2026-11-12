---
title: environment follow-ups
state: evaluating
created: 2026-11-12
tags: [defect]
---

Four things the planning agent's review of the project-environments work found, none of which blocked it. The first is a security matter and the reason this card is a defect.

## an approval covers the file, not what the activation reads

Mega approves the text of `.mega/environment.yaml` and runs exactly that text. But `activate: devprj` goes on to read `.devprj` in the checkout if one is there, and an agent can usually write a file in its checkout without asking. The next runtime start then gets whatever that file describes, for instance a PATH that begins with a directory in the checkout. Mega's own helpers, `git` for path search and `node` for the shim, are found on that PATH and run outside any harness's permission model. So an agent that may only edit files can get code run by mega itself. The same holds for any activation that sources or consults a file in the repository.

- **Mega finds its own executables before the activation.** The resolver already captures the environment before the activation runs. Locate `git`, `node`, the shim's interpreter and the harness binaries on that earlier PATH and run them by absolute path, while the process still receives the full activated environment. An activation can then change what an agent's commands see, inside the harness's permission model, but not which program mega itself executes.
- **The approval remembers what the activation changes.** Record, with the approval, the names of the variables the activation changed when it was approved. A later resolution that changes a different set is refused until the operator has looked, with the difference shown. Names only, as everywhere else. A new `LD_PRELOAD` or `GIT_SSH_COMMAND` then needs a human.
- **Say it plainly** in `docs/current` and the README: approving the file approves its text, and an activation that reads other files in the repository is trusting them.

## the approval is bound to sixty-four bits

`environ.Fingerprint` keeps the first sixteen hex digits of a SHA-256, and that shortened value is what the registry stores and what a changed file is compared against. It is fine for display and for telling two environments apart. For the one place it is a security binding, store and compare the full digest and keep showing the short form. Existing approvals will need approving once more.

## the resolved environment is narrower than a terminal's

The base starts from the bare system PATH and the shell is interactive but not a login shell, so anything set by login scripts is absent. On Michael's machine that drops `/snap/bin` and little else, because his `~/.bashrc` sets his toolchain. For an operator whose tools come from `~/.profile` or `/etc/profile.d` (cargo, Homebrew on Linux, Nix, snaps), agents would lose them. Resolve through a login and interactive shell, and add a check that a variable set only in a profile script survives.

## a flaky test in committed code

`TestWrongSessionReportedDuringLaunchIsStopped` (`internal/service/start_test.go`) failed once under the race detector during the launcher batch and passed on rerun. Find the race or the timing assumption and remove it.

## known and left alone

Codex runs every command as a login shell, which puts its helper directory and the entries of `~/.profile` ahead of the project's bin. The Go path is right. Changing it means changing Codex's own configuration, which is the operator's.
