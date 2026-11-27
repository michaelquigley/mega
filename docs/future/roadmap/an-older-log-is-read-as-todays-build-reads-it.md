---
title: an older log is read as today's build reads it
state: evaluating
created: 2026-11-24
tags: [story]
---

Remove the rules whose only subject is something an earlier build wrote. An older log opens and shows its history, and its lines are read with today's meaning.

A removal from the review of `simplify-the-core`, ruling 2 in `docs/future/simplify-the-core-inventory.md`. Michael, 2026-11-23: "yes, i think so." The card is the whole plan; there is no work order.

## why

Three rules in the core read what only an earlier build wrote:

- **The fill of a conversation's request.** Conversations created before 2026-11-13 could lack a model, an effort or a mode. At start-up each takes what its log last reported, and at its next launch the rest from the harness's settings. Each fill is recorded as a `choice_filled` line before the registry is written, and a fill interrupted between the two is found in the log and applied again.
- **The replay of old import markers.** An earlier build wrote `resuming` before an import's result. Opening a log tracks that pair, so that a failed import under that build does not show as a current error.
- **The `mode_confirmed` flag.** Nothing records it any more. The page still hides a mode report that carries it.

Counted in Michael's workspace on `fortyfive` on 2026-11-26, as kinds of line only: 30 logs, six `choice_filled` lines in six logs, no `mode_confirmed` flag, and no failed import of the old form. All 30 conversations hold a model. Michael, the same day, on his other two machines: "the other two machines have been taking new builds regularly." So the start-up fill has run everywhere it had work. The outside operators installed after 2026-11-13, when every new conversation already held its whole request.

## the failure that becomes impossible

A conversation's request can no longer be changed by anything but the operator after it is created, and what a log line means no longer depends on which build wrote it.

## what changes

- **The start-up fill goes:** `fillChoicesLocked` and `lastReported` in `internal/service/choice.go`, and the call in `internal/service/service.go`.
- **The fill at launch goes:** `fillAtLaunch` and `applyFillLocked`, the two fill sources, and `StatusChoiceFilled` in `internal/model/event.go`.
- **A launch refuses a conversation whose request lacks something,** and says to choose it on the control bar. For a Claude conversation with no effort that is the refusal it gets today, `errChooseEffort`. A request that holds no effort is judged against the model list, which the launch reads first if none is held, since a model that takes no effort holds none. Corrected on 2026-11-26: as first written this was a bare check, which would have refused every such model at its first launch after a restart.
- **The import replay goes:** `legacyImport` in `liveLocked`, `internal/service/live.go`.
- **`ModeConfirmed` goes** from `model.RuntimeFacts`, with `mode_confirmed` in `ui/src/types.ts` and the page's case for it.
- **The page's row for `choice_filled` goes.**

A line of one of these kinds in an older log still shows, as any line of its kind does, with its text.

## the one case that is not an older log

Adoption copies a request from the harness's settings and does not refuse when the settings name no effort (`Adopt`, `internal/service/adopt.go`). So a Claude conversation can be adopted today with no effort. Its first launch is refused until the operator chooses one, and that stays.

What changes for it is small. Today a launch tries the settings once more before it refuses. After this card it does not: a request is copied once, when the conversation is created or adopted, and after that only the operator changes it. `choice.go` already states that rule at its head.

## what does not change

`copyFor`, and what a new or an adopted conversation copies. `errChooseEffort` and its sentence. Choosing a model, an effort or a mode on the control bar, with a runtime or without one. `missingLocked`. Everything else.

## what it costs

A mark that is wrong once on a rare old conversation: a failed import of the old form, as the last runtime line of a log, shows as an error until that conversation next starts. Safety is not relaxed. Where an older log leaves it unclear whether a runtime is alive, the conversation is Unsupervised.

## how it is shown

- a conversation whose registry entry lacks a model is not filled at start-up: nothing is written to its log or to the registry, a launch is refused with what to choose, and it launches once that is chosen;
- a Claude conversation adopted with no effort is refused as it is today, and no `choice_filled` line is written;
- a log holding a `choice_filled` line, an old failed import and a `mode_confirmed` report opens, pages, and shows a row for each;
- nothing in the tree reads `choice_filled` or `mode_confirmed`.

## how it is run

