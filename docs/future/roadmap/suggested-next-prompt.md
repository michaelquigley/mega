---
title: suggested next prompt
state: inbox
created: 2026-11-20
tags: [feature, spike]
---

When the harness suggests a next prompt at the end of a turn, the empty reply box shows it as its placeholder, and Tab puts it in the draft as an actual prompt, to be edited or sent. Asked by Jens on 2026-11-20.

## why

Claude Code's own terminal can suggest a likely next prompt after a reply, and accepting it is one key. Often it is exactly the next step, "run the tests" or "commit this". In mega the operator types it out, or switches to the terminal to see it.

## what exists

- **No harness's suggestion is recorded.** The capability report says nothing about prompt suggestions for Claude Code, Codex or pi, and no adapter maps one.
- **The placeholder** already changes with the conversation's state: `Reply (Ctrl+Enter to send)` when a prompt can be sent, the activity line's words otherwise (`docs/current/mega.md`, the input panel).
- **Tab** has no handler in the reply box today, so the browser moves the focus out of it.

## shape

- **The service** keeps the suggestion that came with the conversation's latest turn, and only that one: a new turn, or a prompt sent, clears it. It is a reading, never made up by mega.
- **The reply box,** while it is empty and a suggestion stands, shows the suggestion as its placeholder in place of `Reply (Ctrl+Enter to send)`, marked as a suggestion, perhaps "Tab: run the tests". The activity line's words still win while the conversation is busy.
- **Tab** puts the suggestion in the draft, with the caret at its end, only while the box is empty and a suggestion shows. Otherwise Tab does what it does today, so it never traps the focus in the box. Nothing is sent until the operator sends it.
- **A harness with no suggestions** shows the placeholder as today.

## spike

- **Claude Code:** whether the Agent SDK's stream sends a prompt suggestion after a turn, under what setting, with what shape and how long after the `result`, and whether it reaches the transcript. Run on a disposable session.
- **Codex and pi:** whether either suggests a next prompt over the app-server or RPC.

Record each in the capability report with its version, as new observations. If no harness sends one, this card closes there.

## decisions for Michael

- Whether this waits for `simplify-the-core`.
- Whether it needs a section in the spec first.
- Whether taking Tab in an empty box with a suggestion showing is acceptable, or another key should accept it.
