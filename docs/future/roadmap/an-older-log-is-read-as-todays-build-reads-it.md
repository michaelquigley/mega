---
title: an older log is read as today's build reads it
state: researching
created: 2026-11-24
tags: [story]
---

Remove the rules whose only subject is something an earlier build wrote. An older log opens and shows its history, and its lines are read with today's meaning.

A removal from the review of `simplify-the-core`, ruling 2 in `docs/future/simplify-the-core-inventory.md`. Michael, 2026-11-23: "yes, i think so."

## what goes

- **The one-time fill of a conversation's choices,** in `internal/service/choice.go`, with its `choice_filled` status and its row on the page. It completed conversations created before 2026-11-13.
- **The replay of old import markers,** `legacyImport` in `internal/service/live.go`, added so a failed import under an older build does not show as a current error.
- **The `mode_confirmed` rows** on the page, and the `ModeConfirmed` field nothing records any more.

A line of one of these kinds in an older log still shows, as any line of its kind does.

## what it costs

A mark that is wrong once on a rare old conversation. Safety is not relaxed: where an older log leaves it unclear whether a runtime is alive, the conversation is Unsupervised.

## before it is built

- **`adoption-reaches-any-session-in-the-checkout` comes first.** The fallback for a conversation an upgrade misreads is to remove it and adopt its session again, and today adoption cannot reach a Codex or pi session outside the 50 newest in its checkout.
- **The fill has to have run everywhere it is needed.** It has nothing left to do on `fortyfive`. Michael's other two machines may still hold conversations from before 2026-11-13, and each needs one start on a build that has the fill.

## when it lapses

This rule holds until the first tagged release. `first-tagged-release` carries the reminder.
