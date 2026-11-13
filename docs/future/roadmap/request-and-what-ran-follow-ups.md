---
title: request and what ran follow-ups
state: horizon
created: 2026-11-13
tags: [enhancement]
---

What the review of `the-panel-shows-the-request-the-turn-shows-what-ran` left open, 2026-11-13. None blocked the batch.

## leaving plan mode leaves the request behind

When Claude changes its own mode, the request on the panel does not follow. The common case is plan mode: Michael requests `plan`, approves the plan, and Claude leaves plan mode. The panel still says `plan`. Every later turn's line then stays visible in the notice colour, "ran ask, asked plan", until he changes the control, and the next launch passes `plan` again.

That is honest and errs strict, and it is noise in a routine flow. Before the batch the bar showed the live mode while running, so this is new.

- **To observe first.** What mode Claude reports after an approved exit from plan mode under the SDK, recorded in the capability report with its version. Nothing there says today.
- **The choice.** Leave it, and let the line prompt one press on the control. Or let the request follow when the change comes from Michael's own approval of the exit, limited to modes mega offers, recorded as an event that says why. The second needs a sentence in `permissions-never-weakened`, which now says only the operator changes a conversation's mode.

## the default row is not a name

A conversation whose settings name no model the list holds copies the list's default row, and the bar shows "Default". Claude resolves it to a real model that moves with its versions. The bar could show what the row resolves to now, so the request reads as a model and not as the unset state this work removed. Michael's own settings name a model, so he does not see it.

## the copy guesses when the list has no default

`copyFor` takes the list's first model when no row is marked default. Neither harness produces such a list today. It should ask or refuse, as a missing Claude effort does, not pick.

## no round trip asserts a clean turn

The e2e round trips pass, and none asserts that a real turn records no difference between what was asked and what ran. A mismatch in how a harness names a model against how its list resolves it would mark every turn. Add the assertion to the model and effort round trips for both harnesses.
