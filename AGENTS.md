# Roundhouse

Roundhouse is a project-oriented workspace for working directly with a gang of heterogeneous coding agents across development environments. It is not an autonomous coordinator or a replacement coding harness. The spec is `docs/future/roundhouse.md`; the first-slice work order is `docs/future/roundhouse-work-order.md`; the harness evidence both lean on is `docs/research/harness-capabilities.md`, with the rerunnable probes under `probes/`.

## Current assignment

Implement the first slice per the work order, in its numbered stages, each gated by terminus to `clean` and then by Michael. The slice is the environment service alone on `fortyfive`, reached on localhost, with Claude and Codex adapters. Everything the spec defers stays deferred; new ideas go to the roadmap, not into a stage.

Read the most recent entries in `docs/journal/` on arrival. The investigation that produced the capability report is complete; do not reopen it unless a stage contradicts a recorded observation, in which case record the new observation with its version.

## Working rules

- Michael commits; agents do not. Leave changes in the working tree and hand off.
- Go for the service and the UI server, using `github.com/michaelquigley/df/dl` for logging and `github.com/michaelquigley/df/dd` for YAML/JSON. TypeScript only in the Claude shim and the UI.
- Preserve native permission behavior. Never weaken a harness's permissions, set trust flags on Michael's behalf, or bypass an approval to make a test pass.
- Test against disposable repositories and newly created sessions only. Do not alter, adopt, interrupt, or delete real-work sessions. The fixtures live under `/home/michael/Repos/q/research/roundhouse-fixtures/`.
- Do not upgrade harnesses, change global configuration, or expose network listeners beyond loopback without Michael's approval.
- Michael runs his own roundhouse on the default port (8470) and the default data directory. Scratch runs, browser scripts and e2e use another port and a `--data` directory under scratch, never his service or his data. Leave his processes alone.
- Keep credentials and private transcripts out of reports and committed fixtures.
- Durable project memory goes in `docs/journal/YYYY-MM-DD.md`, one file per day, appended across sessions: current state, gotchas, decisions, and what a future session would otherwise rediscover. Read recent entries before changing anything; where an entry conflicts with the code, the code wins.
- `CHANGELOG.md` follows the house convention: newest release first, prose entries led by `FEATURE:`, `CHANGE:` or `FIX:`, new entries into the `## Unreleased` slot.
- The `Makefile` is the facade: bare `make` builds, `make test` is the full ordinary gate including the UI, `make clean` removes only project-owned artifacts. Harness round trips live under `make e2e` and are run by hand.
- The roadmap is `docs/future/roadmap/`, one markdown file per item. Capture freely with `state: inbox`; never touch `.ranger/`, never delete items unasked.
- Run `unfurl -i <file>` on authored or edited markdown before handoff.

The grimoire's `AGENTS.md`, `agents/agent-roles`, and `software/conventions/` carry the broader practice when available.
