---
title: Message text is never clipped
state: evaluating
created: 2026-11-09
tags: [adapters, conversation-view, fix]
---

Both adapters clip every event's `Text` at 4000 bytes (`clip` in `internal/harness/claude/mapping.go` and `internal/harness/codex/mapping.go`), keeping the full message only in `Raw`. Michael hit it on the first long reply he needed: a code block in an assistant message ended mid-sentence with an ellipsis, the Copy control copied the clipped block, and nothing in the UI could show the rest. The clip saves nothing on disk, since `Raw` holds the whole payload anyway; it only loses what the conversation view can show.

Shape:

- User and assistant text and tool results are recorded whole. The only bounded text is the tool-use summary line (`Bash sleep 3`), which is a title by design, and it keeps a short bound.
- Long fenced code blocks in a message collapse past a threshold (around forty lines) with a "show all" control; Copy always copies the whole block. Tool results already collapse in a `details` and scroll, and stay as they are.
- Events already in a log stay as recorded; the fix is in the adapters, not a rewrite of history.
- Tests: adapter mapping tests with a message well past 4000 bytes asserting the text is whole; the browser check with the fake shim streaming a long reply with a long code block, asserting the collapse, the control, and that Copy yields the full text.
