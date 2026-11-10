---
title: typography follow-ups
state: inbox
created: 2026-11-10
tags: [ui, typography]
---

Decided by Michael after the typography batch gated clean (round `e935caf6bac5`). Not worth reopening a clean tree; they go first in whichever batch follows, in this order.

- **Latin-only serif.** Import only the Latin subsets of `@fontsource-variable/source-serif-4` (normal and italic, optical size) instead of `opsz.css` and `opsz-italic.css`. That cuts the embedded serif from about 900 KB to about 250 KB; non-Latin text falls back to Georgia. The browser already fetches only the two Latin files, so nothing changes on screen for Latin text.
- **Wider conversation.** Raise the conversation's maximum width from 860px to 960px (`.conversation` in `ui/src/index.css`); the sidebar stays at 280px. Code blocks and tables gain room, and prose lines run a little longer.

Settled with them, no change needed: buttons stay in the serif.
