---
title: background jobs
state: evaluating
created: 2026-11-10
tags: [defect, spike]
---

Roundhouse does not show background jobs. Michael was working in a project whose agent had jobs running in the background, and the conversation looked idle. That is a false "nothing is happening": the turn had ended, so `turn_active` was false, while the harness was still doing work that would later wake it.

## What the harness already says (Claude Code 2.1.318, from recorded native events)

Claude reports background work on the stream, and the adapter records all of it as passthrough `native` events that nothing reads:

- `system/background_tasks_changed` carries the whole current list: `tasks[]`, each with `task_id`, `run_id`, `task_type` (seen: `local_bash`) and `description`. It is a snapshot, so the newest one is the truth.
- `system/task_started` (`task_id`, `task_type`, `description`, `is_backgrounded`, `tool_use_id`), `system/task_updated` (a `patch` with `status` and `end_time`), and `system/task_notification` (`status`, `summary`, `output_file`) describe one task's life. A notification arriving after the turn ended is what wakes the agent for a turn nobody prompted.

Michael's logs on 2026-11-10 hold 14 `background_tasks_changed`, 36 `task_started`, 36 `task_notification` and 7 `task_updated` events.

## Shape

- **A normalized fact, not a new protocol leak.** The Claude adapter keeps the newest task list and reports it to the service through the sink as plain data: the count and each job's description and start time. The service holds it in memory on the live record beside `turn_active`, never stored, gone when the runtime exits. The workspace read gains `background`: the list, absent when empty.
- **Attention is unchanged; activity gains a state.** Background work does not need the operator, so it never enters the needs-you list. It changes what "not in a turn" looks like:
  - an otherwise idle conversation with background jobs shows the working arc in a quieter form (the same arc, muted colour), never the idle dot;
  - a conversation whose mark is something else (unread, request) keeps that mark, and carries a small mono label after its name, `2 bg`, so the fact is visible whatever the mark says;
  - the project rollup treats background work as running for its ordering.
- **The activity line** shows it when no turn is in flight: `2 background jobs · npm test, sleep 300 · 4m`, with the muted arc. During a turn it stays what it is, with the count appended.
- **Release says what it will wait for.** The release confirmation names running background jobs, since a backgrounded tool keeps the CLI alive after input closes (recorded in the capability report).
- **Unsupervised stays unsupervised.** After an unclean restart the list is unknown, and the page says so rather than showing none.

## Probe first

Under `probes/claude/` and `probes/codex/`, recorded in the capability report with versions: whether `background_tasks_changed` is emitted on every change including the final empty list; whether a subagent appears as a task and with what `task_type`; what the stream shows when a background job finishes and wakes the agent after the turn ended; whether Codex has an equivalent (background terminals or long-running commands that outlive a turn) and what its app-server reports. If Codex reports nothing, the Codex adapter reports no background list and the card says so.

## Checks

Adapter tests from recorded frames for the list being replaced by each snapshot and cleared by an empty one; a service test that the list appears in the workspace read, clears on runtime exit, and is absent after a restart; the browser check with the fake shim (a new verb that backgrounds a job and ends the turn) for the muted arc on an idle-looking conversation, the `bg` label beside an unread mark, the activity line, and the release confirmation naming the jobs.
