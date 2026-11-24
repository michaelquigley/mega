---
title: an answer racing a release retires a live pi dialog
state: horizon
created: 2026-11-23
tags: [defect]
---

An answer that crosses a Release can be recorded as aborted while pi still holds the dialog open. Found by Terminus `5b57188f1115` (`release-fence-retires-live-dialog`) on `a-first-launch-holds-nothing-back`. Michael vetoed it there on 2026-11-23: the code is unchanged since `8503877`, and the race is with a release, not a launch.

- **The race.** `Answer` (`internal/service/conversation.go`) reserves the request, lets go of `s.mu` and calls the runtime. Release can fence the runtime in that gap (`rt.Stopping()`) before pi's `Answer` takes its lock.
- **What pi does.** Once fenced, pi's `Answer` (`internal/harness/pi/runtime.go`) refuses an answer that would let work go on and returns `harness.ErrNotPending`. The dialog stays in its map, and pi still waits on it.
- **What the service does.** `Answer` reads `ErrNotPending` as the harness having settled the request. It records `aborted`, removes the card and with it Dismiss, and tells the operator the request was settled before the answer arrived.

How much it matters:

- **It needs one operator to answer and release in the same instant.** The answer has to be on its way to the adapter when the fence lands.
- **Release ends pi's process anyway.** A pending dialog holds pi through the close, and the adapter ends it in steps (SIGTERM, then SIGKILL). The dialog is gone with the process, and the command it gated never runs. What is wrong is only the record: `aborted`, and the card gone, before the harness settled it or exited.

The stop fence and the ordering of an answer against the harness's own settlement are two of the four mechanisms the first step of `the-orchestration-framework-is-extracted` gathers into `internal/harness`. This belongs there, with no separate fix.
