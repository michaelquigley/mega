---
title: unread is output beyond where you have read
state: researching
created: 2026-11-19
tags: [story]
---

Output counts when it is recorded, and only reading moves the reading position. Mega stops working out which output is new.

Third in the sequence of `simplify-the-core`. Piece 2, `megas-history-may-have-a-gap`, removes the resume import and its unread scan.

## why

Today output is counted when its turn ends, with exceptions for output after its turn, for a runtime that exits mid-turn, and for imports by how they happened. A marker in the registry holds the result, and start-up rebuilds it from the log when the write was lost. About seven of the hardening findings were about what counts as unread after a lost runtime or an import, and one fix, the rebuild, failed in three passes running.

Decided by Michael on 2026-11-19, and revised the same day so that an import never moves the reading position: it could pass over a reply he had not seen.

## the failure that becomes impossible

An unread mark can no longer be lost because a write failed, or miscounted by a rebuild at start-up.

## what changes

- **Removed:** the buffering of a turn's output until its end, the counting of output flagged as after its turn, and any remaining special handling for imported output when deciding what counts as unread.
- **The newest output is taken from the log.** The registry's unread marker is neither read nor written. The field is no longer needed. Whether it leaves the registry's file with this piece is settled when the piece is specified.
- **One thing the marker's write did is kept:** the workspace notification, when recorded output changes a conversation's mark or its place in the needs-you list.
- **The canon's `bookkeeping-never-creates-attention`** is amended with it.

## what it costs

An adoption snapshot with agent output shows as unread until the operator opens the conversation. Work done while mega could not see the session stays in the native transcript and contributes nothing to the mark.

## what does not change

The order of attention. A conversation with a turn in flight still reads as running. The flag for output after its turn stays for now, because it also stops late output from opening a turn; `a-turn-is-inferred-from-a-message` removes it.

## how it is shown

- agent output recorded before an unclean restart, beyond the reading position, shows unread after Resume even if mega missed the turn end;
- a conversation with an unread reply, released and resumed without being opened, is still on the needs-you list;
- a session just adopted shows as unread until it is opened;
- two unread conversations, the older then given more output, are in the order of each one's newest output;
- a conversation that is not the one on the page gains its unread mark on the rail from output alone.

## order

Depends on nothing in code. Lands on `main`. A build from before it, opened on the same workspace, counts unread its own way, so marks may differ after going back.
