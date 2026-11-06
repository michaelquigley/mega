# Codex app-server probe

Minimal, dependency-free Node scripts that drive the installed `codex app-server` (codex-cli 0.166.0) over stdio JSON-RPC for the Roundhouse harness investigation.

## Requirements

- Node 24 (tested with v24.12.0) and `codex` on `PATH` (tested: `/home/michael/local/node/bin/codex`, codex-cli 0.166.0).
- The operator's existing Codex login. Nothing here changes `~/.codex/config.toml` or auth.
- Does not use or touch the shared app-server daemon (`codex app-server daemon ...`, `codex agents`).

## Files

- `probe.mjs`: spawns its own `codex app-server` child over stdio, runs scripted turns, answers approval and user-input server requests by policy, logs every frame to JSONL.
- `probe-ws.mjs`: two-client test against one `codex app-server --listen ws://127.0.0.1:PORT`. Written, not run: the session's permission classifier denied starting a loopback listener. Run only with the operator's approval.
- `scenarios/*.json`: turn lists used by the recorded runs.
- `logs/`: gitignored. Each run writes `<name>.jsonl` (all frames), `<name>.stderr.txt` (probe narration), `<name>.summary.json`.

## Usage

```
cd probes/codex
node probe.mjs run --cwd <fixture-dir> --turns scenarios/rt1-managed.json \
  --log logs/rt1-managed.jsonl --approve 'approval-probe' --approve-delay-ms 4000 --answer '^Blue$'
node probe.mjs run --cwd <fixture-dir> --resume <thread-id> --turns scenarios/rt1-resume.json --log logs/x.jsonl --approve 'approval-probe'
node probe.mjs list --cwd <fixture-dir>
node probe.mjs read --thread <thread-id>
```

Flags: `--approve REGEX` (accept matching command approvals, else decline), `--approve-delay-ms N`, `--answer REGEX` (option label for `item/tool/requestUserInput`), `--interrupt-after-ms N [--interrupt-turn I]`, `--interrupt-on-approval`, `--stale-after-interrupt`, `--kill-owner-after-ms N`, `--kill-server-after-ms N`, `--approval-policy`, `--sandbox`, `--timeout-ms`.

## Protocol source

Shapes were taken from the installed binary, not from memory:

```
codex app-server generate-json-schema --out <dir>
codex app-server generate-ts --out <dir>
```

See `NOTES.md` for results.
