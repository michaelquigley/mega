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
7. **`a-failed-read-after-a-reconnect-is-tried-again`.** A read that fails after a reconnect can no longer leave a stretch of a conversation missing. It replaces `one-route-for-recorded-history`, which was built and set aside on 2026-11-25.
8. **Taken out on 2026-11-26.** `the-page-has-one-value-for-where-it-stands` changed almost no behaviour. What the page knows of where it stands is drawn with the central application, and its notes are on `central-application`.
9. **Taken out on 2026-11-24.** `one-kit-under-the-adapters` changed no rule. It is the first step of `the-orchestration-framework-is-extracted` and was folded into that card.
10. **Removed on 2026-11-24.** `one-fold-for-what-happened`.
11. **Removed on 2026-11-24.** `the-registry-records-no-runtime-state`.

Then the full-tree review resumes on `harden-requests-stops-and-restarts`, and its count per pass is compared with five.

**The order was changed on 2026-11-24.** `a-failed-write-stops-mega` is built first of what remains, ahead of piece 3, and was built that day. It depends on nothing, it removes about thirty branches that handle a failed write, and every later piece is simpler to write and to review once that question has one answer. The numbers above are kept, since the journal uses them.

## the plan after the review of 2026-11-24

Built so far: pieces 1, 2 and 5, and the four removals listed below. What remains, in order:

1. Built on 2026-11-24: four small defects, as one batch: `the-dialogs-suite-crashes-on-a-first-full-run`, `a-copied-event-shares-its-turn-record`, `a-service-test-writes-the-registry-behind-the-service` and `a-model-asked-for-with-a-context-suffix-reads-as-a-different-model`.
2. Built on 2026-11-24: piece 3, `unread-is-output-beyond-where-you-have-read`.
3. Built on 2026-11-25, and kept after Michael's trial: piece 4, `a-turn-is-inferred-from-a-message`, in the simpler direction ruled on 2026-11-24: a turn is what the harness says it is. The card for a Codex interrupt lost while a turn is starting closed with it.
4. Built on 2026-11-25, and kept after Michael's trial: piece 6, `reading-the-workspace-starts-nothing`, in its larger size: no environment is resolved in the background at all.
5. Built on 2026-11-25: piece 7, now `a-failed-read-after-a-reconnect-is-tried-again`: the read a reconnect makes is retried when it fails.
6. Built on 2026-11-26: `adoption-reaches-any-session-in-the-checkout`. Codex and pi list a whole checkout, as Claude does. Then `an-older-log-is-read-as-todays-build-reads-it`, which waited on it, built the same day: the fill of a conversation's request, the replay of old import markers and the `mode_confirmed` flag are gone, and `internal/service` is 5,955 non-test lines. Both come before the first tagged release, since the rule that an older log is read as today's build reads it lapses there.
7. Built on 2026-11-27: `the-tested-ranges-reach-the-builds-in-use`. `make e2e` admits a build to its tested range, a map in `internal/e2e` says what the round trips cover and the ordinary gate keeps it, and the three ranges reach the builds in use: Claude Code 2.1.331, Codex 0.172.0 and pi 1.6.1. Until that day the card was `claude-code-21331-is-past-the-tested-range`.
8. The full-tree review resumes on `harden-requests-stops-and-restarts`. It is next: every card of the sequence is built, and `internal/service` is 5,955 non-test lines against 6,638 when the review of 2026-11-23 began. How the epic closes is decided after the review's first pass.

## removals from the review of 2026-11-23

Two pieces in, `internal/service` was the size it started at, and Michael asked for a review of the model, of what had landed, and of this plan. Its rulings are in `docs/future/simplify-the-core-inventory.md`. A removal it finds is a card of its own, built before the next numbered piece:

- **`a-first-launch-holds-nothing-back`.** A callback can no longer be held, reordered or dropped by a launch in progress. Built 2026-11-23.
- **`a-conversation-gets-its-session-once`.** A conversation's session can no longer be known in one place and missing from another. Built 2026-11-23.
- **`release-only-stops-a-runtime`.** A stop can no longer be recorded as two different things, and a session's own history has one way into mega. Built 2026-11-24.
- **`reads-after-the-service-closes`.** A read can no longer show what the record does not hold after the service has closed. Built 2026-11-24.

