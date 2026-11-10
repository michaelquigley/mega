---
title: operator panel expansion behavior
state: researching
created: 2026-11-10
tags: [enhancement]
---

Two problems with the expanding input panel, found in use the day it landed.

- **Expanding hides what Michael was reading.** The panel is anchored at the bottom and grows upward, so opening it covers the lines just above it.
- **Sending leaves the panel open and the view where it was.** After Ctrl+Enter he is looking at a tall empty box, not at the agent starting to answer.

## Shape

- **The conversation moves with the panel's top edge.** When the panel opens, the history scrolls by exactly the height the panel gained, so the line that sat just above the panel before still sits just above it after. Closing reverses it. If the view was following the bottom, it is still following the bottom afterwards. A drag of the resize handle does the same, continuously.
- **Sending closes the panel and goes to the end.** Ctrl+Enter, or the Send control, sends the prompt, collapses the panel to its one line, moves focus out of the box so it stays collapsed, scrolls the conversation to the newest activity, and re-engages follow-at-bottom so the reply streams into view. A send the service refuses changes nothing: the panel stays open with the draft intact and the error shown.
- **Answering a docked request does not disturb the panel**, as now.

## Ride-along: plain-text copy keeps its list markers

Agreed with Michael on 2026-11-10 after the copy-per-block slice. A numbered list needs its numbers, and `1.` and `-` are plain-text convention rather than markdown syntax. The implementer's notes: ordered lists count from the list's `start`; nested lists indent two spaces per level; task items copy as `[x]` and `[ ]`; update the `copy.mjs` expected text, the `docs/current` sentence that says "with no marker", and the journal's decision note.

## Checks

The browser check with the fake shim: a marked line just above the panel is at the same distance from the panel's top edge before and after opening, and after closing; a view following the bottom still follows after opening; Ctrl+Enter collapses the panel, leaves focus outside the box, and the last event is in view with follow engaged while the reply streams; a refused send keeps the panel open and the draft. The plain-text copy of ordered, nested and task lists matches the expected text.
