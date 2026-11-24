---
title: a failed write stops mega
state: evaluating
created: 2026-11-19
tags: [story]
---

On the first failed write to the record, mega logs why and exits at once, the way a crash would leave things. The record is the registry and the event logs.

Fifth in the sequence of `simplify-the-core`, and built first of what remains: Michael moved it to the front on 2026-11-24.

## why

Today a failed write leaves the service running on its last confirmed view. The registry and each event log have an uncertain state of their own that refuses later changes (`registry.ErrUncertain`, `events.ErrUncertain`), and each operation has its own rule for what the page may be told when its record fails. About eleven of the hardening findings were a write that failed partway: an answer hiding a request before it was recorded, a settlement that could not be recorded, a mark for a request that "takes no further answer".

An unclean restart is a path that has to exist and work anyway. This makes it the only one.

Decided by Michael on 2026-11-19. Two things were in earlier drafts and were taken out, and should stay out: ending the runtimes on the way out, and telling the page why. Each needed rules of its own in every review round it survived.

On 2026-11-24 the service held about thirty branches that handle a failed write to the registry or a log, and fifteen tests named for one. The blocking finding terminus raised in six reviews of `a-conversation-gets-its-session-once` was a write failing after another write. Michael's own workspace recorded no failed write in two weeks.

## the failure that becomes impossible

Mega can no longer run on while unable to record.

## what changes

- **A failed write never returns to its caller.** The service writes the record through two wrappers, `update` for the registry and `appendEvent` for a log. They are the one place a failed write is met, and they end the process through a function `cmd/mega` supplies. That function logs the reason and exits, without the orderly stop that `mega serve` and `mega daemon` take today, which releases every runtime. The stores, `internal/registry` and `internal/events`, still return the error and decide nothing.
- **Creating a log is a write to the record,** as an append is.
- **Removed:**
  - both uncertain states, and the rollback of a failed append;
  - every branch in the service that handles a failed write, and the `error` result of whatever is left with nothing else to return;
  - the mark for a settlement that could not be recorded, `unrecorded`, and the refusals that read it;
  - the event that puts a mode request back when its registry write fails.
- **What a change itself refuses is not a failed write.** `update` still returns the error of the function it is given, such as a conversation that does not exist. The refusal of a write after the service has closed stays.
- **In tests,** the two hooks that inject a failure, `failUpdate` and `failRecord`, stay as the way to cause one, and the function that ends the process is replaced by one a test can observe. How a test stops the caller there is the implementer's choice. In production code no caller goes on past a failed write.

## what it costs

Whatever a harness was doing goes on unwatched until its own pipes close or its turn ends, exactly as after a crash. A command already under way may finish what it was sending. The page shows the service as unreachable, and the reason is in mega's log.

It is the right trade for one operator on his own machine, where a full disk is rare and a restart is cheap.

## what does not change

A failure to write anything else under the data directory: a snapshot is best effort, and mega's own log file and the tray's runtime file are diagnostics. A log that could not be deleted is left, as now.

What the next start does: it is the start after a crash. A crash between two writes is still possible, so what a start does about one stays: the unsupervised line, the lost requests, and the finishing of a mode request.

## how it is shown

- an injected write failure, and separately an injected sync failure, in the registry and in a log, each end the process without the orderly stop: a conversation at rest with a runtime is sent nothing;
- the next start shows a conversation that was running as unsupervised and a pending request as lost;
- a failed snapshot stops nothing;
- nothing in the tree names an uncertain state or an unrecorded settlement.

## how it is run

- **It starts by listing every place the record is written and every branch that handles a failed write.** A branch that does something the next start would not put right by itself is brought to Michael before it is removed.
- **The canon is brought in step before the build,** by the planning agent, uncommitted in terminus-canon: `single-authority` loses its degraded mode and flags a service that goes on after a failed write.
- **Documents:** the "Data" section of `docs/current/mega.md` and its HTTP reference, and the changelog. On the paused hardening card, `answer-clears-concurrent-settlement-hold` goes with this piece; say so there. The spec's paragraph "One failure" is the planning agent's to retire.
- **Tests of the removed behaviour go, or are rewritten to the rule.** The report names each. A test or a check that fails for any other reason is a change in behaviour, and stops and asks.
- **The gates are the epic's:** `make test`, all of `make checks`, `make e2e` with the harness versions and every skip named, and terminus on the working tree, then Michael.
- **A terminus finding in code this card did not touch and did not make newly reachable** is matched to an existing card or captured in inbox, and listed at handoff.
- **A finding that is only wording** in a doc or a test of this card's own is fixed and reported at handoff.
- **A finding whose fix would add state, a hold or an ordering rule** is brought to Michael as a question of whether to fix it at all. Anything else this card caused comes to him one at a time.
- **Size.** `internal/service` was 6,419 non-test lines at `4a962cd`. The report gives the figure afterwards, and what was deleted outright.

## as built

Built on 2026-11-24. `internal/service` went from 6,419 non-test lines to 6,222, and the Go diff is 696 lines added and 1,101 removed. The function that ends the process is `exitAtFailedWrite` in `cmd/mega`. About ten functions lost an `error` result that had nothing left to carry.

- **A log is created by its first append,** so creating one goes through the same wrapper and there was no third place to handle. That append syncs the log's directory before it writes.
- **A command that finishes after `Close` changes memory and writes nothing.** After `Close` an event is dropped, since the logs are closed and a write must not end the process then. A read in the last moments before the HTTP server stops can show it, and the next start is correct. Michael left it: the window belongs to `reads-after-the-service-closes`.
- **A directory mega creates may be left unsynced** by a failed sync or a crash at the instant of its creation, since `durable.MkdirAll` syncs only what it creates. It is older than this card and is `a-directory-mega-creates-may-not-be-synced`.
- **Sync failures are injected in the stores' own tests, and write failures in the service's,** since the service has one path for every store error.

The last terminus review, `6f6059a940a4`, was `not_clean` on those two findings, both ruled on by Michael.

## order

Depends on nothing. Lands on `main`.
