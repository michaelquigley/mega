# Simplify the core: an inventory of the model

A working document for the epic `simplify-the-core`, started 2026-11-23. It lists the parts of mega's model as built, says what each is for and whether it gets used, and records Michael's ruling on each: keep, keep but coarse, or drop. The review of what pieces 1 and 2 landed, and the review of the remaining plan, are both made against these rulings. When the epic and the spec have been revised from it, this document is removed.

The goal, in Michael's words on 2026-11-23: "simplify the model down to just the parts that matter (and that we plan to use), hopefully moving towards a core that needs less machinery to cover complicated corner cases."

## Why it was started

Two pieces in, the core is the same size, and piece 2 took seven mercurius rounds and eleven terminus reviews.

| Non-test lines | Before the epic (`83a1493`) | After piece 1 (`19e83c4`) | After piece 2 (`8503877`) |
| --- | --- | --- | --- |
| `internal/service` | 6,643 | 6,685 | 6,638 |
| all of `internal` | 19,378 | 19,511 | 19,267 |
| tests in `internal` | 18,621 | 19,365 | 19,673 |
| `ui/src` | 7,910 | 7,922 | 7,991 |

Piece 2's first build left the service at 6,532. The reviews that followed put 106 of the 153 removed lines back. Mercurius returned 27 findings and every one was answered with a fix. Five consecutive terminus reviews each found a hole in the fix before it, and left a queue in `internal/service/live.go` that holds every adapter callback during a first launch.

## What the counts are

Counts from Michael's own workspace on this machine, read without the service and without reading any message: 13 days with activity, 2026-11-10 to 2026-11-23, 16 projects, 27 conversations (14 Codex, 13 Claude, no pi), 52,999 events, 825 turns. The service's own log shows 24 starts under the daemon.

They are one machine of three, over two weeks in which the build changed daily. A zero means it did not happen here, not that it cannot.

| What | Count |
| --- | --- |
| Turns ended | 825: 821 success, 4 interrupted, 0 error |
| Requests raised | 428: 414 approvals, 14 questions |
| Requests ended | 428 answered; none aborted, lost, expired, unanswered or unconfirmed |
| Runtime starts and stops | 105 starting, 104 started, 102 stopping, 101 stopped |
| Unsupervised | 1 |
| Runtime error | 1, a Codex launch that failed with its session unknown |
| `not_started` | 0 |
| Output recorded after its turn | 0 |
| Permission mode changed while running, or requested with no runtime | 0 |
| Model changed while running | 3 |
| Adoptions | 5 |
| Catch-up imports on resume, removed by piece 2 | 11: 6 added nothing, 3 added one to three records of harness bookkeeping, 1 added a two-message exchange 57 seconds after a clean stop, 1 added 249 events on the first day, from work done outside mega |
| Sessions per conversation | 1 in every conversation that has one |
| A write the service could not confirm | 0 in the service's log |

## The parts

