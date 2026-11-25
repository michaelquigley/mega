# Harness versions probe

This probe is how mega's tested ranges are extended. `harness.Tested` in `internal/harness/versions.go` names, for Claude Code and Codex, the lowest and highest build between which every build this probe ran passed the steps below; pi has no runner here, and its range is the build `probes/pi` ran. That is what the range claims: these steps, on the builds the capability report lists, not every call mega makes on every build in it. To add a build to a range, run this probe against it, record what it saw, then widen the range.

Each run starts a scratch mega built from this checkout. The build under test is named in the scratch config, and the run makes the e2e round trips' calls (`internal/e2e`) over mega's HTTP API. The e2e tests cannot do this themselves: a project's environment never takes mega's own, so they cannot switch Claude Code's updater off or point Codex at a scratch `CODEX_HOME`. Here the fixture project's local override does both.

The probe runs real harness sessions on Michael's own login, as the builds run in his terminal, against the disposable fixtures only. A Claude Code run costs six model turns, and a Codex run seven.

## What it needs

- **Claude Code.** A native build, such as `~/.local/share/claude/versions/2.1.330`, or the bare name `claude`. The shim's pinned Agent SDK comes from `make shim`.
- **Codex.** A whole install: npm's, or the release's `codex-package-<target>.tar.gz` unpacked with its `bin/codex`. The lone `codex-<target>.tar.gz` holds only `codex`, without the helper programs. Mega refuses it, and a shell command in a turn fails without them.
- **The fixtures**, named as for `make e2e` (see `internal/e2e/doc.go`):
  - `MEGA_E2E_CLAUDE_FIXTURE`; Michael's is `/home/michael/Repos/q/research/roundhouse-fixtures/claude-rt1`.
  - `MEGA_E2E_CODEX_FIXTURE`; Michael's is `/home/michael/Repos/q/research/roundhouse-fixtures/codex-rt1`.
- **A build of the UI** from `make`, which mega embeds, and `node`, `go` and `curl`.
- **A scratch port.** Port 6342, the default, and 8470, the default before 2026-11-18, are refused, and so is a port something already answers on.

## Running it

```sh
export MEGA_E2E_CLAUDE_FIXTURE=/home/michael/Repos/q/research/roundhouse-fixtures/claude-rt1
export MEGA_E2E_CODEX_FIXTURE=/home/michael/Repos/q/research/roundhouse-fixtures/codex-rt1

probes/versions/claude.sh ~/.local/share/claude/versions/2.1.330 18501
probes/versions/codex.sh /path/to/codex-0.170.0-package/bin/codex 18520 --discover
```

`claude.sh` runs `driver.mjs` and `codex.sh` runs `codex-driver.mjs`; `lib.mjs` and `lib.sh` hold what they share. `--setup-only` stops either run after the environment guard, before anything runs the harness.

`--discover` is for Codex only. After the run, it starts a second scratch mega on fresh data and the same scratch `CODEX_HOME`. That mega discovers the run's thread and adopts it, importing its history.

## What a run does

**Both runners** turn configuration snapshots off in the scratch config (`snapshots.keep: 0`), since the scratch mega runs on Michael's own `HOME`. They build mega into the scratch root, `$MEGA_PROBE_DIR` (by default `${TMPDIR:-/tmp}/mega-versions-probe`). Each run gets its own directory there. Mega runs with scratch data and XDG directories, but with Michael's own `HOME` and login.

**Before and after each run,** the runners record what a run could change and stop with exit 2 if it changed:
- **For Claude Code:** the builds in `~/.local/share/claude/versions/`, by name, size and time, and where `claude` on `PATH` points.
- **For Codex:** the real `~/.codex` at the top level, by name, size and time. A name that comes or goes, or a changed configuration file, stops the run. Other changes are reported, since Michael's own Codex processes write logs and caches there as they run.

