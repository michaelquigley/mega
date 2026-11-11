---
title: start follow-ups
state: evaluating
created: 2026-11-10
tags: [enhancement]
---

Two things left open when Start landed (terminus round `fb4eed0d1bb6`, clean):

- After a Claude Start, the first turn adds a history row, "Permission mode X: reported by claude at the first turn", even when the mode is what was requested. Keep recording the report, since it is the harness's first word on the mode, but do not draw a row for a report that only confirms the requested mode; the header dropping "(requested)" is the visible change.
- If Claude never answers initialize after a Start, the conversation stays `starting` until released. Give the idle launch a timeout (about a minute) that records an error through the transition helper and leaves the conversation with no runtime, so it can be started again.

## background

Idle cost measured in the Start spike (Claude Code 2.1.318, codex-cli 0.166.0): about 200 to 250 MB per warm conversation, counting the CLI or app-server, the shim's Node process for Claude, and the operator's MCP servers at 5 to 30 MB each.