| # | Part | What it is for | Seen in use | Ruling |
| --- | --- | --- | --- | --- |
| 1 | Project, checkout, project environment | Where work happens and what it runs in | 16 projects, 10 with an approved activation | |
| 2 | Conversation bound to one native session | The durable identity | 27, one session each | |
| 3 | Launch, prompt, turn | The daily loop | 825 turns | |
| 4 | Requests: approvals and questions | What the agent waits on the operator for | 428, all answered | |
| 5 | Reading position, unread, needs you | Where attention is needed | Daily | |
| 6 | The shutdown gate and a clean restart | Updating mega | About two starts a day | |
| 7 | Model, effort and mode chosen per conversation | The operator's choices | Model changed 3 times; mode never | Keep, live changes included. Ruling 5. |
| 8 | Discovery and adoption, with a snapshot of history | Bringing an existing session in | 5, all in the first days | |
| 9 | Release for a terminal handoff, the gap mark, the native transcript view | Leaving mega and coming back | No handoffs, by Michael's account | Drop the handoff. Release stays as "stop this runtime". Ruling 1. |
| 10 | Unsupervised and Resume | A runtime mega lost sight of | 1 | Keep the mark. Nothing fetches or shows what mega missed. Ruling 3. |
| 11 | Launch failure, session unknown, `not_started` | A launch that did not come up | 1, and 0 | Drop the retry of a failed first launch. Ruling 4. |
| 12 | Request endings other than answered: aborted, lost, expired, unanswered, unconfirmed | Saying how an unanswered request ended | 0 of 428 | |
| 13 | Interrupt | Stopping a turn | 4 | |
| 14 | Output after its turn | A late item from a turn that has ended | 0 | |
| 15 | A turn opened by a message nobody prompted | A harness that starts work by itself | 68 of 834 turns, in 8 conversations | Keep. It is in daily use. |
| 16 | The two uncertain states after a failed write | Running on while unable to record | 0 | |
| 17 | pi, its dialogs, Dismiss, expired, waiting | The third harness | None on this machine | |
| 18 | Rules for logs written by older builds | Reading two weeks of earlier formats | Every start | Drop. An old log is read as today's build reads it. Ruling 2. |
| 19 | The one-time fill of a conversation's choices | Conversations created before choices were concrete | 6, all done | Drop, under ruling 2, once his other machines have run it. |
| 20 | The page: where it stands, and its two routes for history | Whether the page is current | Daily | |
| 21 | Observed and never recorded: background jobs, MCP servers | Activity beside the timeline | Daily | |
| 22 | The turn line: asked, ran, differs | What each turn ran with | Daily | |

## Rulings

### 1. Release is "stop this runtime" (2026-11-23)

Mega does not support taking a session to a terminal and bringing it back. Michael: "if it can simplify the implementation then there is no real reason to release a runtime so that it can work in the terminal, with the ability to resume it in mega." And: "i'm pretty sure i'd be fine with release just being 'stop this runtime'."

The ruling is conditional on it simplifying the implementation, which the review of the landed work has to show. What it puts in question, all from piece 2: the handoff flag on a stop, the gap flag on an event, the native transcript view with its route and its links from **More** and from gap rows. They are named in eleven non-test files, the conversation check suite and the three round-trip files. Nothing is removed until that review is ruled on.

Lost supervision is a separate part, 10, and is not settled by this.

The rule for the operator, in Michael's words the same day: "don't interact with the session outside of mega, if you want to keep the session intact inside mega." He wants starting and stopping a harness in mega to stay clean and to lose nothing. It does: with no work done outside, mega recorded everything as it watched, and a stop and a start have nothing to fetch. The counts agree. The only conversation a catch-up import ever brought in on this machine was the stretch of 249 events on the first day.

### 2. An old log is read as today's build reads it (2026-11-23)

Until the first tagged release, mega keeps no rule whose only subject is something an earlier build wrote. An old log opens and shows its history, and its lines are read with today's meaning. Michael: "yes, i think so."

- **What it costs:** a mark that is wrong once on a rare old conversation, such as an error mark from an import that failed under an older build.
- **Safety is not relaxed:** where an old log leaves it unclear whether a runtime is alive, the conversation is unsupervised.
- **A change that would break reading gets a conversion** at start-up, carried for a few builds and then deleted. After that, a workspace that skipped those builds is told to run the older build once first.
- **The fallback is adoption.** Michael: "my naive understanding is that we can always just do a clean full-import (adopt) of a session?" It is right. A conversation can be removed and its session adopted again, and the harness's own history comes back whole. What only mega recorded does not: start and stop rows, approval rows, what each turn ran with, the name and the label.
- **A limit found later the same day.** `Adopt` refuses a session discovery does not list, and Codex and pi list only the 50 newest in a checkout. "Always" was more than had been checked. The fallback holds for a session that is new or was active when mega went down, and can fail for one left alone for weeks in a busy checkout. No real checkout on this machine is past 50 today. The card is `adoption-reaches-any-session-in-the-checkout`, to be built before the rules for older logs are removed.

