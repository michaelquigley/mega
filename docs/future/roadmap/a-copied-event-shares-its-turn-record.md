---
title: a copied event shares its turn record
state: evaluating
created: 2026-11-24
tags: [defect]
---

`Event.Clone` in `internal/model/event.go` copies an event's request, facts and raw payload, and not its turn record, so every stream subscriber's copy of a turn's end points at the same record. Copy it, with its list of differing fields, and show with a test that a change to one copy leaves another alone.

## why

Terminus first raised it in the hardening passes as `turn-record-shared-between-subscribers`, and it has come back in at least six reviews of unrelated cards since, each time as a blocking finding to be matched and set aside. It stood on the paused card `harden-requests-stops-and-restarts` with no piece assigned. The fix is a few lines and adds nothing, so it is cheaper to make than to keep matching.

Nothing is known to change a turn record after it is published, so no wrong value has been seen on a page.

## as built

Built on 2026-11-24. `Clone` copies the turn record and its list of differing fields, and a test that fails without the fix shows a change to one copy leaves another alone.
