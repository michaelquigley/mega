# mega

mega is a project-oriented workspace for working directly with a gang of heterogeneous coding agents across development environments. It is not an autonomous coordinator or a replacement coding harness. The spec is `docs/future/mega.md`; the system as built is `docs/current/mega.md`; the harness evidence both lean on is `docs/research/harness-capabilities.md`, with the rerunnable probes under `probes/`. The first-slice work order was retired when its last stage gated; its decisions live in `docs/current/`, and git history has the document.

## Current assignment

The first slice is built: the environment service alone on `fortyfive`, reached on localhost, with Claude and Codex adapters and attention, and pi, the third harness, built on 2026-11-16. Michael is now using it daily, and work proceeds in polish rounds driven by what he finds: batches of related items on one surface, each reviewed by terminus to `clean` against the canon rubric before handoff, with the same stop-and-ask rule for anything that needs a decision. Look-and-feel items go straight into a batch; a discovery that changes what mega should do goes to the spec first. Everything the spec defers stays deferred; new ideas go to the roadmap, not into a batch.

From 2026-11-13 the work in front was the epic for an external operator installing mega: anyone other than Michael running their own mega. Its card was closed on 2026-11-18, and at least two external operators now run it. One deferral was lifted by his decision of 2026-11-13: pi came into scope, as the card `pi-adapter` describes, and its adapter is built (`docs/current/mega.md`, "The pi adapter").

Since 2026-11-19 the work in front is the epic `simplify-the-core` in the roadmap: a sequence of small pieces, one card each, that make the core smaller so that whole kinds of fault have nowhere to live. They are built one at a time, in the order the epic lists. Each names a failure it makes impossible and shows it. What a card does not say changes, does not: the code and its tests are the specification of the rest. The full-tree hardening review on `harden-requests-stops-and-restarts` is paused until the sequence is done.

On 2026-11-23, two pieces in, the core was no smaller, and Michael asked for a review of the model, of what had landed, and of the plan. Its rulings are in `docs/future/simplify-the-core-inventory.md` and come before the epic's order, so read that document before taking a card. A removal the review finds is a card of its own, listed on the epic and built before the next numbered piece.

Read the most recent entries in `docs/journal/` on arrival. The investigation that produced the capability report is complete; do not reopen it unless a change contradicts a recorded observation, in which case record the new observation with its version.

## Working rules

- Michael commits; agents do not. Leave changes in the working tree and hand off.
- Go for the service and the UI server, using `github.com/michaelquigley/df/dl` for logging and `github.com/michaelquigley/df/dd` for YAML/JSON. TypeScript only in the Claude shim and the UI.
- Preserve native permission behavior. Never weaken a harness's permissions, set trust flags on Michael's behalf, or bypass an approval to make a test pass.
- Test against disposable repositories and newly created sessions only. Do not alter, adopt, interrupt, or delete real-work sessions. The fixtures live under `/home/michael/Repos/q/research/roundhouse-fixtures/`.
- Do not upgrade harnesses, change global configuration, or expose network listeners beyond loopback without Michael's approval.
- Michael runs his own mega on the default port and the default data directory: 6342 since 2026-11-18, and 8470 on any of his megas not yet restarted on a build with that default. Scratch runs, browser scripts and e2e use another port and a `--data` directory under scratch, never his service or his data. Leave his processes alone.
- A browser check is a test, not a spike. Write it into the repository's `checks/` directory and leave it there; only its data (generated conversations, screenshots, logs) goes in scratch. Scratch directories live under `/tmp`, which is erased at reboot, and a reboot on 2026-11-12 took every check written before that day. A batch that changes a surface of the interface first makes sure that surface has a suite in `checks/` and leaves it passing; `checks/README.md` says what each suite covers.
- Clean up scratch when a batch or a spike is done: remove the directories you made under `/tmp` and `/dev/shm`, and say in the report that you did. Both are memory-backed and space is tight; `/dev/shm` is RAM. Keep only what a later step needs, and say what and why. Anything rerunnable, a driver, a probe, a script that produced evidence, goes in the repository under `probes/` or `checks/` before the scratch is removed, never only in scratch. A Go module cache is read-only, so make it writable before removing it. Never remove another project's scratch.
- A scratch run also needs its own `XDG_STATE_HOME`, because mega writes its log file there whenever it is not attached to a terminal, and a scratch `mega daemon` needs its own `XDG_CONFIG_HOME` for the tray runtime file and `XDG_DATA_HOME` for anything `mega desktop` writes. Never run `mega desktop integrate` or `remove` against Michael's real desktop; installing the launcher is his.
- The e2e round trips take their fixtures from the environment and skip when unset. Michael's values: `MEGA_E2E_CLAUDE_FIXTURE=/home/michael/Repos/q/research/roundhouse-fixtures/claude-rt1`, `MEGA_E2E_CODEX_FIXTURE=/home/michael/Repos/q/research/roundhouse-fixtures/codex-rt1` and `MEGA_E2E_PI_FIXTURE=/home/michael/Repos/q/research/roundhouse-fixtures/pi-rt1`, which `make e2e` also reads from his git-ignored `e2e.local.mk`. The pi round trips run on pi's own default model and skip when it does not answer; the idle model switch round trip then moves to `MEGA_E2E_PI_MODEL`, by default the hosted `openai-codex/gpt-6-sol`, which Michael approved for that one test, and skips where pi does not list it.
- A harness build enters its tested range when `make e2e` admits it. Run it when mega's start line calls a build newer than tested, when a change touches an adapter, and before a tag. Widen `harness.Tested` in the same change, and put the summary line in the day's journal.
- A round trip that fails on a new build is tried on `HEAD`. The same failure there is the build's: capture a card and leave the range.
- A change to what mega asks of a harness adds or changes its round trip, and its row in the map in `internal/e2e`.
- Keep credentials and private transcripts out of reports and committed fixtures.
- Durable project memory goes in `docs/journal/YYYY-MM-DD.md`, one file per day, appended across sessions: current state, gotchas, decisions, and what a future session would otherwise rediscover. Read recent entries before changing anything; where an entry conflicts with the code, the code wins.
- `CHANGELOG.md` follows the house convention: newest release first, prose entries led by `FEATURE:`, `CHANGE:` or `FIX:`, new entries into the `## Unreleased` slot.
- The `Makefile` is the facade: bare `make` builds, `make test` is the full ordinary gate including the UI, `make clean` removes only project-owned artifacts. Harness round trips live under `make e2e` and are run by hand.
- The roadmap is `docs/future/roadmap/`, one markdown file per item. Capture freely with `state: inbox`; never touch `.ranger/`, never delete items unasked.
- Run `unfurl -i <file>` on authored or edited markdown before handoff.

The grimoire's `AGENTS.md`, `agents/agent-roles`, and `software/conventions/` carry the broader practice when available.