What it puts in question: the one-time fill in `internal/service/choice.go`, the replay of old import markers in `internal/service/live.go`, and the `mode_confirmed` rows on the page. Open: whether Michael's other two machines still hold conversations from before 2026-11-13, which the fill has not yet run on.

### 3. Work mega missed while it was down stays missed (2026-11-23)

If mega goes down while a turn is running, the harness may finish the turn, and that work is in the harness's session and not in mega's log. Mega does not fetch it and does not show it. The Unsupervised row marks where mega lost sight, the agent still holds the work in its own context, and an operator who wants it in mega removes the conversation and adopts the session again. Michael: "A sounds like the simplest option?"

Declined: keeping the native transcript view for this case, and a catch-up read after lost supervision. With rulings 1 and 3 the native transcript view has no case left to serve.

### 4. A new conversation whose first launch fails is removed, not retried (2026-11-23)

A first launch that fails, for any reason, leaves a conversation that shows the error and can only be removed. The operator corrects the problem and starts a new conversation, or adopts the session if the harness made one. Michael: "if it's a new conversation... there's no loss in not having a retry. so jettisoning the failed conversation, correcting the issue, and re-launching seems like the simplest path?"

What it puts in question: the `not_started` status, the proof each adapter gives that a launch failed before any session could exist (`harness.ErrNotStarted`), and the service's re-read of the log before it allows a fresh session (`freshAllowedLocked`). They are named in ten non-test files. The rule that replaces them: a conversation with no session launches once.

### 5. A change of model, effort or mode stays live (2026-11-23)

Michael, on whether a change should apply only at the next start: "C is the behavior i think most operators expect. would switching to A create significant simplification opportunities?" It would not. Measured, the live switch is about 200 non-test lines of 19,000: some 75 in the service, some 90 across the three adapters, the rest in the shim and the interface. It is self-contained. It does not touch stops, restarts, requests or recording, and the fault it is remembered for, the idle model switch, was one case of a message outside a turn, which piece 4 answers for every such message. The design agent had recommended the change before measuring it and withdrew the recommendation.

### 6. The rest is kept (2026-11-23)

Michael confirmed the remainder in one answer: "that all sounds correct."

- **Kept, in daily use:** parts 1 to 6, 8, 13, 21 and 22.
- **Kept, not used on this machine:** part 17, pi and its dialogs.
- **Kept as they are:** part 12, the five request endings besides answered. Each names a path that exists anyway, so merging the names removes nothing.
- **Already going by the existing plan:** part 16 by piece 5, and part 14 by pieces 3 and 4.
- **Left for the review of the plan:** part 20, with pieces 7 and 8.

The inventory is complete. The review of what pieces 1 and 2 landed is next, against rulings 1 to 4.

### 7. Only the registry binds a conversation to its session (2026-11-23)

Today the service also holds a session id in memory, seeded from the log's last `started` line at start-up, for a session the registry does not hold yet: the write that saves it failed, or mega went down between the launch returning and that write. The next launch then saves it. Michael, given the scenario of a power loss in that instant: "A sounds simpler."

A conversation whose session never reached the registry has no session, and is treated as any failed first launch under ruling 4: it is removed, and its session is adopted, since no conversation holds it. What is lost is the name the conversation was given and mega's own rows for its first seconds.

## The landed work, reviewed against the rulings

Each item says what was found and waits for Michael's ruling. Nothing is removed by this document. Removals he accepts become a piece of their own.

### A. The queue that holds callbacks during a first launch

Added by piece 2's terminus reviews, in five consecutive rounds: `early`, `earlyExit`, `launchCallback`, `report` and `confirmFirstStart` in `internal/service/live.go`, and their handling in `startRuntime` in `internal/service/conversation.go`. While a conversation with no session launches, every callback from the adapter is held. When `Launch` returns, each held start report is compared with the id the runtime gives, and the callbacks are delivered or discarded.

