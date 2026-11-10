---
title: model and effort selection
state: evaluating
created: 2026-11-10
tags: [conversation-view, claude, codex]
---

Launching picks a harness; it also needs to pick the model and the effort level, and a running conversation needs to be able to change both, the way the terminals allow. Michael asked for this on 2026-11-10.

## What the harnesses offer (checked 2026-11-10)

**Claude, SDK 0.3.318.** `query` options take `model` and `effort` (`low`, `medium`, `high`, `xhigh`, `max`). At runtime `setModel(model?)` changes the model for later responses and `applyFlagSettings({effortLevel})` changes the effort (`null` returns to the model's default; `max` is session-only and runs as `high` on a model without it). `supportedModels()` returns `ModelInfo[]` with `value`, `displayName`, `description`, `supportsEffort` and `supportedEffortLevels`, but it is a method of a running query. `system/init` reports the live `model` at every turn start; it does not report effort (hooks and the status line receive it, the SDK stream does not). Michael's user settings set `model: opus` and `effortLevel: xhigh` with per-model overrides, so "inherit" is a real and common choice.

**Codex, codex-cli 0.166.0.** `thread/start` and `thread/resume` take `model`; `turn/start` takes `model` and `effort`, each documented as "override for this turn and subsequent turns". `model/list` (params `includeHidden`, paging) returns `Model[]` with `id`, `displayName`, `description`, `isDefault`, `hidden`, `defaultReasoningEffort` and `supportedReasoningEfforts` (each with a description), and works from a short-lived app-server with no thread, as discovery does. The thread response reports `model`. Michael's `config.toml` sets `model = gpt-6-astra` and `model_reasoning_effort = high`.

## Shape

- **Inherit is the default and a first-class choice.** With no choice, roundhouse passes nothing and the harness applies the operator's own settings, exactly as the permission mode works. The dialog says what inherit currently means where it can be known cheaply (Codex's `isDefault` model and its `defaultReasoningEffort`; Claude's `settings.model` and `effortLevel` from the user settings file, read-only).
- **Launch dialog.** After the harness, a model picker and an effort picker. The lists come from the harness: Codex from `model/list`; Claude from `supportedModels()` if the probe shows it answers on an idle session, else from the alias list (`opus`, `sonnet`, `haiku`) plus a free-text id, with the settings default preselected. The effort picker is constrained to what the chosen model supports and is disabled for a model that supports none. Lists are cached per harness version for the service's lifetime.
- **Stored choice, passed at every launch.** Model and effort are recorded on the conversation beside the permission-mode choice and passed on every launch of it: Claude as `query` options; Codex as `model` on `thread/start` or `thread/resume` and `model` plus `effort` on every `turn/start`, since the override is per turn and re-sending the stored choice is the robust way to make it persistent.
- **Changing while running.** The same header pattern as the permission mode. Claude: `setModel` and `applyFlagSettings`, effective from the next response. Codex: the stored choice changes and the next `turn/start` carries it; nothing is sent until then, and the header says "from the next prompt". With no runtime, the control sets the stored choice only.
- **Honest header.** One writer of the live model: the harness's report (`system/init` each turn for Claude, the thread response and turn events for Codex). Effort is shown as what was requested, labelled as such, because neither harness reports the effective level on the stream the adapter reads; if the probe finds a report, use it. A stored choice that differs from the live report shows as "next prompt: X", as the mode control does.
- **Never offered.** `fallbackModel`, hidden models unless the operator asks, thinking budgets. Fast mode and ultracode are flag settings with their own semantics and are not in this card.

## Probe first

Under `probes/claude/` and `probes/codex/`, recorded in the capability report with versions: whether `supportedModels()` answers on an idle session before the first prompt (and how long the idle start costs); whether `setModel` is reflected in the next `system/init`; whether `applyFlagSettings({effortLevel})` is accepted on 2.1.318 and whether any stream message reflects the effort; Codex `model/list` output shape against the live account (ids, defaults, efforts); whether a `turn/start` `effort` override is visible in `turn/started` or the thread response.

## Checks

Adapter tests for the launch options and the switch frames; a service test that a choice is stored before it is passed and that a change on a running Codex conversation is applied on the next turn only; the browser check with the fake shim and fake Codex for both pickers, the inherit default, the constrained effort list, and the header's requested-versus-reported display; `make e2e` gains one Claude turn and one Codex turn launched with an explicit model and effort, asserting the harness's own report of the model.
