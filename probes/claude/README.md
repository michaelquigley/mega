# Claude probe

Minimal rerunnable probe for Claude Code through the Agent SDK. It launches or resumes a persisted session, drives scripted turns over streaming input, answers permission requests and `AskUserQuestion` with a declared policy, and logs every SDK message to JSONL.

## Dependencies

- Node 24 (tested with v24.12.0).
- `@anthropic-ai/claude-agent-sdk` 0.3.318, pinned in `package.json`. Install locally with `npm install` inside this directory; nothing is installed globally.
- The installed Claude Code executable. The probe defaults to `/home/michael/.local/share/claude/versions/2.1.318`; override with `--exec PATH`. The SDK package does not bundle a Linux binary in this version, so the path is required.
- The operator's existing Claude login. No credentials are read or stored by the probe.

## Usage

```
node probe.mjs run --cwd DIR --turns scenarios/x.json --log logs/x.jsonl \
  [--session-id UUID | --resume UUID] [--permission-mode default|auto|...] \
  [--approve REGEX] [--approve-delay-ms N] [--answer REGEX] \
  [--interrupt-on-permission] [--stale-after-abort] \
  [--interrupt-after-ms N [--interrupt-turn I]] [--kill-owner-after-ms N] \
  [--setting-sources user,project,local] [--exec PATH] [--timeout-ms N] \
  [--set-mode-on-permission MODE] \
  [--relay SOCK [--hold]]
node probe.mjs list --cwd DIR
node probe.mjs info --session SESSION_ID [--cwd DIR]
node probe.mjs messages --session SESSION_ID [--cwd DIR]
```

- A turns file is a JSON array of prompt strings, or of `{"prompt": ..., "set_mode": MODE}`. Each prompt is sent after the previous `result` message. When an item has `set_mode`, `setPermissionMode(MODE)` is called and its result recorded before that prompt is sent.
- `--set-mode-on-permission MODE` calls `setPermissionMode(MODE)` while the first permission request is pending, records whether that request was aborted within two seconds, then applies the policy.
- `--approve` matches `ToolName:<json input>`; matching requests are allowed after `--approve-delay-ms`, everything else is denied with a message. `AskUserQuestion` is answered with the first option whose label matches `--answer` (default: first option).
- `--permission-mode default` forces manual approvals through `canUseTool`. Without it the SDK can start in `auto` mode, where a classifier may approve calls before they reach the host.
- `--relay SOCK` turns the probe into a reconnect-test owner (see `../common/README.md`): every SDK message is published to connected clients, permission and `AskUserQuestion` requests wait for a client answer (`{"decision":"allow"|"deny"}` or `{"answers":{...}}`), clients can submit prompts, and `--hold` keeps the session open after the turns file until a client sends `close`. Keep the socket path short (`/run/user/<uid>/rh-claude.sock`); long paths fail with `listen EINVAL`.
- Progress goes to stderr; a JSON summary (session id, init configuration, requests, results) goes to stdout; the full message log goes to `--log`.
- `logs/` is gitignored. Logs contain full model output for the fixture sessions only.

## Scenarios used in the investigation

| File | Purpose |
| --- | --- |
| `scenarios/rt1-managed.json` | Managed launch: marker, allowed command, `AskUserQuestion`, real permission request |
| `scenarios/rt1-sdk-return.json` | Re-entry after the native terminal turn |
| `scenarios/adopt-native.json` | Adoption of a session first created in the terminal |
| `scenarios/int-active2.json` | Interrupt an active tool turn (`--interrupt-after-ms 15000`) |
| `scenarios/int-permission.json` | Interrupt while a permission request is pending (`--interrupt-on-permission`, optionally `--stale-after-abort`) |
| `scenarios/owner-kill.json` | Kill the SDK host while a tool runs (`--kill-owner-after-ms 10000`) |
| `scenarios/hook-check.json`, `scenarios/marker-check.json` | Which project hooks and instructions load under different `--setting-sources` |
| `scenarios/auto-mode.json` | Auto mode (`--permission-mode auto`): a read, a project `ask` rule, a write outside the checkout |
| `scenarios/auto-switch.json` | `set_mode: auto` between turns, from `--permission-mode default` |
| `scenarios/auto-switch-pending.json` | `--set-mode-on-permission auto` while a request is pending, then a second call in the same turn |
| `scenarios/mode-omitted.json` | No `--permission-mode`: what Claude starts in |