The case it guards, a start report naming another session than the one `Launch` returns, cannot arise with any of the three harnesses:

- **Codex.** `internal/harness/codex/codex.go` sets the runtime's thread id and the start report's session from the same value, `started.Thread.ID`, a few lines apart.
- **pi.** `internal/harness/pi/pi.go` does the same with its session id.
- **Claude.** The adapter chooses the id of a new session and passes it with `--session-id`. Claude sends no `system/init`, and so names no session, until a turn starts. The opening prompt is sent by `StartConversation` after `Launch` has returned and the id is bound. From there the ordinary check in `started`, the one a resume relies on, compares every report with the bound session, and the adapter's own `sameSession` stops relaying on the first frame that differs.

The harness has also been given nothing to answer while the queue holds: the prompt follows the launch. The tests that exercise the queue use a fake that reports one session and returns another.

**Ruled 2026-11-23: take it out.** Michael: "yes. let's take it out. let's use a fresh implementer to do it." The card is `docs/future/roadmap/a-first-launch-holds-nothing-back.md`, in researching, and the epic lists it under its removals.

**Built 2026-11-23.** `internal/service` went from 6,638 non-test lines to 6,547. Two things rode along by Michael's rulings: the read of the runtime's id before the service lock, which the removal had undone, and a refusal of an answer that arrives while a runtime is still starting, which the queue had been hiding on a pi first launch.

### B. The retry of a failed first launch, and the session held outside the registry

Rulings 4 and 7. One rule replaces the machinery: only `StartConversation` launches a conversation with no session, when it creates it, and `Prompt` and `Start` refuse one afterwards. That removes `freshAllowedLocked`, the `not_started` status, `harness.ErrNotStarted` in three adapters, and the `session` and `sessionRecorded` fields with their seeding from the log.

The canon needs no change for it: `exact-id-resume` already flags a new session for an existing conversation. No check suite asserts the retry. The card is `docs/future/roadmap/a-conversation-gets-its-session-once.md`, and the spec's "A simpler core" carries the rule.

### C. The handoff, the gap flag and the native transcript view

Rulings 1 and 3. The card is `docs/future/roadmap/release-only-stops-a-runtime.md`, written 2026-11-24. Two things were settled in writing it:

- **The release dialog stays as a confirmation,** without the terminal command. Stopping a runtime still ends background jobs and a pending pi dialog, and the dialog is where that is said.
- **The transcript read was also how the round trips and the version probe checked native speech.** They keep those checks and read through adoption's snapshot.

The canon was brought in step first: the transcript sentence left `exact-id-resume`, `honest-runtime-state` and `bookkeeping-never-creates-attention`.

**Built 2026-11-24.** `internal/service` is at 6,419 non-test lines, against 6,638 when the review began. One finding was this card's: a release in the middle of a turn no longer has the gap flag to separate that turn from the next. Michael ruled to leave it and add nothing. It is noted on `a-turn-is-inferred-from-a-message`, to be decided with the turn model.

Still to review from the landed work: the rules for older logs under ruling 2, which wait on `adoption-reaches-any-session-in-the-checkout`.

## The plan, reviewed

Begun 2026-11-24, with the three removals built and `internal/service` at 6,419 non-test lines. Five questions, one at a time: the order, pieces 10 and 11, piece 9, piece 4, and where the small parked cards go.

### P1. Piece 5 goes first (ruled 2026-11-24)

Michael: "yes... let's go ahead and fix that next." `a-failed-write-stops-mega` is built ahead of piece 3. The service held about thirty branches that handle a failed write, two uncertain states, the mark for a settlement that could not be recorded, and fifteen tests named for one. The card settles one thing its first form left open: a failed write is met in one place, the service's two write wrappers, and the process is ended by a function `cmd/mega` supplies, so the stores still only return an error.

