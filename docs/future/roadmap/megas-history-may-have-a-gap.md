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

- Mega does not show work done on a session outside it. As first built, a terminal handoff left a gap mark and a read-only native transcript view. Both were removed on 2026-11-24 by `release-only-stops-a-runtime`: Release only stops a runtime, and the terminal handoff is not supported.
- Adoption brings in one initial snapshot. Later starts and resumes skip the import.
- After lost supervision the conversation shows **Unsupervised** until the operator chooses **Resume**, available even when the session id is unknown. The Unsupervised line marks where mega lost sight, and the outcome of any unwitnessed work stays unknown. The next launch uses the exact session id.

## the failure that becomes impossible

A later import can no longer duplicate a reply. A pi turn can no longer fail because mega could not match a live message to an entry in pi's file.

## what it costs

Work done on a session outside mega is not shown in mega. Output mega never saw contributes nothing to unread. Piece 3 decides unread for output in mega's own log.
