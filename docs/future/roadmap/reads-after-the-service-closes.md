---
title: Reads after the service closes
state: inbox
created: 2026-11-12
tags: [defect]
---

An orderly stop releases every runtime and then closes the service, which lets go of each conversation's live state. The HTTP server is stopped after that, so the page that asked for the shutdown gets its answer. A read in that window is answered from the registry alone: `last_seq` is 0, and there is no pending set, background list, facts or environment. Attention had no value at all, and an open page holding it went blank when it went stale. The UI test runner batch fixed attention, which now comes from the registry (unsupervised, unread or idle). The rest of the view is still a torn-down one.

Worth deciding: a closed service could answer reads with `unavailable`, so a page keeps what it last read and says so, while the shutdown's own 202 still reaches the page that asked. Or the window could be closed by stopping reads before `Close`. Seen in `checks/shutdown.mjs`, about one run in twenty before the attention fix; no other wrong value has been seen on a page.

## since this was written

The small-interface batch (terminus `023ffccec7e5`, 2026-11-12) closed part of it while fixing its own findings: each conversation's last confirmed state is set aside at shutdown, so an error stays an error, and a history read after the service has closed is refused as `unavailable` instead of reopening a log from disk. What remains is the workspace read in that window, which is still answered from the registry alone, with `last_seq` 0 and no pending set, background list, facts or environment. The decision above still stands for that read: answer `unavailable`, or stop reads before the close.

Renamed from `reads-after-close` on 2026-11-12 so the filename matches the title.