Fixture repository used: `/home/michael/Repos/q/research/roundhouse-fixtures/claude-rt1` (disposable; `CLAUDE.md` with marker `ROUNDHOUSE-MARKER-7f3a`, `fixture.txt`, `.claude/settings.json` with allow/ask rules and two logging hooks).

## Follow-up scripts (2026-11-06)

| Script | Purpose |
| --- | --- |
| `run-reconnect.sh` | Section 1 of the follow-up brief: client drop during work, pending question across reconnect, request interrupted while away, late answer, follow-up turn |
| `run-suspend.sh` | Section 2: idle stop and reopen, stop with a permission pending, active command with interrupt vs wait (`ONLY=active` runs just the last two) |
| `../common/run-grouped.sh` | Grouped suspension of one Claude session and one Codex thread from a static manifest |

## Model and effort (2026-11-10)

`models.mjs --cwd DIR --log logs/x.jsonl [--model M] [--effort E] [--idle-only]` starts a session with no prompt and times `supportedModels()` and `initializationResult()`. It then runs three marker turns: as launched, after `setModel('sonnet')`, and after `applyFlagSettings({effortLevel: 'low'})`. For each it records `system/init`'s model and effort, and any message that mentions effort. `--idle-only` instead closes without a prompt under a fresh session id, and reports whether a transcript or a `listSessions` entry was left.

## Background jobs (2026-11-10)

`background.mjs --cwd DIR --log logs/x.jsonl [--scenario bash|two|agent] [--linger SECONDS]` sends one prompt that backgrounds work. It answers tool permission only for the scenario's exact commands, then keeps reading after the turn's result for `--linger` seconds (90 by default) before closing input and timing the exit. `bash` backgrounds one Bash call, `two` backgrounds two at once, and `agent` backgrounds a subagent. A short `--linger` closes input while the job still runs.

## `@path` mentions (2026-11-10)

`mention.mjs --cwd DIR --log logs/x.jsonl [--plain]` sends one prompt asking for the exact contents of `@fixture.txt`, with the file tools disallowed and every permission denied. It records whether the reply carries the file's content and whether any message before the reply does. `--plain` names the file without the `@`, as a control. `--file NAME` mentions another file, and `--form bare|quoted|escaped` writes the mention as `@name`, `@"name"` or with each space backslash-escaped, for a name with spaces.

## Idle resume (2026-11-10)

`idle-resume.mjs --cwd DIR --log logs/x.jsonl [--hold SECONDS] [--session UUID] [--mcp SCRIPT] [--then-prompt] [--facts] [--set-mode MODE] [--hook] [--mode MODE]` seeds a fresh session with one short turn, unless `--session` names one. It then resumes the session with streaming input and sends nothing for `--hold` seconds (300 by default). It records every message, `initializationResult()`, `mcpServerStatus()`, the transcript's size and mtime, and the memory of every process below it. At the end it closes input and times the exit.
- `--mcp` passes a stdio MCP server for this query only.
- `--facts` asks `getContextUsage({detail: 'summary'})` for the model.
- `--set-mode` switches the permission mode while idle.
- `--hook` registers an observing `SessionStart` callback.
- `--mode` passes a launch mode.
- `--then-prompt` sends one prompt before closing.

## Moved store (2026-11-11)

`moved-store.mjs --cwd NEWPATH --session UUID --log logs/x.jsonl` checks a session after its checkout was copied to `NEWPATH` and a copy of its store directory was moved to the name `NEWPATH` maps to; the caller does both. It reads the session back through `listSessions` and `getSessionMessages` from `NEWPATH`, then resumes it by id with one prompt asking for its earlier replies. It records the answering session, the reply, and where the transcript grew.