- **It starts by reading every path that can leave a request incomplete today.** Adoption is the one this card knows of. If there is another, or if the control bar cannot choose a model for an idle conversation that has none, the work stops and asks.
- **Tests of the removed behaviour go, or are rewritten to the rule.** In `internal/service/choice_test.go`: `TestStartUpFillTakesWhatTheLogReportedOnce`, `TestInterruptedFillIsAppliedOnceFromItsRecord`, `TestConversationWithNoReportIsFilledAtLaunch` and `TestAFilledFullIdTakesItsAliasEfforts`. In `attention_test.go`: `TestLegacyImportFailureDoesNotChangeRuntimeAttention`. The report names each and says which. A test or a check that fails for any other reason is a change in behaviour, and stops and asks.
- **Documents:** "The one-time fill" and the list of runtime statuses in `docs/current/mega.md`, the glossary if it names the fill, and the changelog.
- **The canon needs no change.** `honest-runtime-state` and `bookkeeping-never-creates-attention` already carry the paragraph on older logs.
- **The gates are the epic's:** `make test`, all of `make checks`, `make e2e` with the harness versions and every skip named, and terminus on the working tree, then Michael. Run them from a disposable copy of the tree, so the Makefile's reinstall of the shim leaves Michael's running mega alone. Run all of `make checks` once more on the final code.
- **A terminus finding in code this card did not touch and did not make newly reachable** is matched to an existing card or captured in inbox, and listed at handoff.
- **A finding that is only wording,** or a fix that only changes this card's own check or test, is made and reported at handoff.
- **A finding whose fix would add state, a hold or an ordering rule** is brought to Michael as a question of whether to fix it at all. Anything else this card caused comes to him one at a time.
- **Size.** `internal/service` was 6,096 non-test lines when `adoption-reaches-any-session-in-the-checkout` was built. Counted by reading: the four fill functions are about 160 lines of `choice.go`, and the import replay about 15 of `live.go`. What is written is the refusal at launch, a few lines. The report gives the figures afterwards. "A few lines" was wrong: reading the list first and a list that cannot be read made it 50.

## when it lapses

Ruling 2 holds until the first tagged release. `first-tagged-release` carries the reminder.

## as built

Built on 2026-11-26. `internal/service` went from 6,096 non-test lines to 5,955, all of `internal` from 18,756 to 18,621, and `ui/src` from 7,964 to 7,945. The refusal that was written, `refuseIncomplete` with its four sentences, is 50 lines.

- **The launch reads the model list before it judges a request with no effort.** `offersEffort` says a model takes an effort whenever no list is held, and nothing reads the list at start-up. A request that holds an effort launches without the list, as before.
- **A list that cannot be read refuses the launch,** for a request with no effort: "the model list could not be read, so mega cannot tell whether this model takes an effort level; launching again reads it again". Michael ruled for it on 2026-11-26, over launching with no effort. Nothing copies the settings' effort any more.
- **A list that has since given a model effort levels refuses the launch** until one is chosen on the control bar.
- **Adoption without an effort reaches Codex and pi too,** when the list fails during adoption and the settings name none. The same refusal covers it, with a sentence that does not name Claude.
- **`StatusResuming` and `StatusResumed` went too,** on 2026-11-27. Only the removed replay read them, and nothing wrote them. A `resuming` or `resumed` line in an older log shows as a runtime row with its text.

The last terminus review, `cffc32eba7ca`, was `clean`. An earlier one raised `answer-remains-pending-during-delivery` in `Service.Answer`, which this card did not touch and `harden-requests-stops-and-restarts` already records.

`make test` and all of `make checks` (612 steps) passed on the final code. `make e2e` passed 18 of 20 with no skips. `TestPiRoundTrip` and `TestPiReleaseWithADialogPending` failed on what the model replied, the same way on `HEAD`: pi on `fortyfive` was answering from a hosted model, not its local default. The journal for 2026-11-27 has what was found. Michael corrected pi's settings and updated pi, and on 2026-11-27 all five pi round trips passed on this tree, none skipped, on pi 1.6.1 with its own default, the local model. So every round trip has passed on this card's code: Claude Code 2.1.331, codex-cli 0.166.0 and pi 1.6.1.

Captured during the build: `a-new-conversation-with-no-model-list-gets-the-claude-effort-sentence`.
