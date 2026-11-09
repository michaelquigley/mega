---
title: auto mode in claude code
state: researching
created: 2026-11-09
tags: [permissions, claude, conversation-view]
---

In the terminal Michael runs Claude Code in auto mode, where a classifier approves routine operations and only the residue is asked. In roundhouse every operation asks, because the shim passes `permissionMode: 'default'` explicitly. Daily use makes that the loudest cost in the workspace.

## What the pinned SDK offers (0.3.318, checked 2026-11-09)

- `permissionMode` accepts `default`, `auto`, `acceptEdits`, `plan`, `dontAsk` and `bypassPermissions`. In `auto` a model classifier approves or denies each call and prompts through `canUseTool` when it cannot decide, so request cards keep working for the residue.
- When the option is omitted, Claude Code picks the starting mode as `claude -p` does: `permissions.defaultMode` from the settings the session loads, else `auto` where available. Michael's settings set no `defaultMode`; he chooses auto per terminal session.
- `setPermissionMode(mode)` changes the mode of a running session in streaming input mode. A per-MCP-server override exists but only tightens, and is not needed here.
- `system/init` already reports `permissionMode`, and the conversation header already shows it.
- Nothing about auto mode has been probed: the capability report ran everything in `default`.

## Decision (spec, "How it is built", adapters)

The permission mode is the operator's per-conversation choice, as in the terminal. Roundhouse never selects a mode on its own and never offers `bypassPermissions`. Its own default is `default` unless the operator's native settings name another mode, in which case that is inherited, as the spec's "native settings are inherited rather than silently stripped" already requires. The canon's `permissions-never-weakened` is amended to say so: what it forbids is a mode roundhouse chose, the bypass mode, and narrowed setting sources, not a mode the operator chose for that conversation.

## Shape

- **Probe first**, under `probes/claude/`, recorded in the capability report with versions: launch with `permissionMode: 'auto'` against `claude-rt1` and run the approval scenario; observe whether the classifier runs under the SDK in stream-json mode, which calls reach `canUseTool`, what `system/init` and the transcript record, and whether `setPermissionMode('auto')` mid-session takes effect on the next tool call. Note what "where available" means if auto turns out to be unavailable headless.
- **Shim**: a `--permission-mode` launch argument, omitted to inherit the settings default, and a `set_permission_mode` frame from service to shim that calls the SDK switch and reports the result. Still no policy in the shim.
- **Adapter and service**: the mode is part of the launch spec and recorded on the conversation; a change is a `runtime` event carrying the new mode, recorded before memory moves, like every transition. The header shows the live mode from `system/init`, which is the harness's own report, and shows "inherited" when roundhouse passed nothing.
- **UI**: the launch dialog offers the modes Claude offers minus bypass, defaulting to `default`; the conversation header lets the mode be changed while the runtime is up, with the change applying from the next tool call. A request card raised in auto mode says that the classifier could not decide it.
- **Codex** has its own approval policies (`on-request`, `never`, `untrusted`) and is a separate card once this one lands, since its policy comes from `config.toml` and `thread/start` can carry one.

## Checks

Adapter tests for the launch argument and the switch frame; a service test that a mode change is recorded before it shows; the browser check with the fake shim for the dialog and the header; `make e2e` extended with one auto-mode turn that proves the residue still arrives as a card.

## Not in this card

`bypassPermissions` in any form; `dontAsk`; per-MCP-server overrides; Codex approval policies; remembering a preferred mode across conversations (a later preference, once the per-conversation choice has been lived with).
