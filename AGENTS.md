# mega

mega is a project-oriented workspace for working directly with a gang of heterogeneous coding agents across development environments. It is not an autonomous coordinator or a replacement coding harness. The spec is `docs/future/mega.md`; the system as built is `docs/current/mega.md`; the harness evidence both lean on is `docs/research/harness-capabilities.md`, with the rerunnable probes under `probes/`. The first-slice work order was retired when its last stage gated; its decisions live in `docs/current/`, and git history has the document.

## Current assignment

The first slice is built: the environment service alone on `fortyfive`, reached on localhost, with Claude and Codex adapters and attention. Michael is now using it daily, and work proceeds in polish rounds driven by what he finds: batches of related items on one surface, each reviewed by terminus to `clean` against the canon rubric before handoff, with the same stop-and-ask rule for anything that needs a decision. Look-and-feel items go straight into a batch; a discovery that changes what mega should do goes to the spec first. Everything the spec defers stays deferred; new ideas go to the roadmap, not into a batch.

Read the most recent entries in `docs/journal/` on arrival. The investigation that produced the capability report is complete; do not reopen it unless a change contradicts a recorded observation, in which case record the new observation with its version.

## Working rules

- Michael commits; agents do not. Leave changes in the working tree and hand off.
- Go for the service and the UI server, using `github.com/michaelquigley/df/dl` for logging and `github.com/michaelquigley/df/dd` for YAML/JSON. TypeScript only in the Claude shim and the UI.
- Preserve native permission behavior. Never weaken a harness's permissions, set trust flags on Michael's behalf, or bypass an approval to make a test pass.
- Test against disposable repositories and newly created sessions only. Do not alter, adopt, interrupt, or delete real-work sessions. The fixtures live under `/home/michael/Repos/q/research/roundhouse-fixtures/`.
- Do not upgrade harnesses, change global configuration, or expose network listeners beyond loopback without Michael's approval.
- Michael runs his own mega on the default port (8470) and the default data directory. Scratch runs, browser scripts and e2e use another port and a `--data` directory under scratch, never his service or his data. Leave his processes alone.
- A scratch run also needs its own `XDG_STATE_HOME`, because mega writes its log file there whenever it is not attached to a terminal, and a scratch `mega daemon` needs its own `XDG_CONFIG_HOME` for the tray runtime file and `XDG_DATA_HOME` for anything `mega desktop` writes. Never run `mega desktop integrate` or `remove` against Michael's real desktop; installing the launcher is his.
- The e2e round trips take their fixtures from the environment and skip when unset. Michael's values: `MEGA_E2E_CLAUDE_FIXTURE=/home/michael/Repos/q/research/roundhouse-fixtures/claude-rt1` and `MEGA_E2E_CODEX_FIXTURE=/home/michael/Repos/q/research/roundhouse-fixtures/codex-rt1`.
- Keep credentials and private transcripts out of reports and committed fixtures.
- Durable project memory goes in `docs/journal/YYYY-MM-DD.md`, one file per day, appended across sessions: current state, gotchas, decisions, and what a future session would otherwise rediscover. Read recent entries before changing anything; where an entry conflicts with the code, the code wins.
- `CHANGELOG.md` follows the house convention: newest release first, prose entries led by `FEATURE:`, `CHANGE:` or `FIX:`, new entries into the `## Unreleased` slot.
- The `Makefile` is the facade: bare `make` builds, `make test` is the full ordinary gate including the UI, `make clean` removes only project-owned artifacts. Harness round trips live under `make e2e` and are run by hand.
- The roadmap is `docs/future/roadmap/`, one markdown file per item. Capture freely with `state: inbox`; never touch `.ranger/`, never delete items unasked.
- Run `unfurl -i <file>` on authored or edited markdown before handoff.

The grimoire's `AGENTS.md`, `agents/agent-roles`, and `software/conventions/` carry the broader practice when available.
