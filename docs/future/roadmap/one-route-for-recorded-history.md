---
title: one route for recorded history
state: building
created: 2026-11-19
tags: [story]
---

A conversation's recorded events reach the page one way, by the history read. The stream stops carrying them. It says how far a conversation has got and carries the text still being written, and the page reads on from the last event it has until it has caught up.

Seventh in the sequence of `simplify-the-core`, and next to be built. The card is the whole plan; there is no work order. Michael tries it in use before it is committed.

## why

Today the selected conversation gets the same recorded events twice over: by history reads and on the stream. `ui/src/log.ts` merges and sorts them, remembers the numbers of events that arrived past a gap, and accounts for hidden native events while it works out how far the page has been given.

A fault follows from the seam. In `ui/src/App.tsx`, `catchUp` is the read a reconnect makes for what the page missed. When it fails it sets the page's error and nothing tries it again on that connection. Later events arrive on the stream and the missing stretch stays missing. The opening read of a conversation has a retry of its own. This one does not.

From the independent review for simplification of 2026-11-19. Michael then: "this sounds like a smart simplification." On 2026-11-25, with the smaller choice of only retrying that read beside it, he took this one, to be tried before it is committed.

## the failure that becomes impossible

A failed read of history can no longer leave a gap nothing fills, and a recorded event can no longer reach the page by two routes that have to be reconciled.

## what changes

- **The stream's frame for a recorded event carries the conversation and the event's number, and not the event.** In the service that is `Update` and the one place an event is published, in `internal/service/live.go`. The frames for the text being written, a change to the workspace and a finished shutdown are as they are.
- **The page has one job for the open conversation: read on.** It reads everything after the last event it has been given, and goes on until it has reached the newest number it knows of, from the stream or from the workspace read. One read is under way at a time, and a number that arrives meanwhile is picked up by the next. It does the same on opening, on a frame and on a reconnect.
- **A read that fails is still owed.** It is tried again, with the waiting the opening read already shows, until it succeeds or the conversation is no longer the open one.
- **Removed from the page:** `received`, `caughtUp`, `advance` and the `ahead` list in `ui/src/log.ts`, and `catchUp` as a path of its own in `ui/src/App.tsx`. What a read returns says how far the page has been given, hidden native events included, so nothing is worked out from gaps.
- **Removed from the service:** the copy of an event made for each subscriber when it is published.

## what it costs

Each burst of finished output is a read from the page where today it arrives on the stream. On the same machine a read that returns a few events costs about a millisecond. Over a share from another machine the extra trip is longer. The text being written still streams.

## what does not change

The history read and what it returns. Scrolling back to earlier turns. Showing and hiding native events. The reading position and what moves it. Conversations that are not open, which cost no reads. A read refused because the service has closed still counts as a dropped stream. What the page knows of the workspace is `the-page-has-one-value-for-where-it-stands`, and is not touched here.

## how it is shown

- an event recorded for the open conversation appears on the page once, in order, with nothing sent for it on the stream but its number;
- several events recorded close together are brought by fewer reads than there are events;
- a history read that fails after a reconnect is tried again on that connection, and the stretch it covers arrives; against the page as it was, that step fails;
- after a reconnect the page reads on from where it was to where the conversation is now;
- with native events hidden, a run of them leaves no gap and holds nothing back;
- the text being written streams, and ends when its reply has been read;
- a conversation that is not open draws no history read;
- nothing in the tree sends a recorded event on the stream.

## the trial

The implementer leaves the piece in the working tree, with every gate passed, and says so. Michael builds it, restarts on it, and works in it for a while before it is committed or backed out. Nothing written to a log or the registry changes, so the old build reads the same files. He watches:

- whether a row appears as promptly as before when an agent finishes a step, above all in a turn with many tool calls;
- the handover from text being written to the finished reply, for a flicker or a repeat;
- a restart of mega with a conversation open, and what the page shows as it reconnects;
- the reading position and the unread marks, which follow what the page has been given.

## how it is run

- **It starts by listing every way an event or its number reaches the page today,** and every reader of the conversation's log on the page. One that needs an event before a read could bring it is brought to Michael.
- **Documents:** the stream's frames in the HTTP reference of `docs/current/mega.md`, the part of "The UI" that describes how a conversation's history is held, and the changelog. The spec's paragraph "Recorded history has one route to the page" is the planning agent's to retire once Michael has kept the piece.
- **Tests and check steps of the removed behaviour go, or are rewritten to the rule.** The report names each. A test or a check that fails for any other reason is a change in behaviour, and stops and asks.
- **The gates are the epic's:** `make test`, all of `make checks`, `make e2e` with the harness versions and every skip named, and terminus on the working tree, then Michael. Run them from a disposable copy of the tree, so the Makefile's reinstall of the shim leaves Michael's running mega alone.
- **A terminus finding in code this card did not touch and did not make newly reachable** is matched to an existing card or captured in inbox, and listed at handoff.
- **A finding that is only wording,** in a doc, a test or the page's own text, is fixed and reported at handoff.
- **A finding whose fix would add state, a hold or an ordering rule** is brought to Michael as a question of whether to fix it at all. Anything else this card caused comes to him one at a time.
- **Size.** `ui/src/log.ts` was 124 lines and `internal/service` 6,096 non-test lines when piece 6 was built. The report gives both afterwards. This piece is on the page, so the service will hardly move.

## order

Depends on nothing. It comes before `the-page-has-one-value-for-where-it-stands`, which is easier once a conversation's events have one route. Lands on `main`.
