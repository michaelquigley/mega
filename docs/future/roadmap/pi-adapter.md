---
title: pi adapter
state: evaluating
created: 2026-11-06
tags: [feature]
---

Add pi as the third harness: launched, prompted, interrupted, resumed, discovered and adopted as Claude and Codex conversations are, through an adapter that speaks pi's JSONL RPC over stdio from Go. Part of the epic for an external operator installing mega, whose card was closed on 2026-11-18, pulled forward from the spec's deferred list by Michael on 2026-11-13.

## what is already known

The capability report's pi section and `probes/pi/` hold the observed behaviour at pi 1.1.0. **The installed pi is now 1.5.2**, read on 2026-11-15; it was 1.1.0 two days earlier. So everything below is what 1.1.0 did, and the first step is to rerun the probes against 1.5.2 and record what changed. One change is already visible by file name alone: pi's configuration now holds `mcp.json` and an MCP cache, so "no MCP list" further down may no longer be true. In short, at 1.1.0:

- **Launch and resume.** `pi --mode rpc --session-id <uuid>` creates a persisted session and `--session <id>` reopens an exact one. `get_state` gives the session id, its file, the model and whether it is streaming.
- **A turn.** A `prompt` answered `started` is acceptance only. `agent_settled` ends the turn, and `stopReason` tells success from error. `tool_execution_start` comes before any dialog, so it does not mean a command is running.
- **Dialogs.** The only input pi asks for is an `extension_ui_request`: select, confirm, input or editor. Nothing marks one as a permission request.
- **Sessions.** One file per session under `~/.pi/agent/sessions/`, in a directory named for the checkout, entries linked by `parentId`. No attach, subscribe or list in the protocol, so discovery is a directory scan.
- **Hazards.** Two processes on one session double-write the file with no lock. An `abort` sent while a dialog is pending gets no answer until the dialog is answered. A dialog nobody answers blocks the run for good, though closing stdin still exits cleanly.

## what pi changes about mega's promises

- **pi has no permission model.** It runs tools without asking unless an extension asks. Under mega that stays exactly so, because mega preserves native behaviour and adds none of its own. The launch dialog and the conversation must say it in plain words, since an operator used to Claude and Codex will assume otherwise.
- **Every dialog is a question.** Mega cannot tell an extension's approval from any other dialog, so a pi request is never labelled an approval and never offers "allow for the session".
- **Mega never passes `--approve`.** That flag trusts the project for the run, and RPC mode cannot show pi's own trust prompt. The probes passed it in a fixture; mega must not. A project the operator has not trusted in pi's own terminal runs without its project extensions, and mega says so where it can read that without writing anything.
- **Mega never answers a dialog to finish an interrupt or a shutdown.** An interrupt that waits on a pending dialog says that it is waiting and on what. The shutdown gate already counts a pending request.
- **One writer.** With no lock in pi, mega's own ownership rule is the only protection: a session mega runs is not opened elsewhere, and a session open in a terminal is adopted only after that terminal has let go. Mega cannot detect an idle terminal holding the file, and the interface must not suggest it can.

## to establish before building

From the version-matched documents pi ships and a probe where they do not settle it:

- The model list, the thinking or effort levels, and how each is set and reported, so a pi conversation's request is concrete as the others are. What pi's own settings name is the source of the copy.
- Whether `@path` in a prompt attaches the file, for the harness table.
- How pi reports its version.
- Which chain of a branched session file is the conversation, for import.
- How the adapter finds `pi` and its interpreter on the path from before activation, as it does for the others.

## shape

