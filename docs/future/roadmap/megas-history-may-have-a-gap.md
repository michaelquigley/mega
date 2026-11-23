---
title: mega's history may have a gap
state: evaluating
created: 2026-11-19
tags: [story, spike]
---

Mega's timeline may have a gap. Second in the sequence of `simplify-the-core`. The implemented behavior is in [Discovery and adoption](../../current/mega.md#discovery-and-adoption).

## the decision

On 2026-11-19 Michael said, "so far, i rarely take harnesses out of mega. so this doesn't bite. and even if i did, in practice i'm always looking at the last 10 turns anyway... so the complete history isn't super relevant."

On 2026-11-20 he accepted the design: "let's give it a try, yes."

- After a terminal handoff, the timeline marks the interval mega could not see. With a known session id, **More** and the gap row both open the read-only native transcript. With an unknown id, the gap stays visible and the operator gets the existing **Adopt a session** guidance. Work done in the terminal stays there.
- Adoption brings in one initial snapshot. Later starts and resumes skip the import.
- After lost supervision the conversation shows **Unsupervised** until the operator chooses **Resume**, available even when the session id is unknown. The gap leaves the outcome of any unwitnessed work unknown. The next launch uses the exact session id.

## the failure that becomes impossible

A later import can no longer duplicate a reply. A pi turn can no longer fail because mega could not match a live message to an entry in pi's file.

## what it costs

Reading an exchange from the terminal takes one extra action. Output mega never saw contributes nothing to unread. Piece 3 decides unread for output in mega's own log. Piece 9 inherits a smaller pi adapter.
