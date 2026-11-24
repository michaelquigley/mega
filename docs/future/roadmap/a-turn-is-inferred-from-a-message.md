---
title: a turn is inferred from a message
state: researching
created: 2026-11-19
tags: [story]
---

A turn opens on a fact: mega sent a prompt, or the adapter says the harness started one, from the harness's own marker. The service no longer takes a message it happens to see for the start of a turn.

Fourth in the sequence of `simplify-the-core`. It builds the first of the three recommendations below; the other two are new things for the page to say and wait here.

## the failure that becomes impossible

A stray message can no longer open a turn that nothing closes.

## what changes

A turn opens in two ways:

- **Mega sent a prompt,** as now.
- **The adapter says the harness started one.** This is a turn nobody prompted, such as Claude waking when a background job finishes. Where the harness also announces a turn mega opened, the announcement changes nothing about it. No adapter decides a start from the output it happens to see.

What goes is the third way a turn opens today: any message from a runtime while no turn is open. A message outside any turn is recorded and opens nothing. The flag for output after its turn goes with it.

Two things follow from a start that can arrive by itself:

- **The check that no turn is open is made where the prompt is recorded,** under the same lock. Today it is made earlier, and a declared start could arrive in between.
- **Once the harness has declared a start, only the harness's own end closes the turn.** An end that mega or an adapter makes up for a prompt, a refusal or pi's "handled" reply, closes only a turn the harness has not declared. A start can arrive after a prompt is recorded and before it is delivered, and mega cannot tell it from the harness's notice of the prompt's own turn. It does not need to.

## what does not change

How a turn closes otherwise. No state is added between idle and running.

## how it is shown

It starts with a probe of each harness for its marker of a turn it starts by itself, recorded with its version under `probes/`. The capability report's record of a Claude wake is the place to start for Claude. If a supported build gives no marker the adapter can trust, the work stops and asks.

- the round trips for all three harnesses, including the model switch on an idle runtime, which is the case that started this;
- a message outside a turn opens none;
- a turn the harness starts by itself shows as running before any of its output arrives, and holds a prompt and a shutdown;
- a harness's own start notice for a turn mega opened leaves that turn as it was;
- a declared start that arrives between a prompt's first check and its recording: the prompt is refused and the turn stands;
- a declared start that arrives after a prompt is recorded and before it is delivered, and the harness then refuses the prompt, or pi reports it handled: the conversation stays running, and holds a shutdown, until the harness's end.

## order

After `unread-is-output-beyond-where-you-have-read`, which is the other user of the flag this removes. Lands on `main`.

## why

Pull request 3 fixed one case of a general fault. Claude Code answers a model switch on an idle runtime with a user message outside any turn. Mega took it for the start of a turn, no end ever came, and the conversation showed as working for fifty minutes with Interrupt doing nothing. The follow-up on 2026-11-19 found that every Claude Code build from 2.1.322 on does this, so it had been broken inside the tested range all along, and asked the wider question.

The answer: the service opens a turn whenever a runtime reports a user message, or any assistant or tool output, while no turn is open. Only the harness's own turn end closes it. Nothing bounds it. So any message a harness sends outside a turn can do what the echo did. Candidates the implementer named, none yet seen: for Claude, a compaction summary or a hook's message; for Codex, an item arriving after its turn that the adapter does not mark as late; for pi, a message an extension injects outside a run.

## the implementer's recommendation

- **Each adapter reports the start and the end of a turn explicitly,** from its harness's own markers, and the service stops inferring a turn from a message.
- **Never end a turn on a guess.** Show how long an open turn has had no activity, and leave the judgement to the operator.
- **An interrupt that gets no answer at all** shows the turn as "interrupt not answered", and offers Release.

## to settle

What each harness's own markers are, and whether all three have both a start and an end the adapter can trust. What a turn the harness opens by itself looks like, such as Claude waking when a background job finishes, which is a real turn nobody prompted. And how this meets `honest-runtime-state`: a turn shown as running with nothing happening is a claim the service cannot stand behind for long.

## from piece 1, 2026-11-20

- **What piece 1 marks.** Piece 1, `a-codex-question-is-a-message`, marks every Codex item `AfterTurn` when it belongs to a turn the runtime saw end.
- **The case it leaves, vetoed for this card.** An item for a turn the runtime never saw end, and that is not the active one, still opens a turn: one that ended before a resume, say. Terminus pass `243b6eb30af0` found it, and Michael vetoed it in piece 1 for this card. Telling it from output of a turn still starting would need state for an outstanding `turn/start`, and this card removes the rule that makes it a fault.
- **More readers of the flag.** Piece 1 also gave `AfterTurn` more readers, which this card's removal of the flag has to replace:
  - the service's first-output call to `ranLocked`, so late output never sets another turn's run settings;
  - the page's live indicator on a tool row.

## depends on

Nothing. The earlier batch addressed this question for recorded history. See item 7 under “What changed” in [the November 19 journal](../../journal/2026-11-19.md#what-changed).

## from the review of 2026-11-23 and its removals

- **A turn nobody prompted is ordinary.** On Michael's workspace 68 of 834 turns ended without a prompt from mega, in 8 conversations. Most began minutes after the turn before, with a shell command or a broker's collect call: an agent waking when its background work finishes. A turn the harness starts has to show as a real turn.
- **A runtime's exit ends a turn in the service's memory and not in the log.** When a runtime exits in the middle of a turn, the service treats the turn as over, and no turn end is recorded. The page and the paging then join that turn to the next and count the next turn's time from the old prompt. Terminus raised it on `release-only-stops-a-runtime` as `release-merges-unfinished-turn-with-next-turn`, and Michael left it on 2026-11-24 to be decided with the turn model. It is two tellings of one fact, which `one-fold-for-what-happened` exists to end.
