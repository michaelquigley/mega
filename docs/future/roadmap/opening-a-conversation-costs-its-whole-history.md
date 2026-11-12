---
title: opening a conversation costs its whole history
state: evaluating
created: 2026-11-12
tags: [defect]
---

Opening or switching to a conversation loads its whole history from the first event, and a long conversation grows heavier every day. Make it cost what is recent: load the last several turns, reach older history with a button, stop re-reading the log for every request, and stop sending events the page hides.

## why

Measured 2026-11-12 on Michael's workspace after three days of real use. The largest conversation holds 16,199 events in 42 MB, four more are between 12 and 19 MB, and the workspace totals 136 MB. Three separate causes make a long conversation heavy:

- **Every switch reloads from the beginning.** The page holds one conversation's history at a time and fetches the next from event zero.
- **The server parses the whole log for every history request.** A read that returns nothing takes 0.42 s on the largest log, the same as one returning its last 200 events; the whole history takes about a second and 50 MB.
- **Every change re-renders the whole list.** Markdown is cached per reply, but each streamed chunk still walks every event and reconciles thousands of rows.

Three quarters of the largest log's events are native passthrough events the page hides by default.

## shape

- **The recent tail.** Opening or switching fetches the last ten turns and the turn in flight. A turn comes whole: its prompt, every tool call with its result, its requests and its replies, starting at the turn's first event, so nothing appears without what it belongs to. Events between turns, such as runtime lines, travel with the turn that follows them.
- **A button for the rest.** "Show earlier" at the top of the conversation loads the ten turns before what is shown and keeps the view where it was; "show all" loads everything. No loading on scroll. The page says how much earlier history there is.
- **An index in the log.** The service already scans each log once when it opens it. It keeps, in memory, where every few hundred events begin and where each turn starts, and maintains both as it appends. A read then seeks instead of parsing from the top.
- **Hidden events are not sent.** History reads leave out native events unless asked for them; ticking the native events box refetches what is shown with them included. Native events arriving on the stream while the box is off are dropped by the page.
- **The reading position** follows what the page has been given, as now: the newest event of the loaded tail. Attention is computed by the service and is unaffected by what a page has loaded.

## so the next slow load explains itself

A half-minute wait on a new tab on 2026-11-12 could not be reproduced or attributed; the server's log for the slow launch and an instant one were identical.

- The service logs any request that takes longer than a second, with its path, duration and response size.
- The page's connecting and loading states say what they are waiting for and for how long: which request, how many attempts, the last error if any.

## what this costs

The browser's find-in-page sees only what is loaded, and the start of a long conversation is one click on "show all" away. Accepted, for a workspace whose action is at the bottom. If the button proves awkward in use, loading on scroll is the next step.

## checks

Log tests for the index: a tail by turns, a page before a given event, a read after a given event, each on a log large enough to cross several index points, and the index staying right across appends and a reopen. A service test that a tail begins at a turn's first event and never splits a tool call from its result. The browser check with a generated conversation of twenty thousand events and tens of megabytes: first content in well under a second on a fresh page, the view at the newest activity, "show earlier" adding ten turns with the position held, "show all", the native toggle refetching, and a streamed reply re-rendering only the loaded tail.

## not in this card

Fetching raw payloads on demand; loading on scroll; skipping off-screen rendering; start-up, which has its own card; compacting logs on disk.
