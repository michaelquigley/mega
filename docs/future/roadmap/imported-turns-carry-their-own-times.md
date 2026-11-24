---
title: imported turns carry their own times
state: researching
created: 2026-11-16
tags: [enhancement]
---

A turn imported from outside mega, which since 2026-11-23 happens only on adoption, is stamped with the time mega imported it, not the time it happened. The event log stamps an event as it is written, and no adapter passes on the time its harness recorded. So since `conversation-timestamps`, such a turn's line says "imported 14:02" with no length, and its prompt's title says "Imported …": true, but not when the operator worked.

Each harness keeps the time in its own record: Claude Code's transcript lines carry a `timestamp`, Codex's rollout items carry theirs, and pi's session entries carry an ISO `timestamp` and each message its own milliseconds. An import could pass that time on, and the turn line could then say when the prompt was sent and how long the turn took, as it does for a turn mega ran.

Things to settle:
- **The log's order.** Events are kept in the order mega records them. An imported turn's own times come before mega's import, and before some events already in the log.
- **Disagreeing clocks.** A harness's time and mega's may disagree.
- **No time.** What an event says when its harness recorded none.

Noted by the implementer of the timestamps batch on 2026-11-16.
