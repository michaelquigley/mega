# mega

metawoo mega: build your constellation. mega is where a solo operator runs an orienteering practice with a gang of agents.

It is a project-oriented workspace for working directly with those agents: Claude Code and Codex conversations, grouped by the checkouts they work in, in one page on your own machine. It is not an autonomous coordinator and not a replacement coding harness. Each agent runs as its own harness would in a terminal, with its own sessions, settings and permissions; mega supervises them, shows what needs you, and lets you hand a conversation to a terminal and back.

One Go binary serves the workspace and its UI on loopback. A small Node bridge (`shim/claude`) talks to Claude Code through its Agent SDK; Codex is driven through `codex app-server`.

## What it needs

- Go (the version in `go.mod`) and Node 24, to build.
- Claude Code 2.1.318, the build the shim's pinned Agent SDK (0.3.318) was matched against, at `~/.local/share/claude/versions/2.1.318` or wherever `claude.executable` says.
- codex-cli 0.166.0 on `PATH`, or wherever `codex.binary` says.
- Your own Claude and Codex logins. mega uses them as they are and changes nothing in them.

## Build and run from source

```bash
git clone https://github.com/michaelquigley/mega.git
cd mega
make          # builds the UI, installs the shim's pinned SDK, installs `mega` into your Go bin directory
mega          # serves http://127.0.0.1:8470
```

The installed binary is stamped with the path of `shim/claude` in the checkout it was built from, so keep the checkout where it is. `make test` runs the full gate, including the UI build and lint and the shim's own tests. `make clean` removes only what the build made: `ui/dist`, `ui/node_modules`, `shim/claude/node_modules`, `checks/node_modules`, what `go clean` removes from the source tree, and the `mega` binary in your Go bin directory. Nothing else in that directory is touched.

A clean `npm ci` in `ui/` and in `shim/claude/` prints no deprecation warning and reports no vulnerabilities. `ui/package.json` pins `minimatch` to 11.0.8 with an `overrides` entry. ESLint and `typescript-eslint` depend on a 10.x release inside the range of GHSA-g3hv-75wg-m249, for which no 10.x fix exists. The linter only reads the project's own source, and nothing from it ships.

The browser checks in `checks/` drive the page with Playwright against a scratch mega and fake harnesses. They are not part of `make test`. Install the browser they use once, then run them:

```bash
make checks-browser   # downloads the Chromium the pinned Playwright drives, into ~/.cache/ms-playwright
make checks           # every suite; make checks SUITE=requests runs one
```

`make checks` fails, saying so, when that browser is missing. `checks/README.md` says what each suite covers and how to add one.

Configuration is optional, in `~/.config/mega/mega.yaml`:

```yaml
listen: 127.0.0.1:8470       # loopback only; anything else is refused
data: ~/.local/share/mega    # the workspace: registry and conversation logs
claude:
  node: node
  shim: /path/to/mega/shim/claude
  executable: ~/.local/share/claude/versions/2.1.318
codex:
  binary: codex
```

`mega --data DIR` runs against another workspace, and `mega --config FILE` reads another config.

### Running as a daemon

`mega daemon` runs the same service under a tray icon. The menu has "open mega" to open the page, "shut down when idle" to arm the shutdown gate (it reads "cancel shutdown" while armed), and Quit. Quit goes through the gate: on an idle workspace it releases every agent and stops; with anything in flight it arms the gate and opens the page instead, so a stray click never interrupts an agent. The tray's icon turns gold, with a count in its tooltip, while something needs you.

To launch it from the desktop:

```bash
mega desktop integrate   # installs com.michaelquigley.mega.desktop and its icons under ~/.local/share
mega desktop remove      # removes exactly those files
```

The launcher runs `mega daemon` through your interactive shell (the one `SHELL` names: bash, zsh or fish), so mega itself starts the way a terminal-started mega does. The agents don't depend on it: each project runs in its own environment (below). `integrate` prints the exact `Exec` line it wrote. `mega desktop integrate --plain` writes the direct form if you want it. The launcher runs the binary from wherever it was when you ran `integrate`; run it again if you move the binary. Nothing starts at login.

