---
title: a turn is inferred from a message
state: evaluating
created: 2026-11-19
tags: [story]
---

A turn is what the harness says it is. It opens when the harness says it has started one and closes when the harness says it has ended, from the harness's own markers. Mega opens none, and no message opens one.

Fourth in the sequence of `simplify-the-core`. Built on 2026-11-25, and kept by Michael after trying it in use.

## why

Today a turn opens in two ways: when mega sends a prompt, and whenever a runtime reports a user message or any output while no turn is open. The second is a guess, and nothing bounds it. Claude Code answers a model switch on an idle runtime with a user message outside any turn. Mega took it for the start of one, no end ever came, and the conversation showed as working for fifty minutes with Interrupt doing nothing. Every build from 2.1.322 on does that.

The first form of this card kept two ways to open a turn, mega's prompt and a start the adapter declares, with rules for when the two overlap. Michael ruled the simpler direction on 2026-11-24: "i think A sounds like a real simplification, which is what we're trying to accomplish. i'd like to give it a try before fully committing to it."

## the failure that becomes impossible

A stray message can no longer open a turn that nothing closes, and mega can no longer show a turn the harness has not started.

## what changes

- **The adapter tells the service when the harness starts a turn.** It is a new call on the sink, beside the ones for an event and a request. It is a signal in memory and nothing is recorded for it.
  - Claude: at each `system/init`.
  - Codex: when Codex names the turn, by `turn/started` or by its reply to `turn/start`, whichever comes first.
  - pi: at `agent_start`.
- **That call is the only thing that opens a turn in the service.** `send` records the prompt and hands it to the runtime, and opens nothing. The rule that a message with no turn open begins one is removed.
- **A recorded turn end from the harness is the only thing that closes one,** besides the runtime's exit, as now.
- **The flag for output after its turn is removed:** `AfterTurn` on an event, the two places the Codex adapter sets it, and its readers in the service and on the page. Late output is recorded and opens nothing, because nothing but the harness's start does.
- **A prompt the harness refuses, and one pi handles by itself, still get the row that says so.** It is a record in the timeline, and it keeps a prompt with no turn from running into the next turn on the page. It closes nothing in memory, since nothing was open.
- **The fakes declare a start as their harnesses do,** so a test that prompts still sees a turn.

## what it costs

A prompt appears at once, and the conversation shows as working when the harness says the turn has begun, a moment later. If a harness never starts the turn, the conversation shows the prompt and no activity, where today it would show as working without end.

In that moment mega does not refuse a second prompt. If two are sent inside it and the harness refuses the second while the first turn runs, the refusal's row ends that turn on the page as a turn's end would. That is left as it is.

## what does not change

Nothing new is written, and no log or registry changes form, so a build from before this piece reads the same files. What a turn ran with is still settled at its first output. How a turn closes. The order of attention. An `after_turn` key in an older log is ignored, which is ruling 2.

## what is already known

Claude sends its start marker for every turn, including one it begins by itself when a background job finishes. On Michael's workspace on 2026-11-24, all 496 Claude turns had a `system/init` before their first output, and 95 of those were turns nobody prompted. Codex's `turn/started` is not kept in mega's logs, so the same count cannot be made for it there; the recorded probe logs can show it.

## how it is shown

- a message outside a turn opens none: the reply Claude gives to a model switch on an idle runtime leaves the conversation idle;
- a prompt shows at once, and the conversation shows as working only once the harness has started the turn;
- a turn the harness starts by itself shows as working before any of its output arrives, and holds a prompt and a shutdown;
- a prompt the harness refuses leaves the conversation idle, with the row that says why, and the next prompt is taken;
- late output from a turn that has ended is recorded and opens nothing;
- the round trips for all three harnesses, including the model switch on an idle runtime;
- nothing in the tree names `AfterTurn`.

## the trial

The implementer leaves the piece in the working tree, with every gate passed, and says so. Michael builds it and runs it for about twenty minutes against his other projects, and it is committed or backed out on what he finds. He watches:

- the moment between send and working, on each harness;
- a Claude conversation waking when a background review finishes;
- whether Interrupt is there when he reaches for it;
- what the shutdown gate takes for idle.

## how it is run

