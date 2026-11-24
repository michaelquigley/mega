---
title: adoption reaches any session in the checkout
state: inbox
created: 2026-11-23
tags: [defect]
---

A session can be adopted by its exact id whether or not discovery listed it. `Adopt` checks the id by the native read it already makes, and by no conversation holding it, and no longer by finding it in discovery's list.

## why

`Adopt` (`internal/service/adopt.go`) calls `Discover` and refuses a session that is not in the result. Codex and pi return at most 50 sessions per checkout, the newest first (`discoverLimit` in `internal/harness/codex/codex.go` and `internal/harness/pi/pi.go`), and the sessions a conversation already holds are taken out after that. So a Codex or pi session that is not among the 50 most recently used in its checkout cannot be adopted at all. The Claude adapter shows no limit of its own.

The review of `simplify-the-core` on 2026-11-23 leans on adoption as the fallback: rulings 2, 3, 4 and 7 in `docs/future/simplify-the-core-inventory.md` each end with "remove the conversation and adopt its session again". A session seconds old, or active when mega went down, is at the top of the list, so rulings 3, 4 and 7 are safe. Ruling 2 is the exposed one: a conversation left alone for weeks, in a checkout that has since gathered more than 50 sessions, could be removed and then not adopted.

On Michael's machine on 2026-11-23 no real checkout held more than 50 Codex sessions. The largest was 38. The round-trip fixture `codex-rt1` held 246.

## to settle

How the operator names a session the list does not show: a field for an id in the adopt dialog, more of the list on request, or a search. The service's side is the smaller part.

## order

Before the removal of the rules for older logs, which is the ruling that most needs the fallback to hold.
