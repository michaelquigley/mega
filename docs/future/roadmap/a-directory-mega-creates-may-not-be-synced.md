---
title: a directory mega creates may not be synced
state: horizon
created: 2026-11-24
tags: [defect]
---

A directory mega creates under the data directory can be left in place with its own name never synced. `durable.MkdirAll` (`internal/durable/durable.go`) syncs the parent of each directory it creates, and only of the ones it creates. If the process ends after a directory is made and before that sync succeeds, the directory is still there at the next start. `MkdirAll` then finds it existing and never syncs its parent. Both writers of the record go through it: a log's creation (`internal/events/log.go`, for `conversations/<id>/`, and `conversations/` itself for the first conversation) and the registry's write (`internal/registry/registry.go`, for the data directory).

Found by Terminus pass `6f6059a940a4` on `a-failed-write-stops-mega`, as `first-append-misses-unsynced-directory-entry`. It is older than that card: the old `Open` made the log's directory through the same `MkdirAll`. Michael carded it on 2026-11-24 rather than fixing it there.

## what it takes to lose anything

Two things, one after the other.

1. **A failed sync or a crash at the instant of creation:** after `MkdirAll` has made the directory and before it has synced the parent naming it. A failed sync ends mega since `a-failed-write-stops-mega`.
2. **A power loss within the kernel's own writeback,** before the kernel writes the directory entry out on its own. Nothing mega does later syncs it, since every later write is inside the directory.

A process crash alone loses nothing, and neither does a power loss once the kernel has written the entry back.

## what is at risk

Mega's log of one new conversation: `conversations/<id>/events.jsonl` with every event in it, confirmed ones included. Its native session stays with the harness and can be adopted again. Losing the data directory's entry would take the registry too, but that directory is made once, at the first write of a new workspace.

## what is already done

Since `a-failed-write-stops-mega`, a log syncs the directory that names it before its first event, whether it created the file or found it empty. So a file left empty by a failed creation is covered. One level up is not.

## where a fix stops

At the data directory, not the filesystem root. The service would give the stores the data directory, and on a log's first event, or the registry's first write in a run, each directory from the one in question up to the data directory would be synced, whether or not this run created it. What stands above the data directory is the operator's, made before mega ran.
