---
title: Meta user messages are not prompts
state: evaluating
created: 2026-11-10
tags: [claude, adapter, import, fix]
---

After a restart, a resume import showed a block labelled "You, outside Roundhouse" that Michael never wrote: a web-fetch subagent's hand-back report. Seen 2026-11-10 in his implementer conversation.

What happens (Claude Code 2.1.318, SDK 0.3.318): a subagent hand-back reaches the main session as a queued command. The transcript records it as a `queue-operation` (enqueue, then remove with reason `absorbed_mid_turn` and a `commandUuid`) plus an `attachment` of type `queued_command`; it is never streamed live as a user message. `getSessionMessages` synthesizes it as an SDK user message with `is_meta: true`, `isQueuedCommand: true`, string content beginning `<agent-message from=…>`, and an `origin` block (`kind: peer`, `handback: true`, `name: web-fetch`, `from`, `senderTaskId`, the report text). `mapMessage` in `internal/harness/claude/mapping.go` maps any user message with text to a `user` event, so the import records it as an operator prompt with origin native. Live and import disagree because live never saw it, so it has no identity in the log to match.

Across all logs on 2026-11-10: 227 native-origin user events without flags, 2 with `isQueuedCommand` alone (prompts typed ahead while a turn ran, genuinely the operator's), 2 with `is_meta` (both this case).

Fix:

- In the Claude mapping, a user message with `is_meta: true` is a `native` event, not a `user` event, named from its origin when present (`meta: hand-back from web-fetch`), otherwise `user/meta`. Same `NativeID`, so import stays idempotent and the event stays visible under the native toggle. `isQueuedCommand` alone stays a prompt.
- Native events already never count as output, so attention is unaffected.
- Record the observation in the capability report with the version: the history reader synthesizes meta and queued user messages the live stream never delivers.
- Tests: mapping test with the captured shape (meta plus origin) yielding a native event; a queued command without meta still yielding a user event; an import idempotence check that a meta message imported twice is recorded once.
- History already recorded is not rewritten; the two existing blocks stay as they are unless Michael asks for a one-off correction.

Also take the Latin-only serif import and the "cannot answer" quoting advisory with this batch, since both are one-line changes waiting for a tree to ride on.
