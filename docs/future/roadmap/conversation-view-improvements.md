---
title: conversation view improvements
state: researching
created: 2026-11-09
tags: [enhancement]
---

Small look-and-feel items noticed during the polish batches, to pick up together on the next conversation-view pass.

- the blocks should all have a border or a background that identifies which block the output is part of
- each block should be copyable as plain text or as markdown with a single click
- the actions invoked by the agent should be indented from the left.
- the actions should have a working spinner when they're running
- the conversation view should include additional vertical whitespace between blocks
- A code block expanded while a reply is still streaming collapses again when the reply lands, because the landed message replaces the streamed one. The expanded state should survive the swap.
- In the adopt dialog the radio button sits above its session text rather than beside it; an existing style rule, not a batch regression.
