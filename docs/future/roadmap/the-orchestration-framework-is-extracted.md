---
title: the orchestration framework is extracted
state: horizon
created: 2026-11-18
tags: [epic, spike]
---

The part of mega that drives harnesses, and the part of mercurius and terminus that does the same, become one framework that stands apart from any of the three. It is the foundation for harness orchestration that is native to the overlay.

## why

From Michael's presentation of mega, 2026-11-18, the first of seven future directions: "the harness orchestration framework in mega and mercurius/terminus is extracted. this becomes the foundation for an overlay-native harness orchestration solution."

Three tools now start, prompt, watch and stop coding harnesses, each in its own way. Mega has adapters for Claude Code, Codex and pi behind one interface, with a service that owns their lifecycle, their requests and their logs. Mercurius and terminus each run a reviewer of their own. What mega learned the hard way, exact-id resume, a pending request never answered for the operator, a release that reports what it could not stop, version tolerance, is exactly what the other two need and do not share.

## what is already shaped like a framework

- The adapter interface in `internal/harness`, with three implementations and a fake for each.
- The capability report and its rerunnable probes, which are the evidence any adapter is written from.
- The canon's qualities for this work: the protocol confined to adapters, honest runtime state, permissions never weakened.
- The service's rules for one authority and durable records.

## what the extraction has to settle

- **Where the line falls.** Adapters alone, or adapters with the lifecycle, the request model and the event log. The second is what makes a second consumer cheap, and is much more to pull out.
- **What "overlay-native" asks of it.** An environment that runs harnesses and is reached over the overlay, not over a local port. That is the next direction, `central-application`, and this card is what it stands on.
- **The Claude bridge.** One adapter runs through a Node program beside the binary. A framework others build on has to carry that, which `first-tagged-release` meets first.
- **Mercurius and terminus as its first other users.** Their reviewers become harness runs like any other, restartable and observable, which is also what the hub epic asks of them.

## raised again, 2026-11-19

Michael wrote a second card for this the day after the presentation, in the inbox as `harness-orchestration-framework`. It was folded in here. His words: "we need to consider separating out the harness orchestration components used by mega, mercurius, and terminus... into a new 'harness orchestration framework' that can be used as a shared foundation for these projects, as well as new projects."

What that adds to the above: new projects are named as users of it, not only the three that exist.

A first step was carded as `one-kit-under-the-adapters`, the ninth piece of `simplify-the-core`, and was folded into this card on 2026-11-24. It changes no rule and removes no case, so it is the start of this work and not a simplification of mega's model.

It moves four things each adapter does for itself into one place under `internal/harness`: checking that a stream or a file belongs to the session asked for, refusing what would start work once a stop has begun, finding what a runtime left running, and ordering an answer against the harness's own settlement of the same request. The kit holds the order and the decision, and each adapter hands it the evidence its harness has. What is left in an adapter after it is the translation of one harness's protocol, which is the shape a framework would want.

- **No rule changes.** What each adapter does today is the specification, and every adapter test passes unchanged. The three differ in what evidence their harness gives, and those differences stay.
- **Less is duplicated than when it was carded.** Finding what a runtime left running is already shared. The session check is almost all Claude's, since Codex and pi take their id from one value. What is written three times is the stop fence and the answer ordering.
- **One test it adds,** for a case none holds today: an answer to a pi dialog after its deadline has passed and before its timer has removed it is refused.
- **One limit stays, recorded and not engineered around.** Codex may have settled a request while its notice is still on the pipe, and an answer written in that moment is recorded as answered though Codex ignored it. Michael declined a fourth outcome for it on 2026-11-19.
- **The card `an-answer-racing-a-release-retires-a-live-pi-dialog` belongs with this step.**

## depends on

Nothing built yet. It is the first of the directions because the next two rest on it.
