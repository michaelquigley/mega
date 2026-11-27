---
title: the page has one value for where it stands
state: researching
created: 2026-11-19
tags: [story]
---

The page works out where it stands in one place, and everything on it reads that: the footer, the rail, the shutdown banner, the held controls and the read.

Eighth in the sequence of `simplify-the-core`.

## why

About eight of the hardening findings were the page's sense of whether it is current: "Live" before it had caught up, a shutdown banner shown when it should not be, reads of output not yet seen, controls live while stale. The pieces that answer them exist now, in `ui/src/stream.ts` and `ui/src/App.tsx`, and each was fitted separately.

## the failure that becomes impossible

Two parts of the page can no longer disagree about whether it is current.

## what changes

The value has two parts that are true at the same time:

- **What the page knows of the workspace:** the five values `stream.ts` already has.
- **What the service last said about itself, if anything:** shutdown asked, tied to its gate as now, or shut down.

One function works it out. Two rules keep it from depending on what arrives first:

- **A read of the workspace counts only if it was asked for on the connection the page is on now.** That is about this value alone, and not about how a conversation's history is read.
- **The reply to the page's own shutdown request sets nothing when it succeeds.** Today the page marks its shutdown as accepted when the reply arrives and learns that it finished from the stream's last frame, and the two can come in either order. After this the page marks its request when it sends it, tied to the gate it was showing, and takes the mark back if the service refuses. That the shutdown finished comes only from the stream.

While the mark stands and the stream has not said finished, the banner says only that the shutdown was asked for. It claims nothing about what the service is doing.

## what does not change

The five values, the gate, and what the page looks like when nothing has gone wrong. No state is added.

## how it is shown

- the side, shutdown and panel suites;
- a page that reconnects to a replacement service already armed for shutdown shows that gate and not the one it remembered;
- a workspace read that fails on a connected stream, on a page that already shows a workspace, reads "Read failed, retrying", holds the controls and acknowledges no read;
- a workspace response held back until the page is on a later connection changes nothing;
- the outgoing shutdown request held in the browser: the banner says it was asked for, and not that runtimes are being released;
- the reply to a shutdown request, held back until after the stream's last frame and the socket's close, leaves the page showing the shutdown as finished; a refusal takes the mark back and says why.

## order

It was to follow a piece that gave a conversation's events one route to the page. That piece was set aside on 2026-11-25, so this one takes the page's two routes as they are: it is about what the page knows of the workspace, not about how a conversation's history is read. Lands on `main`.

## note, 2026-11-25: behind on the open conversation's history

Terminus raised `reconnect-gap-shows-completed-tool-running` (blocking, `honest-runtime-state`) on `a-failed-read-after-a-reconnect-is-tried-again`. A tool finishes while the stream is down, and its turn goes on. After the reconnect the workspace read lands before the history read brings the tool's result. The page is then no longer last known, the old tool row shows its spinner, and the activity line can name the finished tool as current work.

The window opens on every reconnect, for as long as the history read takes. Before that card, a failed history read was never tried again, so the window stayed open until the conversation was opened again or the stream reconnected. That card shortened it to the retry's backoff. It was not fixed there, at Michael's word.

Being behind on the open conversation's history is part of where the page stands: the conversation's `last_seq` in the workspace is past the page's `through`. The tool spinner and the activity line's tool name are two readers that could take it from this card's one value, rather than each working it out or not at all.
