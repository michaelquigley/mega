---
title: a failed read after a reconnect is tried again
state: evaluating
created: 2026-11-25
tags: [defect]
---

When the stream reconnects, the page reads what the open conversation recorded while it was away: `catchUp` in `ui/src/App.tsx`. When that read fails, try it again on the same connection, on the schedule the opening read already uses and with the waiting line it already shows, until it succeeds, the connection changes, or the conversation is no longer the open one.

Seventh in the sequence of `simplify-the-core`, in place of `one-route-for-recorded-history`. The card is the whole plan; there is no work order, and no trial.

## why

When `catchUp` fails it sets the page's error and nothing tries it again on that connection. The retry that then runs reads the workspace, not the history. Later events arrive on the stream, and the stretch the page missed stays missing. The opening read of a conversation has a retry of its own. This read does not.

The piece first carded for this gave recorded events one route to the page: the stream would carry an event's number, and the page would read the event. It was built on 2026-11-25 with every gate passed, and set aside before it was committed. It was meant to take lines off the page and added about fifty-five. It also made every event two trips where one served, which a central interface reaching an environment through a relay would feel on every event. Michael: "the overhead of the additional remote hops might not play so well with this change." The work is kept on the branch `tried/one-route-for-recorded-history`. How an event reaches a page that is not on the same machine is now a question on `central-application`.

## the failure that becomes impossible

A read that fails after a reconnect can no longer leave a stretch of a conversation missing from the page.

## what does not change

The two routes by which events reach the page, and the merging of them in `ui/src/log.ts`. The refusal of a read by a service that has closed, which still counts as a dropped stream and is not retried as a failure.

## how it is shown

- a history read that fails after a reconnect is tried again on that connection, and the reply recorded while the page was away arrives;
- against the page as it is, that step fails. The branch `tried/one-route-for-recorded-history` has such a step in `checks/conversation.mjs`, written for the same fault, which can be lifted.

## how it is run

- **Tests and check steps:** one check step, as above. A test or a check that fails for any other reason is a change in behaviour, and stops and asks.
- **Documents:** the sentence in `docs/current/mega.md` on what a reconnect reads, and a `FIX:` entry in the changelog.
- **The gates are the epic's:** `make test`, all of `make checks`, `make e2e` with the harness versions and every skip named, and terminus on the working tree, then Michael. Run them from a disposable copy of the tree, so the Makefile's reinstall of the shim leaves Michael's running mega alone.
- **A terminus finding in code this card did not touch and did not make newly reachable** is matched to an existing card or captured in inbox, and listed at handoff.
- **A finding that is only wording,** or a fix that only changes this card's own check or test, is made and reported at handoff.
- **A finding whose fix would add state, a hold or an ordering rule** is brought to Michael as a question of whether to fix it at all. Anything else this card caused comes to him one at a time.

## as built

Built on 2026-11-25. `catchUp` in `ui/src/App.tsx` tries the read again on the opening read's schedule, one second doubling to at most eight, until a read succeeds, the connection changes, or the conversation is no longer the open one. Each try reads from where the page's history stands at that moment. `ui/src` grew by 40 lines, comments included.

- **The workspace's error banner no longer appears for this read.** It said "The workspace could not be read", which was untrue of a failed history read, and it set off the workspace's retry in place of the history's. The line "Loading history:" shows instead, on a loaded conversation too.
- **The check step was lifted from the branch** `tried/one-route-for-recorded-history`, with one change. Against the page as it was it fails: the page named the workspace and never read again.

The last terminus review, `12b235319ef7`, was `not_clean` on one finding, `reconnect-gap-shows-completed-tool-running`: a tool that finishes while the stream is down keeps its spinner until the history read lands. The window is open on every reconnect and this card shortened it. Michael left it. The note is on `central-application`, where what the page knows of where it stands is to be drawn.

