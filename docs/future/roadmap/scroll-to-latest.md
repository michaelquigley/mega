---
title: Conversation view opens at the latest activity
state: evaluating
created: 2026-11-09
tags: [ui, conversation-view]
---

Switching to a conversation leaves the view at the top of its history, so every visit starts with a scroll to the bottom. Michael asked for this on the first day of daily use.

Shape: on open, once history has loaded, scroll to the newest event. While a turn streams, follow the bottom only when the view is already at the bottom; a reader who has scrolled up to re-read is not yanked down, and a small "newer activity below" affordance returns them. A pending request card is part of the newest activity and must be in view after the scroll. Switching between conversations must not inherit the previous one's scroll position.

Candidate for the first polish batch, with remove-conversation.
