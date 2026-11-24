---
title: simplify the core
state: researching
created: 2026-11-19
tags: [epic]
---

Make the core of mega smaller where it can be, so that whole kinds of fault have nowhere to live, instead of finding and patching each instance. It is done as a sequence of small pieces, each specified, reviewed and built on its own.

## why

Michael, 2026-11-19, five passes into `harden-requests-stops-and-restarts`: "part of this work needs to be an effort to simplify the core architecture. rather than patching and covering every possible corner case, we might consider simplifying the code or the model where we can." And later the same day: "the goal here is to figure out how to drive complexity out of mega and simplicity into the design. we need to keep pursuing ways to make the design (and the code) simpler."

The hardening work fixed about forty-five findings in three days, every one real, and each pass still returned five. A third of each pass was a hole in an earlier pass's fix. Sorted by what made each possible:

| Cause | About |
| --- | --- |
| A write that fails partway through | 11 |
| The page's sense of whether it is current | 8 |
| Work after a stop, or in the gap before a start | 7 |
| Whose session a stream or a file belongs to | 7 |
| What counts as unread after a lost runtime or an import | 7 |
| The rest | 5 |

Most are not about a feature. They are about how the core is put together, and about what mega promises.

## the sequence

One card each, in this order. Each names the failure it makes impossible.

1. **`a-codex-question-is-a-message`.** An answer to a Codex question can no longer start a turn after a stop.
2. **`megas-history-may-have-a-gap`.** A handoff leaves a gap in mega's timeline. A later import can no longer duplicate a message.
3. **`unread-is-output-beyond-where-you-have-read`.** An unread mark can no longer be lost to a failed write or miscounted by a rebuild.
4. **`a-turn-is-inferred-from-a-message`.** A stray message can no longer open a turn that nothing closes.
5. **`a-failed-write-stops-mega`.** Mega can no longer run on while unable to record.
6. **`reading-the-workspace-starts-nothing`.** A read of the workspace can no longer run anything.
7. **`one-route-for-recorded-history`.** A failed read of history can no longer leave a gap nothing fills.
8. **`the-page-has-one-value-for-where-it-stands`.** Two parts of the page can no longer disagree about whether it is current.
9. **`one-kit-under-the-adapters`.** A fix to one adapter's stop rule can no longer be missing from the other two.
10. **`one-fold-for-what-happened`.** A restart can no longer recover something different from what the service had decided. On the branch.
11. **`the-registry-records-no-runtime-state`.** The registry and a log can no longer disagree about whether a runtime is running. On the branch, with the cutover.

Then the full-tree review resumes on `harden-requests-stops-and-restarts`, and its count per pass is compared with five.

## removals from the review of 2026-11-23

Two pieces in, `internal/service` was the size it started at, and Michael asked for a review of the model, of what had landed, and of this plan. Its rulings are in `docs/future/simplify-the-core-inventory.md`. A removal it finds is a card of its own, built before the next numbered piece:

- **`a-first-launch-holds-nothing-back`.** A callback can no longer be held, reordered or dropped by a launch in progress. Built 2026-11-23.
- **`a-conversation-gets-its-session-once`.** A conversation's session can no longer be known in one place and missing from another. Built 2026-11-23.
- **`release-only-stops-a-runtime`.** A stop can no longer be recorded as two different things, and a session's own history has one way into mega. Built 2026-11-24.

## how each piece is run

- **Spec first, where a promise changes.** The section "A simpler core" in `docs/future/mega.md` holds the target. A piece that changes what mega promises brings that section in step before it is built.
- **A short work order or a prompt of its own,** written from its card when its turn comes, and reviewed by mercurius where its size warrants. A small piece gives the review one surface, and converges.
- **Terminus to `clean` on its working tree, then Michael.** It leaves `make test`, all of `make checks` and `make e2e` passing, and its report names the harness versions its round trips ran on and every case that was skipped, because `make e2e` passes when a fixture or a model is missing.
- **The checks are what make this safe to attempt:** about 550 steps in nine suites, and the round trips. A piece that needs a check weakened has changed behaviour, and stops.
- **Two measures.** It names a failure that has become impossible and shows it. And it leaves `internal/service` no larger than it found it, reporting what was deleted outright, since moving code is not removing it.
- **What it does not say changes, does not.** The code and its tests, as they stand when a piece starts, are the specification of everything its card is silent on.

## where the work lands

Pieces 1 to 9 land on `main`, one at a time, like any batch. Pieces 10 and 11 are built on a branch, `simpler-core`, which Michael creates. His words: "we do this work on a branch so that we can test it locally and ensure it's solid before merging it to `main` and to the two new downstream users." The merge is a gate of its own and his, not before the cutover has run on each of his three machines.

## what must not change

- A session is resumed by its exact id, and a new one is started only for a new conversation, once, when it is created.
- Permissions are never weakened, no trust is set for the operator, and nothing is answered for him.
- A state mega cannot vouch for is never shown as certain. A simpler mega may say "look at this" more often. It may not say something untrue.
- A pending request is answered once, by the operator, and never after it stopped being pending.

## what was taken out, so it is not added back

This began as one spec section and one work order of seven stages. The work order was removed on 2026-11-19 when its content had moved into these cards; an early form of it is in git history as `docs/future/mega-simpler-core-work-order.md`. It went through eight mercurius rounds in one day: six findings in each of the first six, then four, then three. Michael, at round 3: "we need to be careful we're not back to patching corner cases when a simpler design would eliminate them." Most findings were about things the drafts had added. Each of these was in a draft and was removed:

- **Moving the operator's facts into the log.** The session id and the requested mode, model and effort were to leave the registry. Old logs never recorded them, so that needed a seed, and the seed needed rules of its own.
- **Ending the runtimes when a write fails.** That needed an order between the stop and every write to a harness, in three adapters.
- **Telling the page why mega stopped.** A last frame before the exit needed a rule in each of three rounds.
- **Reading only the end of each log at start-up.** Checkpoints are a speed feature, and brought a new kind of line in the log. It went back to `start-up-reads-every-log`.
- **Changing the order in which a choice and its history are written.** Tidy, and not needed.
- **Describing what does not change.** Every paraphrase of the code was slightly wrong, and each became a finding.
- **One document for all of it.** Six stages gave a review six surfaces to find gaps between. Michael: "we might want to break some of these out into smaller chunks anyway, rather than trying to ram a big simplification spec and work order through."

The lesson is the epic's own, applied to its plan. A finding count that does not fall is a sign the design, or its description, has too much in it, and the answer is to take something out.

## where the ideas came from

Pieces 3, 4, 5, 8, 9, 10 and 11 are the planning agent's reading of the hardening findings. Pieces 1, 2, 6 and 7, and the measure "name a failure that has become impossible", are from an independent review for simplification that Michael asked of an Astra 6.2 agent on 2026-11-19. It found what the first reading could not: mechanisms that exist because of what mega promises, not because of how the core is built.

## a sign to stop

If a piece finds it needs a new record, a new kind of event line, a second copy of a fact, a new state, or a rule for a failure between two writes, that is the signal to stop and ask whether the design is still the simple one.
