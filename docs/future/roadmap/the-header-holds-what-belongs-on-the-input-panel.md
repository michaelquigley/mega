---
title: the header holds what belongs on the input panel
state: evaluating
created: 2026-11-12
tags: [enhancement]
---

Move what the operator acts on or needs to notice from the conversation header to the input panel, where his eyes are. The header is at the top of the history and scrolls away with it; Michael, 2026-11-12: "items at the top of the conversation (which i generally never look at) which should be on the operator panel, and the mcp pop-open is one of those items."

## why

The header was the first place anything went. Since then the view follows the bottom, the recent tail loads first, and the controls moved to the control bar. What is left in the header is a mix: identity that is rarely consulted, facts the control bar now repeats, and state that matters at the moment of sending and is shown where nobody is looking.

## what is in the header today

- Name, label, state.
- One line of facts: harness, model, effort, harness version, runtime mode, live permission mode, the stored choice for the next launch when it differs, and what the next prompt will do.
- The MCP summary, which opens the server list.
- The environment: fingerprint, changed variable names, and the stale warning.
- Checkout path and native session id.

## shape

Settled with Michael, 2026-11-12:

- **A harness icon on the control bar.** One small mark per harness, Claude Code and Codex now and pi when it is supported, at the left of the bar beside the model, so the harness is known at a glance whether the reply box is open or collapsed. The model text beside it already says the rest. The mark comes from the harness table in `ui/src/harnesses.ts`, so a new harness brings its own.
- **The MCP list leaves the header for the More menu.** It takes no room on the control bar. The menu item carries the count and opens the server list. When a server failed or needs authorising, the item says so and the More button itself carries a small mark, so a failure is still noticed without opening anything.
- **The environment shows on the panel only when it needs attention.** Stale or failed, as a mark on the control bar, with the same words the header uses today. The fingerprint and changed names stay at the top.
- **What the next send will do shows on the control bar** when it differs from the live state: the "next launch" choice and the next-prompt note.
- **Dropped from the header.** Model, effort and permission mode, which the control bar already shows.
- **Stays at the top.** Name, label, state, harness and version in words, checkout, native id, the environment's fingerprint and changed names.

## the icons

Vendor logos, Michael's call, 2026-11-12: they read at a glance because he already knows them, and the worst case is a request to take them down. So they are held in a way that makes taking them down a small change:

- One SVG file per harness, taken from the vendor's own published brand assets, unmodified, with the source and date recorded beside them. Not redrawn from memory.
- Named only in the harness table, so removing or replacing one is a change to one file and one line.
- Used only to say which harness a conversation runs on. Never in mega's own identity: not the favicon, the README header or the About panel's heading.
- A line in the README saying the marks belong to their owners and imply no endorsement.
- A harness with no icon falls back to its name in words, which is also what a removed logo leaves behind.

## checks

The panel suite covers the control bar and gains the harness icon, collapsed and open, for both harnesses. The MCP steps in the conversation suite and the stale-environment step in the environment suite move with what they test. A failed server and a stale environment are each noticeable from the bottom of a long conversation without scrolling.
