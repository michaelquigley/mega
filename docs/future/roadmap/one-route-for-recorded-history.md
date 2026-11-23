---
title: one route for recorded history
state: researching
created: 2026-11-19
tags: [story]
---

A conversation's recorded events reach the page one way. The stream says how far a conversation has got and carries the text still being written; the events themselves come from the history read. The page has one job on open, on a notice and on a reconnect: read on from the last event it has, until it reaches the number the stream named.

Seventh in the sequence of `simplify-the-core`. Michael, 2026-11-19: "this sounds like a smart simplification."

## why

From the same independent review as `reading-the-workspace-starts-nothing`, 2026-11-19. The planning agent checked the defect it names and it is real.

Today the selected conversation gets the same recorded events twice over: by history reads and on the stream. `ui/src/log.ts` merges and sorts them, remembers numbers that arrived beyond a gap, and accounts for hidden native events while working out how far the operator has read.

**A defect that follows from it.** In `ui/src/App.tsx`, `catchUp` is the read a reconnect makes for what the page missed. When it fails it sets the page's error and nothing tries it again on that connection; the retry loop that then runs reads the workspace, not the history. Later events arrive on the stream and the missing stretch stays missing. The opening read of a conversation has a retry of its own. This one does not.

With one route the failure has a natural answer: the job to read on is still outstanding, so it is tried again.

## what it costs

A local read for each burst of finished output, where today the event arrives on the stream. Bursts should fold into one read. Over a private share from another machine the extra trip is longer. The text being written still streams, so the page stays as lively.

## the failure that becomes impossible

A failed read of history can no longer leave a gap nothing fills, and a recorded event can no longer reach the page by two routes that have to be reconciled.

## order

Depends on nothing. It comes before `the-page-has-one-value-for-where-it-stands`, which is about what the page knows of the workspace and is easier once a conversation's events have one route. Lands on `main`. To settle when it is specified: what the stream's frame for "reached N" looks like.

The unretried `catchUp` is fixed by this and not before it. Michael, 2026-11-19, on whether to fix it on its own first: breaking all of it into chunks is the answer.
