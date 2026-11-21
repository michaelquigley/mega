---
title: slash commands in the reply box
state: inbox
created: 2026-11-20
tags: [feature, spike]
---

Typing `/` at the start of the reply box lists the commands the conversation's harness offers, and the list narrows as the operator types. Picking one puts it in the draft, to be sent as the prompt. Asked by Jens on 2026-11-20.

## why

Each harness has commands, its own and the operator's: Claude Code's custom commands and skills, Codex's and pi's. Today the operator has to remember their names, or look in the terminal, which mega exists to save him from. A list where he types is how every harness's own terminal offers them.

## what exists

- **Claude Code.** `system/init` carries `slash_commands` and `skills`, and is sent again at the start of every turn (`docs/research/harness-capabilities.md`, the Claude identity line). The adapter reads the model and the mode from it and nothing else. Whether the Agent SDK also gives each command a description and an argument hint is not recorded.
- **Claude Code's local commands.** mega already tells Claude's records of a local command, such as the `/model` a model switch runs, from prompts (`isCompletedLocalCommand`, `internal/harness/claude/mapping.go`). Which commands run, and what they do, when mega sends `/name args` as a prompt is not recorded.
- **Codex and pi.** Nothing recorded about commands over the app-server or RPC.
- **The reply box.** The `@` path list (`ui/src/mention.ts`, `PathList`) already opens a list above the caret, filters it as the operator types, and takes Up, Down, Enter and Escape. A `/` list is the same control with another source.

## shape

- **The service** keeps each conversation's command list as its harness last reported it, with each command's name and, where the harness gives them, its description and argument hint, and carries it in the workspace read or on request. It is a reading. A harness that reports none has none.
- **The reply box** opens the list on `/` as the first character of the draft, and only there, so a path or a sentence with a slash in it opens nothing. It filters by the name typed so far, and Enter or a press puts `/name ` in the draft, the caret after it. The operator sends it as any prompt. Escape closes the list for that word, as it does the `@` list.
- **Only commands that work through mega are listed.** One that needs the harness's terminal, such as a login or a settings screen, is left out or shown as not available here, by what the spike finds, never by a guess.

## spike

- **Claude Code:** send each kind of command as a prompt through the shim, on a disposable session: a custom command, a skill, a built-in that acts on the session (`/compact`, `/clear`), and one that needs the terminal (`/config`, `/login`). Record what each does, what the stream says, and what lands in the transcript. Also whether the SDK's own command list adds descriptions, and whether it differs from `init`'s.
- **Codex:** whether the app-server lists commands or custom prompts, and what a prompt starting with `/` does.
- **pi:** the same over RPC.

Record each in the capability report with its version, as new observations.

## decisions for Michael

- Whether this waits for `simplify-the-core`, since it adds to the service and to every adapter.
- Whether it needs a section in the spec first.
- Whether Claude ships first, from `init` alone, before Codex and pi are known.
- What a command mega cannot run should do: be left out, or be shown and refused with a reason.
