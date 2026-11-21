---
title: context use on the input bar
state: inbox
created: 2026-11-20
tags: [feature, spike]
---

Show how full the open conversation's context window is, as a percentage on the input panel's control bar. Asked by Jens on 2026-11-20, while he reviewed the Mac keys batch.

## why

A long conversation fills its model's context window, and the harness then compacts it or refuses. The operator sees this only in the harness's own terminal view, such as Claude Code's `/context`, which mega does not show. A number on the bar tells him when to start a new conversation or hand off, before the harness decides for him.

## what exists

mega records no context use today. The workspace read carries none, and no adapter maps it.

- **Claude Code.** Each turn's `result` message carries `usage`, with its `iterations` (the last request's `input_tokens`, `cache_read_input_tokens` and `cache_creation_input_tokens`), and `modelUsage`, which gives each model's `contextWindow` (1,000,000 for `claude-opus-5-5` in the recording). See `internal/harness/claude/testdata/background.jsonl`. The Agent SDK also answers `getContextUsage({detail: 'summary'})` in about 60 ms, with no turn running (`docs/research/harness-capabilities.md`, the idle resume section). The adapter reads neither.
- **Codex.** The app-server sends `thread/tokenUsage/updated`, seen at start-up with no turn (`docs/research/harness-capabilities.md`). The adapter drops it as transport chatter (`internal/harness/codex/runtime.go`). The repository's copy of the protocol schema has no file for it, so its fields are not recorded.
- **pi.** Nothing recorded.

## shape

- **The service** keeps one value per conversation: the tokens in context, the window's size, the model they apply to, and when they were measured. The workspace read carries it. It is a reading, not workspace state the operator sets, and it is never estimated. A harness that reports nothing leaves it empty.
- **The bar** shows it as a short percentage, such as "34%", in the mono register of the bar's other values. Its title says what it counts and when: "Context: 340k of 1M tokens, at the end of the last turn". Over a threshold, perhaps 80%, it takes the notice colour. With no reading it is not shown.
- **Each adapter** maps what its harness reports, and nothing it does not.

## choices for Claude

1. **From each turn's `result`.** Context used is the last request's input, cached and cache-creation tokens. The window is `modelUsage[model].contextWindow`. No new call, and no change to the shim's protocol. It updates only at a turn's end, and it is empty for a resumed conversation until its first turn.
2. **The shim asks `getContextUsage`** after each turn and on resume. It agrees with Claude Code's own `/context`, which counts the system prompt, tools and memory files as well as the messages. It is a new shim request and reply, and one more thing to probe at each Claude Code version.

The first is enough to start with; the second can follow if the numbers disagree with `/context` enough to matter.

## spike

- **Codex:** the fields of `thread/tokenUsage/updated` at the pinned codex-cli, whether it carries the model's window or only counts, when it is sent (each turn, each request, at start-up), and what a resumed thread reports before its first turn.
- **pi:** whether pi reports context use at all over RPC, and in what form.
- **Claude:** whether the result-based number and `getContextUsage` agree, on one long conversation, and what each says after a compaction.

Record each in the capability report with its version, as new observations.

## decisions for Michael

- Whether this goes in now or waits for `simplify-the-core`, since it adds to the service and to every adapter.
- Whether it needs a section in the spec first, since it is new data mega collects.
- Which Claude source to start with, and whether Codex and pi wait for their spike or the bar shows Claude alone at first.
- The threshold for the notice colour, if any.