**Built 2026-11-24.** `internal/service` is at 6,222 non-test lines, against 6,638 when the review began. Terminus raised one thing three times that this piece made visible: a command that finishes after `Close` changes memory and writes nothing. Michael left it for `reads-after-the-service-closes`, which is to be built next.

### P2. The board, put in order (2026-11-24)

Michael: "give the entire roadmap a pass... order anything that is unordered. feel free to propose the next item to build." Every card in a ranked lane now has a position, and the inbox is empty.

- **Proposed next:** `reads-after-the-service-closes`, as "refuse the read". It removes the copy `Close` sets aside for late reads, and makes the finding terminus raised three times on piece 5 unreachable.
- **Then three small defects,** which could go as one batch: the flaky `dialogs` suite, the shared turn record, and the test that writes the registry behind the service.
- **Parked findings now have cards:** `a-copied-event-shares-its-turn-record` for the turn record, and `codex-interrupt-while-a-turn-is-starting` placed after piece 4, where its window lives. `an-answer-racing-a-release-retires-a-live-pi-dialog` sits after piece 9.
- **Ruling 2 has its card,** `an-older-log-is-read-as-todays-build-reads-it`, placed after `adoption-reaches-any-session-in-the-checkout`, which it waits on.
- **To horizon:** `a-directory-mega-creates-may-not-be-synced`, and three of Michael's own captures, tagged: `dfw-window-mode`, `harness-session-names` and `retired-conversations`.
- **Pieces 9, 10 and 11 keep their old places** until the plan questions about them are answered.

### P3. Pieces 10 and 11 are removed (ruled 2026-11-24)

`one-fold-for-what-happened` and `the-registry-records-no-runtime-state` are gone from the board, not parked. Michael: "i'd probably lean towards A, with the modification that if they're not actually likely to be problems, we just remove them from the board entirely?"

They are not likely to be. A failed write now ends mega, so the registry and a log can disagree only after a crash between two writes. At a launch the log is written first and the harness is not started until after the registry write. At an exit the log is written first, and a crash there leaves the registry saying running, which a start turns into Unsupervised. Once piece 3 lands, a start marks pending requests lost, marks a running conversation Unsupervised and finishes an interrupted mode request, and nothing else. The epic's list of what was taken out carries the reasons.

**The decision that rode on piece 10 is dropped.** "A stop mega forced does not clear an error" was decided on 2026-11-19 for the fold. Any completed stop clears the mark. The one stop mega still forces with an error is on a runtime that reports the wrong session, which has never been seen. Michael: "i think A sounds reasonable."

### P4. Piece 9 is folded into the horizon card it began (ruled 2026-11-24)

Michael: "let's do A." `one-kit-under-the-adapters` changed no rule and removed no case, and its own card called it the first step of `the-orchestration-framework-is-extracted`. Its content is on that card now, and `an-answer-racing-a-release-retires-a-live-pi-dialog` sits beside it on horizon.

### P5. Piece 4: a turn is what the harness says it is (ruled 2026-11-24, to be tried)

The card kept two ways for a turn to open, mega's prompt and a start the adapter declares, with rules for when they overlap. The direction ruled is simpler: mega opens no turn, and a turn opens and closes on the harness's own markers. Michael: "i think A sounds like a real simplification, which is what we're trying to accomplish. i'd like to give it a try before fully committing to it... maybe when it lands, before committing, i'll build it and switch to it. if it still feels reasonable in use, we'll commit and continue?"

The trial is in the working tree, with no branch: "there are a number of other projects working in mega that i can test with while we pause mega work during evaluation for 20 minutes." Nothing new is written by the piece, so it can be backed out. The card carries the direction and what to watch for, and is rewritten as a plan when its turn comes.

### P6. `reads-after-the-service-closes` is built (2026-11-24)

