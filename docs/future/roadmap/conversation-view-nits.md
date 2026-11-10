---
title: Conversation view nits
state: researching
created: 2026-11-09
tags: [ui, conversation-view]
---

Small look-and-feel items noticed during the polish batches, to pick up together on the next conversation-view pass.

- A code block expanded while a reply is still streaming collapses again when the reply lands, because the landed message replaces the streamed one. The expanded state should survive the swap.
- In the adopt dialog the radio button sits above its session text rather than beside it; an existing style rule, not a batch regression.
