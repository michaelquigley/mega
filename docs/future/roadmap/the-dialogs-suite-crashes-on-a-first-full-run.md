---
title: the dialogs suite crashes on a first full run
state: evaluating
created: 2026-11-24
tags: [defect]
---

On a first full `make checks`, the `dialogs` suite sometimes crashes at the step "secret registered" in the register-project dialog, and then passes 58 of 58 when run on its own. Find why, and make a full run pass or fail on the code.

## why

Two implementers met it on two days, on changes that do not touch the register dialog: on 2026-11-23 while building `a-first-launch-holds-nothing-back`, and on 2026-11-24 while building `release-only-stops-a-runtime`. Each time a second full run passed. A gate that fails once and passes on a rerun teaches everyone to rerun, and the day it fails for a reason nobody will look.

It is not known whether the cause is the suite's order, something left by the suite before it, or timing under load.

## as built

Built on 2026-11-24. The cause was in the check. The suite typed the secret path the moment the second register dialog opened. The dialog fills its field with `~/Repos/` when its first listing arrives, and Playwright's `fill` selects the field and inserts the text as two steps. When the prefill landed between them the path was appended to `~/Repos/`, the service refused it with a 400, and the step timed out. All three earlier crashes had logged that 400.

`openRegister` in `checks/dialogs.mjs` now waits for the field to hold `~/Repos/` before it types, as the suite's first open already did. `probes/register-click/` forces the interleaving and reproduces the 400 every run. Ruled out: a click lost to the listing moving the button, load alone, and the registration waiting under the service lock.

The same thing can happen to a person who types the instant the dialog opens. That is the card `the-register-dialogs-prefill-can-land-on-what-is-typed`.
