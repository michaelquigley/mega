---
title: unread is output beyond where you have read
state: evaluating
created: 2026-11-19
tags: [story]
---

Output counts when it is recorded, and only reading moves the reading position. Mega stops working out which output is new.

Third in the sequence of `simplify-the-core`. Built on 2026-11-24.

## why

Today output is counted when its turn ends. A marker in the registry, `output_seq`, holds the result. Output is buffered through a turn so the marker moves only at its end, output flagged as after its turn is counted at once, and a start rebuilds the marker from the log in case a crash kept it from the registry. About seven of the hardening findings were about what counts as unread, and the rebuild at start-up failed in three passes running. Terminus has raised it on every card since.

Decided by Michael on 2026-11-19, and revised the same day so that nothing but a read moves the reading position: an import could pass over a reply he had not seen.

## the failure that becomes impossible

An unread mark can no longer be miscounted by a rebuild at start-up, because nothing is rebuilt: the mark is the newest output in the log set against where the operator has read.

## what changes

- **The newest agent output is kept in memory, by one function.** It is applied to each event as it is recorded and to each event as a log is opened, so the running service and a start cannot count differently. Agent output is assistant text, tool calls and their results, of any origin. Native events and the operator's prompts are not.
- **A conversation is unread when its newest output is beyond its reading position.** A conversation with a turn in flight and no pending request still reads as running whatever output it holds, and a pending request still comes first.
- **Removed:**
  - `OutputSeq` from the conversation's record, from the server's view and from the page's type;
  - `markUnreadLocked`, `endTurnLocked` and the buffering of a turn's output in `turnOutput`;
  - `finishedOutput` and the part of `recover` that uses it;
  - the counting of output flagged as after its turn;
  - adoption's own write of the reading position, in `internal/service/adopt.go`.
- **The workspace is announced as changed when a turn ends and when output arrives outside a turn,** as now, and not for each event of a turn in flight. The marker's registry write used to make that announcement.
- **An older registry still has `output_seq`.** It loads as it is, since the registry's reader ignores a key it does not know, and the key is gone at the next write. That was checked on 2026-11-24. The registry's version does not change.

## what it costs

A session just adopted shows as unread until it is opened. Adopting one opens it on the page, so the mark clears at once in ordinary use. Work a harness did while mega could not see it contributes nothing to the mark.

## what does not change

The order of attention. What moves the reading position on the page. The flag for output after its turn stays, because it also stops late output from opening a turn; `a-turn-is-inferred-from-a-message` removes it. A build from before this piece, opened on the same workspace, finds no marker and counts unread its own way from the log.

## how it is shown

- agent output recorded before an unclean restart, beyond the reading position, shows unread after Resume even if mega missed the turn's end;
- a conversation with an unread reply, stopped and started again without being opened, is still on the needs-you list;
- a session just adopted shows as unread until it is opened;
- two unread conversations, the older then given more output, are in the order of each one's newest output;
- a conversation that is not the one on the page gains its unread mark on the rail when its turn ends, from output alone;
- a turn in flight announces no workspace change for each of its events;
- a registry written by an earlier build, holding `output_seq`, opens, and the key is gone after the next write;
- nothing in the tree names `OutputSeq`, `markUnreadLocked` or `finishedOutput`.

## how it is run

- **It starts by listing every reader and writer of the unread marker,** in the service, the server and the page. One that does something besides deciding unread is brought to Michael before it is removed.
- **The canon is brought in step before the build,** by the planning agent, on Michael's word: `bookkeeping-never-creates-attention` says today that output counts once its turn has ended and that an adoption snapshot starts read.
- **Documents:** the "Attention" section of `docs/current/mega.md`, the glossary's entries for attention and the reading position, the "Data" section's description of the registry, and the changelog. On the paused hardening card, `recovery-counts-non-exit-errors` goes with this piece; say so there. The spec's paragraph "Unread is output beyond where you have read" is the planning agent's to retire.
- **Tests and check steps of the removed behaviour go, or are rewritten to the rule.** The report names each. A test or a check that fails for any other reason is a change in behaviour, and stops and asks.
- **The gates are the epic's:** `make test`, all of `make checks`, `make e2e` with the harness versions and every skip named, and terminus on the working tree, then Michael.
- **A terminus finding in code this card did not touch and did not make newly reachable** is matched to an existing card or captured in inbox, and listed at handoff.
- **A finding that is only wording** in a doc or a test of this card's own is fixed and reported at handoff.
- **A finding whose fix would add state, a hold or an ordering rule** is brought to Michael as a question of whether to fix it at all. Anything else this card caused comes to him one at a time.
- **Size.** `internal/service` was 6,281 non-test lines when the batch of small defects was built. The report gives the figure afterwards, and what was deleted outright.

## as built

Built on 2026-11-24. `internal/service` went from 6,281 non-test lines to 6,163. One rule follows the sequence and the time of the newest assistant message, tool call or tool result, on append and when a log is opened. The registry's marker, the turn buffer, the rebuild at start-up and adoption's write of the reading position are gone. An older registry keeps its reading position and drops the old marker at its next write.

- **Left as it was, on Michael's ruling:** opening an adopted conversation can acknowledge an earlier reply that sits above the viewport while a long unanswered prompt fills it. Terminus raised it as `opening-acknowledges-output-outside-viewport`.
- **The implementer ran the gates from a disposable copy of the tree,** so the Makefile's reinstall of the shim left Michael's running mega alone.

The last terminus review, `4eae4811f761`, was `not_clean` on that finding and on `answer-in-flight-still-listed-as-pending`, which is on the paused hardening card.

## order

Depends on nothing in code. Lands on `main`.
