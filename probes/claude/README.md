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
  [--relay SOCK [--hold]]
node probe.mjs list --cwd DIR
node probe.mjs info --session SESSION_ID [--cwd DIR]
node probe.mjs messages --session SESSION_ID [--cwd DIR]
```

- A turns file is a JSON array of prompt strings. Each prompt is sent after the previous `result` message.
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

Fixture repository used: `/home/michael/Repos/q/research/roundhouse-fixtures/claude-rt1` (disposable; `CLAUDE.md` with marker `ROUNDHOUSE-MARKER-7f3a`, `fixture.txt`, `.claude/settings.json` with allow/ask rules and two logging hooks).

## Follow-up scripts (2026-11-06)

| Script | Purpose |
| --- | --- |
| `run-reconnect.sh` | Section 1 of the follow-up brief: client drop during work, pending question across reconnect, request interrupted while away, late answer, follow-up turn |
| `run-suspend.sh` | Section 2: idle stop and reopen, stop with a permission pending, active command with interrupt vs wait (`ONLY=active` runs just the last two) |
| `../common/run-grouped.sh` | Grouped suspension of one Claude session and one Codex thread from a static manifest |
