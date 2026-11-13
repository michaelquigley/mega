---
title: the checks run in a browser michael does not use
state: inbox
created: 2026-11-12
tags: [enhancement]
---

Run the browser suites under Firefox as well as Chromium. Michael's daily browser is Zen, which is Firefox underneath, and the suites launch only Chromium (`checks/lib/mega.mjs`). Twice now a defect lived in that difference.

## why

- **Alt+N, stage 4.** The shortcut never reached the page in Zen. No check could have said so.
- **The line under the dock, 2026-11-12.** After a send, the newest line slid under the input panel in Zen. On the unfixed tree the check passed in Chromium, whose scroll anchoring happened to keep the view pinned. It failed only once the check turned anchoring off by hand with `.main { overflow-anchor: none }`.

The second is the telling one. The check was right and the product was wrong, and the browser under test hid it. Turning one browser behaviour off by hand works when somebody already knows which behaviour matters. Running the browser he uses needs no such knowledge.

## shape to explore

- Playwright drives a Firefox build of its own; the pinned version in `checks/package.json` decides which. `make checks` runs each suite under both, or a second target does, depending on what it costs in time.
- Start with the suites where layout and scrolling carry the meaning: panel and conversation.
- Playwright's Firefox is not Zen. It is the same engine, which is where both defects lived, and it will not catch what Zen's own chrome does with a key. Shortcuts still want Michael's confirmation.
- Whether the hand-written `overflow-anchor: none` in the panel suite stays once Firefox runs: it should if the step still fails on an unfixed tree only with it.

## decided alongside

The product does not set `overflow-anchor: none`. Anchoring keeps a reader's place wherever mega does not manage scrolling itself: rows appearing above the view when native events are switched on, an action group above the view opening or closing, content settling as it renders. Turning it off to make browsers agree would make each behave like the least helpful one.
