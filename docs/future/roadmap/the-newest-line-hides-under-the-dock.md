---
title: the newest line hides under the dock
state: building
created: 2026-11-12
tags: [defect]
---

After sending a prompt, the bottom of the conversation slides under the input panel by one line and stays there until the agent's first output arrives. Keep the view pinned to the newest content whenever the dock at the bottom changes height, while the view is following the bottom.

## why

Michael's screenshot, 2026-11-12: his prompt block cut off at its last line, the activity line "Working · 9s" directly beneath it. The order of events on Ctrl+Enter is the cause. The prompt is recorded and the page scrolls to the bottom while the conversation is still idle. Then the service reports the turn started and the activity line appears in the dock above the reply box, making the dock one line taller. The history's visible area shrinks by that line from the bottom, and nothing scrolls, because `ui/src/latest.ts` re-pins only when new content arrives (an event, streamed text, a request). The appearance of the activity line is a change in the dock's height, which nothing observes. The first output re-pins it; a wheel scroll does too.

## shape

- While the view is following the bottom, any change in the dock's height keeps it pinned: observe the dock's size, or the scroll area's visible height, and re-pin on change. A view the operator has scrolled away from the bottom is left alone.
- This replaces special cases rather than adding one. The same rule covers a docked request appearing or leaving, the activity line changing height, and the reply box growing with its content; the existing handling for the panel opening and closing should fold into it where it can without changing the "keep my place" behaviour when not at the bottom.

## checks

In the panel suite: with the fake shim holding its reply, send a prompt from the bottom of a conversation and assert, before any output arrives, that the prompt block's bottom edge is above the dock's top edge. The same assertion when a docked request appears while following. A view scrolled up is not moved by either.