**`driver.mjs` (Claude Code)** gives the fixture the override `export DISABLE_AUTOUPDATER=1 DISABLE_UPDATES=1` and resolves the project from it, as the project page's Resolve does, since nothing resolves a project in the background. It then makes these calls:
- the model list, with its count, version and time;
- a launch and the marker turn;
- an approval for Bash `sleep 3`, asserted to be exactly that before Allow;
- an interrupt with a second such approval pending, which is never answered: the turn ends `interrupted` and the request `aborted`;
- a live switch to plan and back, each confirmed;
- MCP status, by counts only;
- a release, then a start without a prompt, which reports the build's version and imports nothing;
- a prompt in the resumed session;
- a model switch while idle, to another listed model, `sonnet` where it can, which Claude confirms at once. Over five idle seconds no message or turn may be recorded, and the conversation must not be running. Whether the build echoes the switch as a replayed user message, which mega records as the native "user/replay", goes in the summary's `idle_switch.echo`. The next prompt's turn must ask for the new model and run on it;
- a release and an exact-id resume by a prompt that must remember the conversation, with zero events imported;
- a release, and every turn's `differs`;
- the conversation removed and its session discovered and adopted again; the adoption’s snapshot must include the known assistant reply and omit Claude’s synthetic interruption and local model command from speech.

**`codex-driver.mjs` (Codex)** gives the fixture the override `export CODEX_HOME=<scratch home>`.
- **The guard.** It launches nothing, not even the model list, until two things hold. The project's resolution, from the override and made after the override was set, must change `CODEX_HOME`. A live resolution must then set `CODEX_HOME` to the scratch home. That one value is the only one read.
- **The calls** are Claude Code's, with these differences:
  - the approval is for a write outside the checkout;
  - the interrupted approval is for a second write, and must leave nothing written;
  - an MCP approval for the fixture's `fixture/echo` is added: the tool must not run before Allow and must run after;
  - there is no mode switch;
  - the idle model switch, `gpt-6-luna` where it can, is stored for the next prompt (`next_prompt`), whose turn must run on it.

**The scratch `CODEX_HOME`** is `<scratch root>/codex-home`, made on the first Codex run and shared by every later one, so each build reads what the others wrote. It holds copies of `~/.codex/config.toml`, any other `*.toml`, `AGENTS.md`, `rules/` and `skills/`. It also holds a symbolic link to `~/.codex/auth.json`, which is never copied or read. Codex warns about the location under `/tmp`, saying "Refusing to create helper binaries under temporary dir"; none of the calls needs them.

**The approvals.** Each driver answers only the approvals it has asserted, and only with Allow. A request that is not the one expected stops the run unanswered. So does a turn that ends without raising one, which is how a Codex without its helpers shows.

## Reading the result

Each step prints `ok` or `FAIL`, and a run ends with a JSON summary, which is also written to `summary.json` in its directory. The summary has:
- the version each start reported;
- the model list;
- mega's own start-up check of the build, with its standing against `harness.Tested`;
- the steps passed and the steps that failed;
- every event imported from native history;
- for Claude, the adopted conversation, the snapshot’s count of prompts and replies, and the results of the speech checks;
- the turns and any that differed.

A build is fit to add to a range when every step passed and the state checks found nothing changed.

Claude Code can write “No response requested.” after the interrupted turn and record the local `/model` command in its native transcript. The exact-id resume must import zero events, just as a start without a prompt does. The adoption’s snapshot must show the known assistant reply `DELTA-7a8b` as speech and omit both bookkeeping records from speech.

Each run directory holds `driver.out`, `summary.json`, `version.txt`, `state-before.txt` and `state-after.txt`. It also holds mega's own output in `mega.out` and its log in `xdg/state/mega/mega.log`; a warning about versions would show there. A Codex run's `adopt/` holds the second mega's run.

## Extending a range

1. Run the probe against the new build, and against the range's current end as a control if anything looks different.
2. Record what it saw in `docs/research/harness-capabilities.md`, with the build's version and the date.
3. Change `harness.Tested` in `internal/harness/versions.go`.
4. Note the run and the change in the day's journal.

## Cleaning up

The scratch root is not removed by the runners. Remove the link to the real Codex login first, by itself, then the rest:

```sh
rm "${MEGA_PROBE_DIR:-${TMPDIR:-/tmp}/mega-versions-probe}/codex-home/auth.json"
rm -rf "${MEGA_PROBE_DIR:-${TMPDIR:-/tmp}/mega-versions-probe}"
```
