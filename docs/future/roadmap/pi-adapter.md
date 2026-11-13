---
title: pi adapter
state: researching
created: 2026-11-06
tags: [feature]
---

Add pi as the third harness: launched, prompted, interrupted, resumed, discovered and adopted as Claude and Codex conversations are, through an adapter that speaks pi's JSONL RPC over stdio from Go. Part of the epic `a-coworker-can-install-mega`, pulled forward from the spec's deferred list by Michael on 2026-11-13.

## what is already known

The capability report's pi section and `probes/pi/` hold the observed behaviour at pi 1.1.0, which is the version installed on 2026-11-13. In short:

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
- **A fixture** `pi-rt1` beside the others, and `MEGA_E2E_PI_FIXTURE` for the round trips. The round trips use Michael's own pi default, `ollama/qwen3.8-27b`, served from his host `eleven`, by his decision of 2026-11-13: it is a fair example of local inference, and it is what he will show. The probes used his Codex login through pi only because no Ollama server was up that day. Mega changes nothing in pi's configuration to reach it, and the round trips skip with a plain message when the server does not answer. A local model is slower and less exact than the hosted ones, so the round trips assert on markers and protocol events, never on phrasing.
- **The canon.** `permissions-never-weakened` gains pi's sentences: no `--approve`, no answering a dialog, no trust written.
- **The documents.** The spec's deferred list loses pi, and `docs/current/mega.md`, the glossary and the README gain it.

## order of work

1. Establish the open points and record them in the capability report.
2. The adapter and its fake: launch, turn, dialogs, interrupt, release, exact resume.
3. Discovery, adoption and native history import.
4. The page and the documents.
