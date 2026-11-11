---
title: favicon
state: researching
created: 2026-11-09
tags: [enhancement]
---

Give mega a mark: a favicon for the page, in the house form, that also tells the operator from the browser tab when something needs him.

## the mark

The house pattern is one `ui/public/favicon.svg`, linked as `image/svg+xml`, as ranger, pane and reckon do; ranger renders its tray and desktop icons from the same file. Mega has only an empty placeholder icon today.

**Decided 2026-11-11: the hub.** One gold centre with five light points around it, on the dark tone: many agents, one operator. The draft is `images/favicon-hub.svg`; it is a direction, not final art, and its proportions need checking at 16 pixels, where the points are barely more than a pixel across. A heavy M was ruled out, and a mark built from the workspace's own attention shapes was the second choice.

## shape

- The chosen mark as `ui/public/favicon.svg`, linked from `ui/index.html`, replacing the placeholder.
- **The tab says when something needs the operator.** While the needs-you list is not empty, the icon inverts: the same mark on the notice gold with dark points, a change that reads at tab size where a corner dot would be lost beside the mark's own gold centre. The title is prefixed with the count, `(2) metawoo mega`. The page swaps the icon link between two embedded SVGs; nothing is fetched. The shutdown gate's "Ready to shut down" title takes precedence while it applies. While the page shows last-known state, the dot is not shown as live.
- PNG renderings of both states of the mark at 32, 48, 192 and 512 pixels and a tray size, checked in for the daemon card to use, with the command that renders them recorded beside them.

## checks

The browser check with the fake shim: the icon link's content with nothing waiting, with a request pending, and after it is answered; the title's count; the shutdown title winning; no request leaving 127.0.0.1.

## not in this card

A wordmark or logo for documentation; animation in the tab for working conversations; the tray icon's behaviour, which belongs to the daemon card.