Michael launched it as "refuse the read". Every read is refused once the service has closed, and the copy `Close` set aside is gone. `internal/service` grew by 41 lines, to 6,263, because the card said every read before the reads had been counted and only two of thirteen used the copy. Michael accepted the lines: one rule with no exceptions.

### P7. The batch of four small defects is built (2026-11-24)

The flaky `dialogs` suite turned out to be a race in the check itself, against the register dialog's own prefill. The copied event, the registry test and the context suffix are fixed as carded. `internal/service` is at 6,281 non-test lines; the batch was fixes and not removals. Four findings outside it were captured: three in mega, and the logger race in df's own roadmap.

### The plan as it stands

The epic's section "the plan after the review of 2026-11-24" has the order: four small defects as one batch, piece 3, piece 4 with its trial, piece 6, pieces 7 and 8, the adoption limit and then the older-log rules, and then the full-tree review.

## The gate

Michael, 2026-11-23: "we also need to make sure the terminus canon is appropriately aligned, as we're getting pretty continuous `not_clean` results."

Three causes were found:

- **Terminus keeps no dispositions, and its prompt calls the changed files "where to begin, not a fence".** Each piece's review re-reads the surrounding core and raises every known finding again. The paused hardening review runs inside every piece.
- **None of mega's seven blocking qualities stated a limit.** A rubric's `characteristics` do not reach the reviewer, so a limit can only be written into quality bodies.
- **The qualities are behind the decisions:** native transcript language in three, the degraded mode in `single-authority`, a first-slice note in `honest-runtime-state`.

### G1. A stated limit in five qualities (ruled 2026-11-23)

Michael, on the draft: "that sounds reasonable." One paragraph, the same words, ends the `boundary` section of `exact-id-resume`, `honest-runtime-state`, `pending-requests-distinct-from-history`, `bookkeeping-never-creates-attention` and `single-authority`. It is uncommitted in the terminus-canon working tree, with a journal entry there, for Michael to commit. `permissions-never-weakened` and `protocol-confined-to-adapters` were left alone on purpose.

### G2. No dated exemptions, and terminus is left alone (ruled 2026-11-23)

A "do not flag until piece 3 lands" line was proposed for the unread rebuild and withdrawn. Michael: "this is essentially telling terminus about planned work, yes?" And: "if the remainder of the recurring terminus findings are already planned to be addressed, we can leave terminus alone for now." The canon changes when a rule changes.

### G3. Older logs, stated in two qualities (ruled 2026-11-23)

Ruling 2 is a rule, so it went into the canon when its first application drew a blocking finding: a paragraph at the end of the `boundary` section of `honest-runtime-state` and `bookkeeping-never-creates-attention`. Michael: "sure, that's fine. we'll clean it up once we're past that part of the journey." It lapses at the first tagged release, and `first-tagged-release` carries the reminder.

### What worked on the removal cards

- **A finding in code the card did not touch and did not make newly reachable** is matched to an existing card or captured, and listed at handoff.
- **A finding that is only wording** in a doc or a test of the card's own is fixed and reported at handoff.
- **A finding whose fix would add state, a hold or an ordering rule** is a question of whether to fix it at all.

## Findings that needed no ruling

- **Part 15, a turn nobody prompted, is common.** 68 of 834 turns on this machine ended without a prompt from mega, in 8 conversations, one of which had 31. Most began one to ten minutes after the turn before ended, and began with a shell command or with `collect_review` on terminus or `collect_round` on mercurius. It is an agent waking when its background work finishes, which is how the review brokers are driven. A turn the harness starts is a real turn, and piece 4 has to treat it as one.
- **A permission mode request is two writes, a model request is one.** A mode request is recorded as an event and then written to the registry, with a rule at start-up to finish it if a crash came between (`settleModeRequestLocked`), and a second event to put it back if the registry write fails. A model request is one registry write. This is the epic's "rule for a failure between two writes". It is for the review of the plan, and it bears on `permissions-never-weakened`, which may be why the event was wanted.
