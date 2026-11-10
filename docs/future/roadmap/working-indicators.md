---
title: working indicators
state: evaluating
created: 2026-11-10
tags: [enhancement]
---

First slice of the look-and-feel cluster (side panel, conversation view, operator input panel). Michael cannot see at a glance what is working: the sidebar's running mark is an 8px ring fading between full and a third opacity, projects show no state, and in the conversation view "working" is a small header label, a muted " running" on tool lines and the reply box placeholder, with nothing at the bottom where he is looking.

Decided with Michael on 2026-11-10: a rotating arc for working, elapsed time beside it, and a single project mark showing the most urgent state.

## One vocabulary, three scales

**Conversation mark** (`AttentionMark`, used in the tree and the needs-you list).

- Running is a rotating arc in the accent colour, a little larger than today's 8px ring (10 to 12px), replacing the opacity pulse.
- Nothing moves that is not live: with `prefers-reduced-motion`, or while the page shows last-known state, running is a static half-filled ring. The existing rule that a stale mark must not move stays.
- The other states keep their shapes (square for a request, filled dot for unread, red ring for an error, filled red for unsupervised, grey ring for idle), resized to match.

**Elapsed time.** A running conversation shows how long the current turn has run, in the mono register after its name: `implementer 4m`. Seconds under a minute (`45s`), minutes after (`4m`), hours and minutes past an hour (`1h 12m`). The service adds a `turn_started_at` to each conversation in the workspace read: the time the turn in flight began, held in memory beside `turn_active`, never stored, absent when no turn is active. One shared timer in the page re-renders the labels; no per-row timers.

**Project mark.** Each project line carries one mark showing the most urgent state among its conversations, in this order: request, unsupervised, error, unread, running, idle. States that need the operator outrank working, because the mark answers "does this project need me, and if not, is it busy". An idle project shows the idle ring. It is the same component at the same size, with a title naming the state and the count ("2 working"). Derived in the page from the conversations' confirmed attention; nothing new is stored.

**Activity line in the conversation view.** A persistent line pinned directly above the input panel, present whenever the conversation is not idle:

- spinner, what is happening, elapsed: `Working · Bash go test ./... · 1m 12s`; `Writing · 8s` while text streams; `Working · 3s` otherwise;
- for `starting`, `stopping` and `resuming`, that word with the spinner;
- with a request pending, no spinner and the notice colour: `Waiting for your approval` or `Waiting for your answer`;
- while stale, the last known text without motion, labelled last known.

"What is happening" comes from what the page already has: the newest tool call without a result gives its summary line, a streaming delta gives Writing. No new protocol knowledge.

**Tool rows.** An action in flight shows the same small spinner in place of the muted " running" text; " running when last seen" keeps its words and gets no motion.

## Checks

A service test that `turn_started_at` appears when a turn starts, clears when it ends, and is absent after a restart. The browser check with the fake shim: the arc on a running conversation in the tree and the needs-you list; the elapsed label advancing; the project mark following request over unread over running over idle as conversations change; the activity line through tool running, writing, a pending request and idle; no animation under reduced motion and while disconnected. Screenshots, light and dark, of the sidebar with mixed states and the conversation bottom, for Michael.

## Not in this slice

Collapsible projects, sorting and starring (side panel structure, a later slice); block borders and indentation (conversation structure, the next slice); moving controls to the input panel; a browser tab or favicon indicator, which belongs with the favicon card.
