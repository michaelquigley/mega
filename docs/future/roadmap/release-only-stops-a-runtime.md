---
title: release only stops a runtime
state: evaluating
created: 2026-11-24
tags: [story]
---

Release stops a conversation's runtime and means nothing more. The handoff to a terminal, the gap flag and the native transcript view are removed. Mega neither marks nor shows work done on a session outside it.

A removal from the review of `simplify-the-core` on 2026-11-23, built before piece 3. It is rulings 1 and 3 in `docs/future/simplify-the-core-inventory.md`, and the spec's section "When things stop" has the rule.

## why

Piece 2 gave Release a second meaning, a handoff to a terminal. A stop asked for by Release is recorded with a gap flag, the timeline says history may have a gap, and a dialog reads the session's own transcript on demand. The same view was offered after lost supervision. They drew about eleven of piece 2's 27 design findings.

Michael does not take sessions out of mega. On 2026-11-23: "if it can simplify the implementation then there is no real reason to release a runtime so that it can work in the terminal, with the ability to resume it in mega. i'm pretty sure i'd be fine with release just being 'stop this runtime'." His rule for the operator: "don't interact with the session outside of mega, if you want to keep the session intact inside mega." And for work a harness finishes after mega goes down mid-turn, he took the simplest answer: mega does not fetch it or show it, and the Unsupervised line marks the place.

## the failure that becomes impossible

A stop can no longer be recorded as two different things, and a session's own history has one way into mega, which is adoption.

## what changes

- **Removed from the service:** the `handoff` field of `live`, the `Gap` field of `model.Event` and both places that set it, and `NativeTranscript` with its route and handler. `HistoryBoundary` keeps its one remaining case, an `unsupervised` line.
- **Removed from the page:** the native transcript dialog, "View native transcript" in **More** and on a row, and the sentence that history may have a gap.
- **Removed from the adapters and the shim:** whatever only the transcript view used. What adoption's snapshot and Look use stays. The line between them is found by reading the callers.
- **The release dialog stays as a confirmation.** It says Release stops the runtime, and that the next prompt or Start brings the conversation back on the same session. It keeps its notices about a pending pi dialog and about background jobs. It loses the terminal command, the copy button and the advice about closing a terminal.
- **The round trips and the version probe keep their checks of native speech,** read through adoption's snapshot where they read the transcript today: `internal/e2e` and `probes/versions/driver.mjs`. What they assert about speech does not change.
- **A `gap` field in a log already written is ignored.** Its row shows as any stop does. That is ruling 2.

## what does not change

Release's name, its route, and how it stops a runtime. `stopping` until the exit is seen. Unsupervised and Resume. Discovery, adoption and Look. Everything else.

## how it is shown

- a release and then a prompt leave `stopping`, `stopped`, `starting` and `started` in the log with no gap flag, and the turn before and the turn after still page and time as two turns;
- lost supervision still records one `unsupervised` line, and it still separates the turn before it from the next for paging and timing;
- the transcript route is gone, and **More** offers no transcript;
- a log holding a `gap` field from an earlier build opens and shows its rows, with no gap sentence;
- the round trips for all three harnesses, with their assertions about native speech intact;
- the conversation and panel suites, with the steps for the transcript and the gap row removed and named in the report.

## how it is run

- **The canon is brought in step before the build,** by the planning agent, uncommitted in terminus-canon: the transcript sentence leaves `exact-id-resume`, `honest-runtime-state` and `bookkeeping-never-creates-attention`.
- **Documents:** the release, gap and transcript passages in `docs/current/mega.md`, its HTTP reference, and the glossary's entries for Release, Unsupervised and the transcript. The changelog's unreleased entry for piece 2 is amended, since the transcript view was never in a release. The card `megas-history-may-have-a-gap` is left for the planning agent.
- **Tests and check steps of the removed behaviour go, or are rewritten to the rule.** The report names each. A test or a check that fails for any other reason is a change in behaviour, and stops and asks.
- **The gates are the epic's:** `make test`, all of `make checks`, `make e2e` with the harness versions and every skip named, and terminus on the working tree, then Michael.
- **A terminus finding in code this card did not touch and did not make newly reachable** is matched to an existing card or captured in inbox, and listed at handoff.
- **A finding that is only wording** in a doc or a test of this card's own is fixed and reported at handoff.
- **A finding whose fix would add state, a hold or an ordering rule** is brought to Michael as a question of whether to fix it at all. Anything else this card caused comes to him one at a time.
- **Size.** `internal/service` was 6,447 non-test lines at `75fc0b4`. The report gives the figure afterwards, and what was deleted outright.

## as built

Built on 2026-11-24. `internal/service` went from 6,447 non-test lines to 6,419, and the whole tree lost 280 lines net. Nothing came out of the adapters or the shim: the transcript view used the same read adoption makes.

- **A release in the middle of a turn is left as any runtime exit in the middle of a turn.** The log gets no turn end, so the page and the paging join that turn to the next, and the next turn's time is counted from the old prompt. The gap flag had separated that one case. Michael ruled to leave it and add nothing. The question under it, that a runtime's exit ends a turn in the service's memory and not in the log, is noted on `a-turn-is-inferred-from-a-message`.
- **A release whose exit was logged but whose registry write failed comes back Unsupervised,** like any other stop. Before, the gap-flagged stop stood in for that line.
- **The round trips and the version probe check native speech by removing the conversation, adopting its session again and reading the snapshot.** The probe passed 64 of 64 on Claude Code 2.1.326.

The last terminus review, `b50464f4bea4`, was `not_clean` on the one finding Michael ruled on, `release-merges-unfinished-turn-with-next-turn`.
