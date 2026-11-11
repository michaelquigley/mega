---
title: start or reattach a runtime without a turn
state: researching
created: 2026-11-09
tags: [feature, spike]
---

Every launch rides on a prompt, so after a restart a long-lived conversation is cold until Michael spends a turn on it. Give him an explicit gesture that starts, or reattaches, a conversation's runtime without sending anything: a Start control on a conversation with no runtime. Nothing starts on its own.

## why

A warm conversation has its MCP servers connected and its session held, shows the live model and mode from the harness's own report, and takes mode, model and effort changes at once rather than "at the next launch". It complements the stored choices rather than replacing them: writing a choice is free, starting a process is not, and twenty idle conversations should not need twenty processes. The spec already says reopening a conversation need not start a process, let alone a model turn; this is the operator choosing to start one.

## what earlier probes already showed

- A fresh Claude session started with streaming input and no prompt answers `supportedModels()` in about 400 ms, costs no turn, leaves no transcript and does not appear in discovery (model-and-effort batch, Claude Code 2.1.318).
- `setPermissionMode` takes effect from the next tool call even mid-turn (permission-mode batch).
- Codex `thread/resume` restores history and starts MCP with no turn (capability report, X4 and the idle row).
- Closing input while a background job runs makes Claude wait for the job and take one more turn before exiting (background-jobs batch).

## spike

What is not yet known, to record in the capability report with versions: whether a *resumed* Claude session with no prompt emits `system/init`, connects MCP servers and holds idle indefinitely; whether anything is written to the transcript by an idle resume; what closing input on a never-prompted resumed session does and how it exits; what an idle runtime costs in memory per harness, so the card can say how many is reasonable.

## shape

- **A Start control** in the input panel's control bar on a conversation with no runtime that is not unsupervised (Resume handles that case, and imports first). It launches through the existing path with no prompt, importing native history first exactly as a prompt to a stopped conversation does, so a start after a terminal visit is not behind.
- **Recorded as today.** `starting` then `started` through the transition helper; the session binding rules are unchanged, since a start of a conversation with a known session is a resume by exact id and a conversation with none cannot be started without a prompt.
- **Honest state.** A started conversation with no turn is idle with a runtime: the mark stays the idle dot, the header shows the reported facts, and the control bar offers Release. Background jobs cannot exist yet, since nothing has run.
- **Nothing automatic.** No start on open, on selection, or on service restart.

## not in this card

Starting every conversation of a project at once; keeping runtimes warm across a service restart; a conversation with no session yet (it needs its opening prompt).
