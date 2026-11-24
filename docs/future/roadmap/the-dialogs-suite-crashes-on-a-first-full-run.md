---
title: the dialogs suite crashes on a first full run
state: researching
created: 2026-11-24
tags: [defect]
---

On a first full `make checks`, the `dialogs` suite sometimes crashes at the step "secret registered" in the register-project dialog, and then passes 58 of 58 when run on its own. Find why, and make a full run pass or fail on the code.

## why

Two implementers met it on two days, on changes that do not touch the register dialog: on 2026-11-23 while building `a-first-launch-holds-nothing-back`, and on 2026-11-24 while building `release-only-stops-a-runtime`. Each time a second full run passed. A gate that fails once and passes on a rerun teaches everyone to rerun, and the day it fails for a reason nobody will look.

It is not known whether the cause is the suite's order, something left by the suite before it, or timing under load.
