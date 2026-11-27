---
title: central application
state: horizon
created: 2026-11-06
tags: [epic]
---

The central application on `apps.hq` at `roundhouse.hq.quigley.com`: local username/password login, per-environment enrollment and revocation, environment services dialing out over persistent authenticated connections, relay of commands and state, cross-project views assembled from several environments, and a last-seen cache that is never an authority. Brings `twentytwo` in as the second environment. Transport choice (plain outbound WebSocket through nginx or a private zrok share) is decided here.

## background

Noted by the planning agent on 2026-11-14. This card was written before the rename and before `twentytwo` ran a mega of its own. Two things have moved since. The hostname is no longer settled, since the project is now mega. And `twentytwo` is not waiting to be brought in: it runs its own independent workspace, so this card's first job is joining two workspaces that already exist, which is also what `moving-conversations-between-hosts` wants in a smaller form.

## the second direction, 2026-11-18

From Michael's presentation: "mega grows multiple environment support. today mega is a single service that contains the harness orchestration and the user interface. future mega separates these into any number of harness orchestration environments, reachable anywhere, consolidated into a single pane of glass."

That is this card, in his present words, and it sharpens two things the card of 2026-11-06 left loose:

- **The split is between orchestration and interface.** An environment runs harnesses; the pane of glass shows many environments. Today's mega is both in one process. `the-orchestration-framework-is-extracted` is what makes the first half something that can stand alone.
- **Reachable anywhere means the overlay.** He already reaches a mega from another machine through a private share, and each page already says which mega it is. Those are the single-environment version of this.

What the first card got right and still holds: environments dial out, nothing is published, a login stands in front, and a cached last-seen state is never an authority.

## how a recorded event reaches a page on another machine, 2026-11-25

`simplify-the-core` tried giving recorded events one route to the page: the stream carried an event's number, and the page read the event. On one machine a read costs about a millisecond. Through a relay it is a second trip for every event, and while a read is slow a tool row keeps its spinner and the start of a streaming reply can be cleared. It was set aside for that, and for adding lines where it was meant to remove them. The work is on the branch `tried/one-route-for-recorded-history`.

The spec leaves "the event retention and replay contract once a central application caches state" open, and this is part of it. Two shapes are on the table for when it is designed:

- **Announce and read,** as tried: simple to make whole after a reconnect, and two trips for every event.
- **The stream as the one route:** it carries the events and resumes from the last one the page was given, so nothing is read for live events at all. That is one trip, and it needs the service to replay from a number.

Today the page has both routes and merges them, in `ui/src/log.ts`.

## what the page knows of where it stands, 2026-11-26

`simplify-the-core` had a piece, `the-page-has-one-value-for-where-it-stands`, to gather into one function what the page knows of the workspace and what the service last said about itself. It was taken out on 2026-11-26 and its card removed. It changed almost no behaviour, and a page that reaches an environment through this application has more to know: whether the application answers, whether the environment is connected to it, and whether what is shown is live or last seen. So the one value is drawn here, once, for both hops. Michael: "if it's likely to be an obvious cleanup during that work", no card is needed for it.

What that card had worked out, for whoever draws it:

- **The page's standing has two parts, both true at once:** what it knows of the workspace (nothing yet, unreachable, unreadable, current, or only what it last read, the five values in `ui/src/stream.ts`), and what the service last said about itself (shutdown asked, or shut down).
- **A read of the workspace counts only if it was asked for on the connection the page is on now.**
- **The reply to the page's own shutdown request sets nothing when it succeeds.** That the shutdown finished comes only from the stream. Until then the banner says only that it was asked for.
- **Being behind on the open conversation's history is part of where the page stands.** After a reconnect the workspace read can land before the history read, and a tool that finished meanwhile keeps its spinner until the history arrives. Terminus raises this as `reconnect-gap-shows-completed-tool-running`. The window opens on every reconnect and closes when the read lands. It was left as it is, at Michael's word, on 2026-11-25. A review that raises it again is matched to this card.

Cases the card listed to show it right: a page that reconnects to a replacement service already armed for shutdown shows that gate and not the one it remembered; a workspace read that fails on a connected stream says so, holds the controls and acknowledges no read; a workspace response that arrives after the page has moved to a later connection changes nothing; a shutdown request still in the browser reads as asked for, and not as runtimes being released; a reply to a shutdown request that arrives after the stream's last frame leaves the shutdown showing as finished, and a refusal takes the mark back.