- **The adapter** in `internal/harness/pi`, with the protocol confined to it, and a committed fake beside it as Codex has, so the checks and the Go tests need no model.
- **The service** treats it as any harness. No permission mode, no MCP list, no background jobs: each absent, not pretended.
- **The page** gains pi in the harness table, with its vendor's mark held as the other two are, or its name in words until a mark from the vendor's own assets is to hand.
- **A fixture** `pi-rt1` beside the others, and `MEGA_E2E_PI_FIXTURE` for the round trips. The fixture is an example project and nothing more: a small disposable git repository with an instructions file naming a marker and an extension that asks before running a command, as the other two fixtures are. The implementer creates it. Michael accepts pi's trust prompt for it once, in pi's own terminal, since mega never trusts a project for him. The round trips use Michael's own pi default, `qwen3.8-27b`, served from his host `eleven`, by his decision of 2026-11-13: it is a fair example of local inference, and it is what he will show. It does not run on Ollama, as this card first said; he corrected that on 2026-11-15. It runs on a custom inference engine for a 5090. The adapter takes the model's name and provider from pi's own settings and assumes neither. The probes used his Codex login through pi only because the local model was not reachable that day. Mega changes nothing in pi's configuration to reach it, and the round trips skip with a plain message when the server does not answer. A local model is slower and less exact than the hosted ones, so the round trips assert on markers and protocol events, never on phrasing.
- **The canon.** `permissions-never-weakened` gains pi's sentences: no `--approve`, no answering a dialog, no trust written.
- **The documents.** The spec's deferred list loses pi, and `docs/current/mega.md`, the glossary and the README gain it.

## order of work

1. Establish the open points and record them in the capability report.
2. The adapter and its fake: launch, turn, dialogs, interrupt, release, exact resume.
3. Discovery, adoption and native history import.
4. The page and the documents.

## decisions at pi 1.5.2, 2026-11-16

The probes were rerun against 1.5.2 before anything was built. What changed, and what Michael decided:

- **A pending dialog now holds pi open.** At 1.5.2 pi ignores a closed input and a first stop signal while a dialog waits; only a second signal ends it. Michael's decision: release always works, as it does for the other harnesses. With a dialog pending, release ends pi's process, by closing its input, then signals with bounded waits, then the process group. The request is recorded as never answered, the command it gates does not run, and the release dialog says so before the operator confirms. Mega's own exit uses the same path. Ending the process is not answering the dialog.
- **Every pi dialog offers Dismiss,** which sends pi's own cancel when the operator presses it, as Escape does in pi's terminal. Mega never sends it.
- **An interrupt with a dialog pending** says which dialog it is waiting on and stays waiting. The shutdown gate is unchanged.
- **pi has MCP built in now** and starts the operator's servers itself. It reports them only as free text, so a pi conversation shows no MCP list. With no permission model, a pi conversation can call those servers without asking. That is native, and preserved.
- **Effort comes from the model, not the settings.** The local model has no reasoning, so its only level is off.
- **`@path` is not expanded** over RPC, as with Codex.
- **A resume passes the session file's path,** since a bare id that matches another project's session makes pi ask a question on its input.
- **Snapshots copy `models.json`** though it holds an API key for the provider on `eleven`: it is configuration, and the most valuable part of pi's. A snapshot can therefore hold a secret that lives inside a configuration file, which matters when `a-workspace-that-can-be-rebuilt-elsewhere` moves anything off the machine.
- **Trust needed nothing.** The fixture was already trusted from a terminal visit, and 1.5.2 honours a saved decision in RPC mode.

The four round trips pass on pi 1.5.2 with `qwen3.8-27b` on `eleven`, in 36 seconds together.

## built, 2026-11-16

All three steps are done and reviewed to clean on terminus's seventh pass, with the round trips passing on pi 1.5.2 and the local model. Two things remain open after the build:

- **The terminal round trip at 1.5.2.** Michael ran it on 2026-11-16 and reported that it "seemed to work just fine": a managed session opened in pi's own terminal, continued there, and resumed in mega. Not instrumented, so it is his observation and not a recorded probe run.
- **pi starts the operator's MCP servers at every launch,** as Claude Code and Codex each start their own. Lookups of the model list and settings run pi with `--no-mcp` and start none.
