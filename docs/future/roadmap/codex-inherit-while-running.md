---
title: Codex can return to inherit while running
state: inbox
created: 2026-11-10
tags: [codex, conversation-view]
---

Choosing inherit for model and effort is refused while a conversation runs, for both harnesses, because neither returns to the operator's settings mid-session. Since the model-and-effort batch, Codex inherit means "send the config's current model and effort on every turn", so it can be chosen on a running Codex conversation and applied at the next prompt like any other Codex change, with the header showing "next prompt". Claude stays refused: its reset calls go to Claude Code's built-in defaults, not the operator's settings, so there is no honest inherit to offer.

Small; take it with the next conversation-view batch.
