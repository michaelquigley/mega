---
title: readme for newcomers
state: researching
created: 2026-11-13
tags: [documentation]
---

Rewrite the README for someone who has just seen mega demonstrated and wants to try it: what it is, what it looks like, what they need, and the shortest path to a first conversation. What only a contributor needs moves out.

## why

The README was written while the thing was being built, for the people building it. It has no picture of the page. Its install section carries a paragraph on npm audit findings and the browser checks before it reaches configuration. The first-run path is correct and hard to find.

## shape

- **Top.** One paragraph on what mega is and is not, then a screenshot of a workspace with a few conversations in different states.
- **Try it.** Requirements as a short list with the tested ranges from `harness-version-tolerance`, then clone, build, run, register a project, launch. One block per platform where they differ, including the WSL2 note.
- **What to expect.** The safety posture in a few lines: loopback only, your own logins, permissions as the harness has them, what mega stores and where.
- **Where things go wrong.** The few failures the rehearsal found, each with its message and its fix.
- **Moved out.** The checks, the audit notes, `make clean`'s details and the e2e fixtures go to a contributor document that the README links.

Written after `clean-machine-rehearsal` and `harness-version-tolerance`, since both change what it must say. The screenshot comes from a scratch workspace with generated conversations, never from Michael's own.

## from the clean-machine rehearsal, 2026-11-13

What following today's README, top to bottom, left a newcomer to guess. The rehearsal used a scratch home with Go and Node only, and a fresh clone from GitHub. The full record is in `docs/journal/2026-11-13.md`.

- **`mega` is not found after `make`.** `make` installs it into `$(go env GOPATH)/bin`, and the next line runs `mega`, which gives "bash: line 1: mega: command not found" unless that directory is on `PATH`. The README never says to add it.
- **How to get the harnesses.** It names Claude Code 2.1.318 at `~/.local/share/claude/versions/2.1.318` and codex-cli 0.166.0, but not how to install either build, nor that the usual Claude Code install gives today's build in another directory, which mega then cannot use (`a-missing-claude-is-named`).
- **Logging in.** "Your own Claude and Codex logins" does not say to run `claude` and log in, and `codex` and log in, before mega. It also does not say that Claude's trust dialog has to be accepted once in each checkout, which the project page asks for.
- **Where the harnesses must be found.** Every agent runs in a login, interactive shell started in the checkout. `node`, `claude` and `codex` must be on the `PATH` that shell builds, not only on the one in the terminal that started mega. The README says this under Project environments, well after the steps. Without it, the dialog's "'claude' cannot run: 'node' was not found on the project's PATH" is the first a newcomer hears of it, though `node` is on their `PATH`.
- **The port.** There is no flag for it; only `listen:` in the config file changes it. The config example's `shim: /path/to/mega/shim/claude` is a placeholder that breaks Claude if the block is copied whole to change the port: "'claude' cannot run: claude shim: stat /path/to/mega/shim/claude/shim.mjs: no such file or directory". The example could show only what a newcomer would change, with the harness keys as optional overrides.
- **The missing-harness paragraph is not true for Claude yet.** "the new-conversation dialog marks that harness as unable to run" holds for Codex and for `node`, not for Claude's executable, until `a-missing-claude-is-named` lands.
- **What went well.** The clone and `make` worked first time in 22 s, with no warning and no vulnerability reported. A relative or missing checkout path, a non-loopback `listen`, and a missing `codex` each drew a refusal that says what is wrong.
