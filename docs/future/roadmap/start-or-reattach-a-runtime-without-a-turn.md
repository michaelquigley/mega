---
title: start or reattach a runtime without a turn
state: researching
created: 2026-11-09
tags: [feature, spike]
---

Every launch in the first slice rides on a prompt, so after a restart a long-lived conversation is cold until Michael spends a turn on it. He asked for a way to start, or reattach, a runtime without burning a turn. The spec already says reopening a conversation need not start a process, let alone a model turn; this is the explicit operator gesture that starts the process and nothing else.

What it gives: a warm conversation (MCP servers connected, the session held), the header showing the live mode from the harness's own report, the mode control available, and release behaving as it does today. It complements the stored permission-mode choice rather than replacing it: writing a choice is free, starting a process is not, and twenty idle conversations should not need twenty processes to be set to auto.

Shape:

- Probe first, under `probes/claude/`, recorded in the capability report with versions: with streaming input and no user message sent, does the CLI emit `system/init`, connect MCP servers, and hold the session idle; does `setPermissionMode` take effect before the first turn; what does it cost to leave it idle; does closing input with no turn ever run exit cleanly. Codex is expected to be simpler, since `thread/resume` already precedes any turn; confirm.
- A Start control on an idle conversation (not unsupervised; resume handles that) that launches the runtime through the existing launch path with no prompt. The `starting` and `started` events record it as now.
- Nothing automatic: no auto-start on open, on restart, or on selection. Starting a process is the operator's choice, as the spec's "reopening need not start a process" requires.

Not in this card: starting every conversation of a project at once; keeping runtimes warm across service restarts.
