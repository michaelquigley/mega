---
title: an answer before the runtime is installed is aborted
state: inbox
created: 2026-11-23
tags: [defect]
---

An answer to a request that reaches mega during a launch is recorded aborted, although the harness never saw it and may still be waiting. Found by Terminus `aeea0513af51` (`answer-retires-request-before-runtime-is-installed`) on `a-first-launch-holds-nothing-back`. Michael first vetoed it there, as older than that card and outside its scope. He then took the refusal into that piece on 2026-11-23, once it was found that the piece opens the window on a pi first launch.

**Fixed in `a-first-launch-holds-nothing-back`.** `Answer` (`internal/service/conversation.go`) refuses an answer with `conflict`, "the runtime is starting", when `l.runtime` is nil. The check comes after the settled-but-unrecorded check and before the reservation, and the request stays pending. `TestAnAnswerDuringALaunchIsRefusedAndTheRequestStaysPending` (`internal/service/start_test.go`) covers it. The card can be removed.

- **The window.** A runtime's callbacks reach the service while `Launch` is still running, and a request joins the pending set as soon as its event is recorded (`sink.Request`, `internal/service/live.go`). `startRuntime` (`internal/service/conversation.go`) sets `l.runtime` only after `Launch` returns and the service lock is taken again.
- **What an answer did in it, before the fix.** `Answer` reserved the request, found `l.runtime` nil, and took `harness.ErrNotPending` in place of the harness's reply. It recorded the outcome `aborted` and removed the card. Nothing reached the harness.
- **Where it can happen.** On a resume the window exists at `8503877`, before the queue's removal. pi is the adapter that reaches it. Its runtime starts relaying frames (`go r.work()`, `internal/harness/pi/pi.go`) before `Launch` returns, and `uiRequest` (`internal/harness/pi/runtime.go`) turns an extension's dialog into a request with no turn needed. An extension that asks at session start can therefore raise one during the launch. Since the queue's removal the same holds on a first launch: the queue used to hold such a request until the runtime was installed.

The answer that needs no new machinery is a refusal, not a hold. An answer that arrives while the runtime is still being installed is refused with "the runtime is starting", and the request stays pending. This is what the model and mode controls already do in that window, on `l.busy` (`internal/service/model.go`, `internal/service/permission.go`). The operator answers again once the launch has returned.
