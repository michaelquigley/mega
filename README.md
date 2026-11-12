# mega

metawoo mega is a project-oriented workspace for working directly with a gang of coding agents: Claude Code and Codex conversations, grouped by the checkouts they work in, in one page on your own machine. It is not an autonomous coordinator and not a replacement coding harness. Each agent runs as its own harness would in a terminal, with its own sessions, settings and permissions; mega supervises them, shows what needs you, and lets you hand a conversation to a terminal and back.

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

The installed binary is stamped with the path of `shim/claude` in the checkout it was built from, so keep the checkout where it is. `make test` runs the full gate, including the UI build and lint and the shim's own tests. `make clean` removes only what the build made: `ui/dist`, `ui/node_modules`, `shim/claude/node_modules`, what `go clean` removes from the source tree, and the `mega` binary in your Go bin directory. Nothing else in that directory is touched.

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

The launcher runs `mega daemon` from wherever the binary was when you ran `integrate`; run it again if you move the binary. Nothing starts at login.

With no terminal, as when the launcher starts it, mega logs to `~/.local/state/mega/mega.log` (or under `XDG_STATE_HOME`); `mega version` prints the path. At start-up a log past 10 MB becomes `mega.log.1`, replacing an earlier one, and a fresh log begins.

Launching the daemon while mega is already running opens the running mega's page and exits. `mega serve` beside a running mega refuses to start and names it.

Prebuilt binaries are not offered yet: the Claude shim runs from a checkout with its pinned Agent SDK installed beside it, so a binary on its own cannot run Claude conversations.

## Safety posture

- **Loopback only.** The listener must be a loopback address. Requests are refused unless their `Host` is loopback and any `Origin` matches it, which keeps out web pages in your own browser as well as the network.
- **No login.** Anyone who can reach the port on your machine can drive your agents, which is why it never listens anywhere else.
- **Agents run with your permissions.** Each harness runs as you, in your checkout, with your settings and your permission rules. mega never relaxes a harness's permissions, never sets a trust flag on your behalf, and never approves anything for you: every approval it is asked for waits for your answer.

## More

- `docs/current/mega.md` describes the system as built.
- `docs/future/mega.md` is the design it grows toward, and `docs/future/roadmap/` holds the items on the way.
- `docs/research/harness-capabilities.md` records what each harness was observed to do, with versions.

## License

MIT; see `LICENSE`.