## how each piece is run

- **Spec first, where a promise changes.** The section "A simpler core" in `docs/future/mega.md` holds the target. A piece that changes what mega promises brings that section in step before it is built.
- **A short work order or a prompt of its own,** written from its card when its turn comes, and reviewed by mercurius where its size warrants. A small piece gives the review one surface, and converges.
- **Terminus to `clean` on its working tree, then Michael.** It leaves `make test`, all of `make checks` and `make e2e` passing, and its report names the harness versions its round trips ran on and every case that was skipped, because `make e2e` passes when a fixture or a model is missing.
- **The checks are what make this safe to attempt:** about 550 steps in nine suites, and the round trips. A piece that needs a check weakened has changed behaviour, and stops.
- **Two measures.** It names a failure that has become impossible and shows it. And it leaves `internal/service` no larger than it found it, reporting what was deleted outright, since moving code is not removing it.
- **What it does not say changes, does not.** The code and its tests, as they stand when a piece starts, are the specification of everything its card is silent on.

## where the work lands

Every piece lands on `main`, one at a time, like any batch. The branch `simpler-core` was for pieces 10 and 11 and went with them.

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

Three more were taken out by the review of the plan on 2026-11-24:

- **The fold and the registry change, pieces 10 and 11.** They were to end rules for a failure between two writes, and start-up working state out with code of its own. `a-failed-write-stops-mega` ended the first. Piece 3 removes the start-up code that actually failed, the unread rebuild. What was left for them was three small things a start does, and they were the two heaviest pieces: a branch, new facts recorded on events, a tool to compare old and new on a real workspace, a week's soak, and a registry migration on five machines. The registry and a log can now disagree only after a crash between two writes, and both orders err toward Unsupervised. Michael: "if they're not actually likely to be problems, we just remove them from the board entirely?"
- **A stop mega forced not clearing an error.** Decided on 2026-11-19 for the fold, and dropped with it. Any completed stop clears the mark. The one stop mega still forces with an error is on a runtime that reports the wrong session, which has never been seen, and its error row stays in the timeline.
- **The kit under the adapters, piece 9.** It changed no rule and removed no case, and it is the start of `the-orchestration-framework-is-extracted`, a later capability. It is folded into that card.
- **One route for recorded history, piece 7 as first carded.** The stream was to carry an event's number, and the page to read the event. It was built on 2026-11-25 and set aside before it was committed. It was to take forty to sixty lines off the page and added about fifty-five, since the loop that reads, waits and retries is longer than the path it replaced. It made every event two trips where one served, which costs nothing on one machine and would be felt on every event through a central interface. And it brought two edges when a read is slow: a tool row keeping its spinner, and the start of a streaming reply cleared. The work is on the branch `tried/one-route-for-recorded-history`. The fault it was for is fixed by a retry. How events reach a page on another machine is a question on `central-application`, and the answer there may be the opposite: the stream as the one route.
- **One value for where the page stands, piece 8.** It gathered into one function what the page already works out in several places, and changed almost no behaviour. Three page pieces in a row came out larger than estimated. And a page that reaches an environment through the central application has more to know than this value held, so it would have been built for one hop and rebuilt for two. Its notes, and the one finding matched to it, are on `central-application`. Taken out on 2026-11-26.

The lesson is the epic's own, applied to its plan. A finding count that does not fall is a sign the design, or its description, has too much in it, and the answer is to take something out.

## where the ideas came from

Pieces 3, 4, 5, 8, 9, 10 and 11 are the planning agent's reading of the hardening findings. Pieces 1, 2, 6 and 7, and the measure "name a failure that has become impossible", are from an independent review for simplification that Michael asked of an Astra 6.2 agent on 2026-11-19. It found what the first reading could not: mechanisms that exist because of what mega promises, not because of how the core is built.

## a sign to stop

If a piece finds it needs a new record, a new kind of event line, a second copy of a fact, a new state, or a rule for a failure between two writes, that is the signal to stop and ask whether the design is still the simple one.