- **It starts by checking each harness's start marker against the recorded probe logs under `probes/`:** that every turn has one, and that each is followed by that turn's end. If a supported build starts a turn with no marker the adapter can trust, the work stops and asks.
- **`codex-interrupt-while-a-turn-is-starting` closes with this piece.** Its fault is an interrupt pressed before Codex has named the turn, and under this rule no turn is open until it has. The report says whether that holds as built.
- **Documents:** "Conversations and runtimes" and "Attention" in `docs/current/mega.md`, the glossary's entry for a turn, and the changelog. The spec's paragraph "A turn is what the harness says it is" is the planning agent's to retire once Michael has kept the piece.
- **Tests and check steps of the removed behaviour go, or are rewritten to the rule.** The report names each. A test or a check that fails for any other reason is a change in behaviour, and stops and asks.
- **The gates are the epic's:** `make test`, all of `make checks`, `make e2e` with the harness versions and every skip named, and terminus on the working tree, then Michael. Run them from a disposable copy of the tree, as piece 3's implementer did, so the Makefile's reinstall of the shim leaves Michael's running mega alone.
- **A terminus finding in code this card did not touch and did not make newly reachable** is matched to an existing card or captured in inbox, and listed at handoff.
- **A finding that is only wording** in a doc or a test of this card's own is fixed and reported at handoff.
- **A finding whose fix would add state, a hold or an ordering rule** is brought to Michael as a question of whether to fix it at all. Anything else this card caused comes to him one at a time.
- **Size.** `internal/service` was 6,163 non-test lines when piece 3 was built. The report gives the figure afterwards, and what was deleted outright.

## as built

Built on 2026-11-25. The adapter reports the harness's start through a new call on the sink, and that call is the only thing that opens a turn. `send` records the prompt and opens nothing. The flag for output after its turn is gone from the model, the Codex adapter, the service and the page. `internal/service` went from 6,163 non-test lines to 6,171: folding the turn's start into one function took six out, and recording pi's handled row in the service put fourteen back.

- **The marker check came first,** in `probes/turn-markers/`. In the recorded probe logs every turn has its start before its end: Claude 89, Codex 54, pi 62. pi sends `agent_start` again for each retry inside one run, so a second start while a turn is open changes nothing.
- **pi's handled row is recorded by the service,** as a refusal's is. pi's `Prompt` signals that it handled the prompt, and the adapter no longer sends a turn end of its own through the sink. Terminus found that such a row, sent while a run was in flight, closed the live turn in memory. Now the only turn end that reaches the service from an adapter is the harness's own.
- **A turn's asked settings and its start time come from the harness's start,** not from the prompt. A Codex model change made between a prompt and Codex naming the turn therefore shows as a mismatch on that turn's line. Left as it is.
- **In the moment between a prompt and the start,** a release sends no interrupt and the shutdown gate counts the conversation at rest.
- **A Codex item reported after its own turn has ended,** with no result row of its own, shows as running if it lands inside the next turn. Left as it is: no event on Michael's workspace has ever carried the old flag.
- **`codex-interrupt-while-a-turn-is-starting` holds as built.** Before Codex names the turn the page shows no Interrupt, and an interrupt sent through the API marks nothing as under way.

**The trial.** Michael built it and ran it against his other projects on 2026-11-25. "so far it doesn't actually feel materially different on either claude or codex conversations." A send followed at once by an interrupt worked. A Claude conversation with a background command showed as working when the command finished and it woke by itself, twice. A pi conversation showed as working through a twenty-second command and was interrupted partway. "honestly, this doesn't really feel any different... and i bet the other users won't even notice."

The last terminus review, `674be2ff2fb8`, was `not_clean` on findings Michael vetoed or the card states as its cost. On the final code `make e2e` passed 19 of 20: `TestCodexMCPApproval` timed out with the turn open and Codex silent for 240 seconds, and passed alone in 13. That stall was seen on 2026-11-23 too and is not explained.

## notes carried from earlier pieces

- **A runtime's exit ends a turn in the service's memory and not in the log.** When a runtime exits in the middle of a turn, the service treats the turn as over, and no turn end is recorded. The page and the paging then join that turn to the next and count the next turn's time from the old prompt. Terminus raised it on `release-only-stops-a-runtime` as `release-merges-unfinished-turn-with-next-turn`, and Michael left it on 2026-11-24. This piece does not change it. If it is ever settled, the place is here: an exit could record the turn's end as it clears it.
- **From piece 1.** `a-codex-question-is-a-message` marks every Codex item `AfterTurn` when it belongs to a turn the runtime saw end, and gave the flag two more readers: the service's settling of what a turn ran with, and the page's live indicator on a tool row. All of that goes with the flag.

## order

After `unread-is-output-beyond-where-you-have-read`, which was the other user of the flag this removes. Lands on `main`.
