---
title: reads after the service closes
state: researching
created: 2026-11-12
tags: [defect]
---

Once the service has closed, every read is refused as `unavailable`, and nothing is kept to answer one from. The page keeps what it last read.

Proposed as the next build in `simplify-the-core`, on 2026-11-24, after `a-failed-write-stops-mega`. It takes the first of the two options this card has carried since 2026-11-12, the one that needs nothing kept.

## why

An orderly stop releases every runtime and then closes the service. The HTTP server is stopped after that, so the page that asked for the shutdown gets its answer. For a read arriving in that window, `Close` sets aside a copy of every conversation's live record, `closedLive`, so the workspace can still be answered from the facts last confirmed. A history read in the window is already refused.

`a-failed-write-stops-mega` made a fault in that window visible. After `Close` nothing is written, so a command that finishes late changes the set-aside copy and records nothing, and a read can show it. Terminus raised it three times on that card, under `single-authority`, and Michael left it for this one.

## the failure that becomes impossible

A read can no longer show what the record does not hold, in the moments after the service has closed.

## what changes

- **Every read still answered after `Close` is refused as `unavailable`,** as the history read already is. The workspace read is the one known to remain.
- **Removed:** `closedLive`, the place `Close` fills it, and the branch of `viewLocked` that reads it.
- **The page keeps what it last read.** When the refusal arrives during a shutdown it shows the shutdown, and no read error.

## what does not change

The order of an orderly stop. The reply to the page's own shutdown request, which still reaches it. The stream's last word that the shutdown finished.

## how it is shown

- a workspace read after `Close` is refused as `unavailable`;
- a command that finishes after `Close`, such as an answer that returns after the shutdown's wait has run out, changes nothing a read can show;
- in the shutdown suite, the page that asked for the shutdown shows it finished, and a page that reads in the window shows the shutdown and no read error;
- nothing in the tree names `closedLive`.

## how it is run

- **It starts by listing every read the HTTP surface answers and what each returns after `Close`.** One that would need something kept to be refused cleanly is brought to Michael.
- **Documents:** the shutdown passage of `docs/current/mega.md` and its HTTP reference, the changelog, and the sentence `a-failed-write-stops-mega` added about a command that finishes after `Close`.
- **Tests and check steps of the removed behaviour go, or are rewritten to the rule.** The report names each. A test or a check that fails for any other reason is a change in behaviour, and stops and asks.
- **The gates are the epic's:** `make test`, all of `make checks`, `make e2e` with the harness versions and every skip named, and terminus on the working tree, then Michael.
- **A terminus finding in code this card did not touch and did not make newly reachable** is matched to an existing card or captured in inbox, and listed at handoff.
- **A finding that is only wording** in a doc or a test of this card's own is fixed and reported at handoff.
- **A finding whose fix would add state, a hold or an ordering rule** is brought to Michael as a question of whether to fix it at all. Anything else this card caused comes to him one at a time.
- **Size.** `internal/service` was 6,222 non-test lines when `a-failed-write-stops-mega` was built. The report gives the figure afterwards.

## as first written, 2026-11-12

An orderly stop releases every runtime and then closes the service, which lets go of each conversation's live state. The HTTP server is stopped after that, so the page that asked for the shutdown gets its answer. A read in that window is answered from the registry alone: `last_seq` is 0, and there is no pending set, background list, facts or environment. Attention had no value at all, and an open page holding it went blank when it went stale. The UI test runner batch fixed attention, which now comes from the registry (unsupervised, unread or idle). The rest of the view is still a torn-down one.

Worth deciding: a closed service could answer reads with `unavailable`, so a page keeps what it last read and says so, while the shutdown's own 202 still reaches the page that asked. Or the window could be closed by stopping reads before `Close`. Seen in `checks/shutdown.mjs`, about one run in twenty before the attention fix; no other wrong value has been seen on a page.

## since this was written

The small-interface batch (terminus `023ffccec7e5`, 2026-11-12) closed part of it while fixing its own findings: each conversation's last confirmed state is set aside at shutdown, so an error stays an error, and a history read after the service has closed is refused as `unavailable` instead of reopening a log from disk. What remains is the workspace read in that window, which is still answered from the registry alone, with `last_seq` 0 and no pending set, background list, facts or environment. The decision above still stands for that read: answer `unavailable`, or stop reads before the close.

Renamed from `reads-after-close` on 2026-11-12 so the filename matches the title.


## before the registry gives up its runtime state, 2026-11-19

A read in that window is answered from the registry alone. `the-registry-records-no-runtime-state`, the last piece of `simplify-the-core`, takes the runtime state out of the registry, so such a read would then have even less to say. The decision above wants making before that piece is built. Answering `unavailable` once the service has closed is the one that needs nothing kept.
