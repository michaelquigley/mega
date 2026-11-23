---
title: a first launch holds nothing back
state: evaluating
created: 2026-11-23
tags: [story]
---

Remove the queue that holds an adapter's callbacks while a conversation with no session launches. What the adapter reports is delivered as it arrives, as on every other launch.

A removal from the review of `simplify-the-core` on 2026-11-23, built before piece 3. Michael, 2026-11-23: "yes. let's take it out."

## why

Piece 2's terminus reviews added the queue over consecutive rounds, each answering a hole in the fix before it. While a conversation with no session launches, every callback is held. When `Launch` returns, each held start report is compared with the id the runtime gives, and the callbacks are delivered or discarded.

The case it guards is a start report that names another session than the one `Launch` returns. No harness can produce it:

- **Codex.** `internal/harness/codex/codex.go` sets the runtime's thread id and the start report's session from one value, `started.Thread.ID`.
- **pi.** `internal/harness/pi/pi.go` does the same with its session id.
- **Claude.** The adapter chooses a new session's id and passes it with `--session-id`. Claude names no session until a turn starts, and `StartConversation` sends the opening prompt after `Launch` has returned and the id is bound. From there `started` compares every report with the bound session, as it does on a resume, and the adapter's `sameSession` stops relaying at the first frame that differs.

While the queue holds, the harness has been given nothing to answer. Only a test fake that reports one session and returns another exercises it.

## the failure that becomes impossible

A callback can no longer be held, reordered or dropped by a launch in progress. Two of the rounds that built the queue were about exactly that: a callback that reached the log while the check was pending, and an exit that settled before the refusal was recorded.

## what changes

- **Removed from `internal/service/live.go`:** the `early` and `earlyExit` fields, `launchCallback`, `report`, `confirmFirstStart`, and the copies made only so a callback could be held.
- **Removed from `startRuntime` in `internal/service/conversation.go`:** the handling of held callbacks and of an exit held during the launch.
- **Kept:** a new conversation's session is still written to the registry from the id the runtime returns, when `Launch` returns.
- **Tests:** `TestFirstLaunchRejectsAnEarlyReportOfAnotherSession` and `TestFirstLaunchDeliversMatchingCallbacksInOrder` in `internal/service/start_test.go` go with the queue, and so does anything in `internal/harness/harnesstest` that only they use.

## what does not change

Everything else. That includes the check in `started` against the bound session, `sameSession`, the refusal in `send` of a runtime marked wrong or stopping, the latch the Claude adapter checks before it writes a prompt, and everything else piece 2 added. Other removals from the same review come as cards of their own.

## how it is run

- **It starts by checking the three observations under "why" against the code.** If one does not hold, the work stops and asks.
- **A test that fails afterwards** is one of the two named, or it is a change in behaviour. A change in behaviour stops and asks.
- **The gates are the epic's:** `make test`, all of `make checks`, `make e2e` with the harness versions and every skip named, and terminus on the working tree, then Michael.
- **Terminus will repeat three findings Michael has already disposed of,** listed in the journal for 2026-11-23. It may also raise the first-launch mismatch again. This card is the answer to that one: no harness can produce it.
- **A finding whose fix would add state, a hold or an ordering rule** is brought to Michael as a question of whether to fix it at all, before any fix is proposed.
- **Size.** `internal/service` was 6,638 non-test lines at `8503877`. The report gives the figure afterwards, and what was deleted outright.

## as built

Built on 2026-11-23. The queue is gone and `internal/service` went from 6,638 non-test lines to 6,547. Two things rode along by Michael's rulings:

- **The runtime's id is read before the service lock is taken again,** as at `8503877`. The removal had moved the read under the lock, which is the order the card `codex-request-raised-under-the-runtime-lock` described. With it restored, no call from the service into a runtime holds the service lock, and that card was removed.
- **An answer that arrives while a runtime is still starting is refused** with "the runtime is starting", and the request stays pending. The queue had been hiding that window on a pi first launch, where an extension can ask at session start. The inbox card for it, `an-answer-before-the-runtime-is-installed-is-aborted`, was fixed here and removed.

The last terminus review, `f0c417e1ff9b`, was `not_clean` on three findings in code this card did not touch: the race captured as `an-answer-racing-a-release-retires-a-live-pi-dialog`, the unread rebuild that piece 3 removes, and the shared turn record on the paused hardening card.
