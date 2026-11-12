---
title: opening a conversation costs its whole history
state: researching
created: 2026-11-12
tags: [defect]
---

A fresh browser tab loads a conversation's entire event log, raw payloads included, before it shows anything. Make opening a conversation cost what is on screen: send the recent tail first, leave raw payloads on the server until asked for, and stop scanning every log at start-up.

## why

Measured 2026-11-12 on Michael's workspace after three days of real use, while looking into a half-minute wait he saw on a new tab opened from the tray. That wait is not explained by these numbers: a new tab opens with no conversation selected and reads only the workspace, about 50 milliseconds, and a second launch populated at once. The cost below is paid when a large conversation is opened, and it is real regardless:

- the largest conversation holds 16,199 events in 42 MB; four more are between 12 and 19 MB; the workspace totals 136 MB;
- 86 percent of the largest log is raw harness payloads, and 11,918 of its events are native passthrough events the page hides by default;
- `GET /api/conversations/{id}/events?since=0` for it returns about 50 MB, which the server sends in a second and the page then parses and renders in full, some 590 replies through the markdown renderer and 1,500 tool rows;
- start-up took 3.6 seconds from process start to serving on both launches that day, spent opening and scanning every log.

A tab that stays open reconnects and fetches only what it missed, so the cost shows on a new tab and after any reload, and grows every day.

## shape

- **Raw payloads stay on the server.** History reads and the stream omit `raw` by default. The page fetches one event's raw payload when the operator expands it under the native events toggle. Copy and display already use the event's own text.
- **Native events are not sent unless asked for.** The history read takes a flag for them, set when the toggle is on.
- **The tail first.** Opening a conversation fetches the newest turns, enough to fill the view and a little more, and fetches older history as the operator scrolls up, keeping the scroll position. The reading position and the follow-at-bottom rule work from the tail. A conversation's total length is known from its last sequence number without loading it.
- **Start-up does not read every log.** A log is opened and scanned when its conversation is first needed, or its last sequence number and seen identities are kept in a small sidecar the service writes beside the log and can always rebuild from it.
- **Rendering is bounded.** Whatever is loaded but far off screen need not be rendered; at minimum the markdown rendering of a message is done once and kept.

## so the next slow load explains itself

The half-minute wait on 2026-11-12 could not be reproduced or attributed: the server's log for the slow launch and the instant one were identical, with no errors. Two small diagnostics would have settled it.

- The service logs any request that takes longer than a second, with its path, duration and response size.
- The page's connecting and loading states say what they are waiting for and for how long: which request, how many attempts, the last error if any. A page that has been waiting more than a few seconds says so in words the operator can repeat.

## checks

A service test for reads without raw, with native events excluded, and for a tail read with a backward page; a start-up test that does not open an untouched conversation's log. The browser check with a generated conversation of twenty thousand events and tens of megabytes: time to first content under a second on a fresh page, the view at the newest activity, older history arriving on scroll-up with the position held, a raw payload fetched on expansion.

## not in this card

Compacting or archiving old logs on disk; the log's on-disk format.