If a harness's executable cannot be found in a project's environment, the new-conversation dialog marks that harness as unable to run, and its conversations refuse new work with the name of what is missing.

With no terminal, as when the launcher starts it, mega logs to `~/.local/state/mega/mega.log` (or under `XDG_STATE_HOME`); `mega version` prints the path, and so does the panel the version in the side panel's footer opens. At start-up a log past 10 MB becomes `mega.log.1`, replacing an earlier one, and a fresh log begins.

Launching the daemon while mega is already running opens the running mega's page and exits. `mega serve` beside a running mega refuses to start and names it.

Prebuilt binaries are not offered yet: the Claude shim runs from a checkout with its pinned Agent SDK installed beside it, so a binary on its own cannot run Claude conversations.

## Project environments

Every agent in a project runs in that project's own shell environment: the one a terminal opened in the checkout and activated your usual way would have. mega never hands an agent the environment mega itself was started in. mega starts your shell in the checkout from a clean base, as a login and interactive shell so your profile scripts and start-up files both run, and applies the project's activation, at every start of a runtime and for its own helpers.

The activation lives in the repository, in `.mega/environment.yaml`, as one key:

```yaml
# a Go project isolated by devprj: its own GOPATH, its bin first on PATH
activate: devprj
```

```yaml
# a project that uses direnv
activate: eval "$(direnv export bash)"
```

A project with no file gets your shell's own environment in the checkout and nothing more, which suits a project that needs no activation.

**mega will not run a repository's activation until you approve it.** The project page shows the file and an Approve control. If the file changes after you approve it, nothing starts in that project until you have compared the two versions and approved the new one. The approved version is found in the checkout's git history; mega stores only its SHA-256 digest. An approval made before mega stored the full digest needs approving once more. A local override on the project page lets you ignore the file, or run another activation on this machine. A file that cannot be read, an activation that fails, and one that takes longer than ten seconds each refuse to start anything, and say why.

**Approving the file approves its text, nothing more.** An activation that reads other files in the repository is trusting them. `devprj` reads `.devprj`, and a snippet may source a script, and an agent can usually change such a file without asking. Two things limit that:
- **mega finds its own programs before the activation.** `node`, `git` and the harness binaries come from your shell's environment as it was before the activation ran. An activation can change what an agent's commands see, but never which program mega itself runs.
- **The approval remembers the names of the variables the activation changed.** If a later start finds it changing a different set, a new `LD_PRELOAD` say, nothing starts until you have looked and approved again. The project page shows the added and removed names, never their values.

mega records which environment each runtime started in by names and a fingerprint, never values. The control bar at the foot of a conversation marks a running agent that started in an older environment than the project's current one; the header keeps the fingerprint. The project page shows the variables the activation changes, and their values only when you ask.

## Safety posture

- **Loopback only.** The listener must be a loopback address. Requests are refused unless their `Host` is loopback and any `Origin` matches it, which keeps out web pages in your own browser as well as the network.
- **No login.** Anyone who can reach the port on your machine can drive your agents, which is why it never listens anywhere else.
- **Agents run with your permissions.** Each harness runs as you, in your checkout, with your settings and your permission rules. mega never relaxes a harness's permissions, never sets a trust flag on your behalf, and never approves anything for you: every approval it is asked for waits for your answer.
- **A conversation's model, effort and permission mode are its own.** A new conversation copies them once from the harness's settings, and mega passes them on every launch; only you change them after that. A bypass mode in your settings is never copied: the conversation starts in the mode that asks. The control bar shows what you request, and each turn's line shows what the harness ran with.

## More

- `docs/current/mega.md` describes the system as built.
- `docs/future/mega.md` is the design it grows toward, and `docs/future/roadmap/` holds the items on the way.
- `docs/research/harness-capabilities.md` records what each harness was observed to do, with versions.

## License

MIT; see `LICENSE`.

The Claude Code and Codex marks on the control bar say which harness a conversation runs on; they belong to their owners and imply no endorsement.
