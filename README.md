# mega

build your constellation. mega is where a solo operator runs an orienteering practice with a gang of coding agents. It is one page, on your own machine, where you work directly with them: Claude Code, Codex and pi conversations, grouped by the checkouts they work in. It shows what needs you, docks the approvals and questions the agents ask, and lets you hand a conversation to a terminal and take it back. Each agent runs as its own harness would in a terminal, with your logins, your settings and its own permissions. mega is not an autonomous coordinator, it does not replace a coding harness, and it never answers for you.

![A mega workspace: three projects in the side panel, an approval waiting and a reply unread under "needs you", a pi turn running, and a Claude Code conversation open with an action expanded to show the command it ran and what came back.](docs/images/workspace.png)

*A scratch workspace made with mega's own fakes (`make readme-image`).*

## Try it

### What you need

- **Linux.** It is what mega is built and used on; see [Platforms](#platforms).
- **git, Go 1.27.4 or later, and Node 24**, to build mega. Go's and Node's own pages say how to install them: [go.dev/doc/install](https://go.dev/doc/install), [nodejs.org/en/download](https://nodejs.org/en/download).
- **At least one harness**, installed and logged in as you:
  - **Claude Code**, tested with 2.1.318 to 2.1.331. It updates itself, and a newer build is the normal state.
  - **Codex**, tested with 0.163.0 to 0.172.0, installed whole.
  - **pi**, tested with 1.5.2 to 1.6.1. Optional.

A range's upper end is the newest build mega's harness round trips, `make e2e`, admitted. A build outside its tested range still runs. mega says where each build stands on its first page, in `mega version` and in the version panel, and marks one older than the range, which is the likelier to fail.

### 1. Install a harness and log in

Follow each vendor's own instructions, then run the harness once in a terminal and log in. mega uses those logins as they are and changes nothing in them.

- **Claude Code:** [code.claude.com/docs/en/setup](https://code.claude.com/docs/en/setup). Its native installer puts a `claude` on your `PATH`, which is what mega runs.
- **Codex:** [github.com/openai/codex](https://github.com/openai/codex). mega was tested with Codex installed through npm. However you install it, it must be installed whole: a lone `codex` binary lacks the helper programs Codex runs a shell command with, and mega refuses it.

mega starts every agent in a login, interactive shell in the project's checkout, so your profile and start-up files run. `claude`, `codex` and `node` must be on the `PATH` that shell builds, set in your shell profile, not only in the terminal you start mega from. mega reads that environment when it starts, for its report of which agents it can find, and for each project when something there first needs it, and keeps what it found. So after changing your profile, restart mega.

### 2. Trust each checkout in Claude Code

Run `claude` once in each checkout you will use with mega, and accept its trust dialog. Until you do, Claude ignores the project's permission rules. mega never accepts the dialog for you, and the project's page says when it has not been accepted.

### 3. Build and run mega

```bash
git clone https://github.com/michaelquigley/mega.git
cd mega
make                                      # builds the page, installs the Claude bridge's dependencies, installs mega
export PATH="$PATH:$(go env GOPATH)/bin"  # where make put mega; add this to your shell profile to keep it
mega                                      # serves http://127.0.0.1:6342
```

Then open <http://127.0.0.1:6342>.

- **If `GOBIN` is set,** `make` installs into it instead, and that is the directory to put on your `PATH`.
- **Keep the clone where it is.** mega runs its Claude bridge, `shim/claude`, from the checkout it was built in. If you move the clone, run `make` again from the new place.
- **Another port:** `mega --listen 127.0.0.1:8471`, or `listen:` in the config file. It must be a loopback address.

### 4. Register a project and launch a conversation

The first page says what mega found: each harness with its version, or why it cannot run.

1. **Register the project.** Press **Register a project**, the folder button at the top of the side panel, and give the checkout's absolute path.
2. **Launch.** On the project's page, press **New conversation**, choose the harness, write the opening prompt, and launch. The model and effort start as the harness's own settings name them.
3. **Answer.** An approval or a question docks above the reply box until you answer it. A question Codex asks in its message docks only while the turn that asked is running; once that turn ends, answer it in the reply box. Ctrl+Enter sends a reply. With the reply box empty, it allows the oldest docked approval.
4. **Keys.** Opening a conversation puts the cursor in its reply box, which opens as you type.
   - **Ctrl+Space** puts the cursor back in the reply box from anywhere on the page, except while a dialog is open.
   - **Escape** leaves the box for the history, whose keys then scroll it.
   - **Page Up and Page Down** scroll the history from an empty reply box.
   - **Ctrl+/** opens the next conversation waiting on you, except while a dialog is open.
   - **Ctrl+'** shows and hides the side panel.

### pi, if you want it

Install pi as [pi.dev](https://pi.dev) says, and set up its model in pi itself. mega runs it with the model and thinking level pi's own settings name, local or hosted.

**pi asks for nothing unless an extension does.** It has no permission model and runs its tools without asking, in mega as in its own terminal. A project's own pi extensions load only once you have trusted the checkout in pi's terminal: run `pi` there once. mega never trusts a checkout for pi.

## What to expect

- **Loopback only, and no login of its own.** mega listens on `127.0.0.1` and refuses anything else. Anyone who can reach its port can drive your agents, so never expose it to a network. To reach it from another machine, go through something that authenticates you first, such as an ssh tunnel or a private share closed to your own account.
- **Your own logins.** Each harness runs as you, with your login, settings and permission rules.
- **Permissions as each harness has them.** mega never relaxes a harness's permissions, never accepts a trust dialog for you, and never approves anything for you. Every approval waits for your answer.
- **What mega stores, and where:**
  - **The workspace,** in `~/.local/share/mega/`: the projects you registered and each conversation’s timeline. It contains the adoption snapshot, if there was one, and the prompts, replies and actions mega witnessed. Work done on the session outside mega is not shown; adopting the session again brings it in.
  - **Project environments** are recorded by variable names and fingerprints, never values.
  - **Its log,** when it runs with no terminal: `~/.local/state/mega/mega.log`.
  - **Each harness's own sessions** stay in that harness's own store.
- **Configuration snapshots.** mega keeps copies of each harness's configuration in `~/.local/share/mega/snapshots/`, readable only by you. It takes one at start-up when the configuration changed, and when a harness reports a new version.
  - **Never copied:** credentials, transcripts and caches.
  - **May still hold secrets:** a configuration file can, such as a settings `env` block, so treat the snapshots as private.
  - **Never written back.** mega never writes a harness's configuration. `mega snapshots` lists the snapshots, and `mega snapshots diff NAME` prints the copy commands to restore one.

## Platforms

- **Linux** is what mega is built and used on, on amd64.
- **macOS** builds, on arm64, thanks to a contributed change. Its tests do not all pass there yet, and it has not been used for real work. One known gap: on a Mac, mega cannot yet see a process that outlives its runtime, so a release there says it could not check.
- **Windows** is expected to work under WSL2, following the Linux steps inside it, and has not been tried. Native Windows is not supported.

There are no prebuilt binaries yet: you build mega from a clone, as above.

## Where things go wrong

Each of these is what mega, or your shell, says, and what to do.

- **`mega: command not found`** after `make`. Your Go bin directory is not on your `PATH`: add it, as step 3 shows.
- **"Claude Code cannot run: 'claude' was not found on your shell's PATH, and no build is installed in '…/.local/share/claude/versions'; …"** on the first page. Claude Code is not installed, or not on the `PATH` your shell profile builds. Install it and reload the page. If you had to change your profile's `PATH`, restart mega first. The same holds for Codex and pi: "'codex' was not found on your shell's PATH".
- **"Claude Code cannot run: 'node' was not found on your shell's PATH"**, while `node` works in your terminal. Your shell profile does not put Node on the `PATH`. Add it there, and restart mega.
- **"'codex' at '…' is a lone binary; install Codex with npm or from the release's codex-package archive, whose helper programs it needs".** Reinstall Codex whole, as step 1 says.
- **A build outside the tested range.** A newer build is said plainly: "version 2.1.340, … newer than the 2.1.318 to 2.1.331 mega was tested with". An older one is marked: "Claude Code '2.1.300' is older than the builds mega was tested with, '2.1.318 to 2.1.331'; the Agent SDK mega runs expects Claude Code '2.1.318' or later, so some calls may fail". Update the harness.
- **"Claude has not been trusted in this checkout, so it ignores the project's permission rules. Run claude here once and accept the trust dialog; mega will not accept it for you."** Do as it says, in that checkout. For pi: "pi has not been trusted in this checkout, so its project extensions, settings and MCP servers did not load…": run `pi` there once.
- **"Claude Code cannot run: claude shim: stat …/shim/claude/shim.mjs: no such file or directory"** after moving or deleting the clone mega was built from. Put it back, or run `make` again where it is now.
- **"another mega is already running at 'http://127.0.0.1:6342'; open it there, or stop it (Quit in its tray, or Ctrl+C where it runs) before starting this one"**, or the port being in use by another program. Open that one or stop it, or serve this one on another port with `--listen`.
- **"another mega is running on the data directory '…', at 'http://127.0.0.1:8470'; open it there, or stop it before starting this one".** A mega already holds your workspace, such as one started before the default port moved to 6342 and still serving 8470. Another port does not help: one workspace has one mega. Open the one it names, or stop it and start again. A second, separate mega needs its own `--data` directory.
- **A project's `.mega/environment.yaml` not yet approved.** If a repository defines how its environment is activated, nothing starts in it until you approve that file on the project's page. See [Project environments](#project-environments).

## Configuration

Configuration is optional, in `~/.config/mega/mega.yaml`. This example is safe to copy whole; change what you need and leave out the rest:

```yaml
listen: 127.0.0.1:6342    # loopback only; anything else is refused
name: my-laptop           # what the tab and side panel call this mega; the hostname by default
path_search:              # directories @~/ in the reply box reaches beyond the checkout; none by default
  directories:
    - ~/Screenshots
```

| Key | Default | What it does |
| --- | --- | --- |
| `listen` | `127.0.0.1:6342` | Where mega serves; loopback only. `--listen` overrides it. Before 2026-11-18 the default was 8470. |
| `data` | `~/.local/share/mega` | The workspace. `--data` overrides it. |
| `name` | the hostname | What the tab title, the side panel and `mega version` call this mega. |
| `claude.executable` | `claude` on your `PATH` | Another Claude Code, as a native build's path. |
| `claude.node` | `node` on your `PATH` | Another Node for the Claude bridge. |
| `claude.shim` | the clone mega was built from | The Claude bridge's directory. Leave it unset unless you moved the bridge. |
| `codex.binary` | `codex` on your `PATH` | Another Codex. |
| `pi.binary` | `pi` on your `PATH` | Another pi. |
| `environment.shell` | your `SHELL` | The shell projects' environments are resolved in. |
| `environment.timeout` | `10s` | How long resolving a project's environment may take. |
| `snapshots.keep` | `10` | Ordinary configuration snapshots kept; `0` takes none. |
| `snapshots.keep_at_version` | `20` | Snapshots taken at a harness's new version, kept longer. |
| `path_search.directories` | none | Directories `@~/` or `@/` lists beyond the checkout, a folder's own files first, newest first, and nothing outside them. |
| `projects_root` | your home directory | Where the register dialog's folder picker starts, and all it lists. A path typed in full is registered wherever it is. |

`mega --config FILE` reads another config file. `mega version` says what this mega is called, where it logs, each harness's tested range and the version it finds, and where the snapshots are.

## Project environments

Every agent in a project runs in that project's own shell environment: the one a terminal opened in the checkout, and activated your usual way, would have. mega never hands an agent the environment mega itself was started in.

A repository can say how to activate its environment, in `.mega/environment.yaml`:

```yaml
# a project that uses direnv
activate: eval "$(direnv export bash)"
```

A project with no such file gets your shell's own environment in the checkout.

**mega will not run a repository's activation until you approve it** on the project's page. If the file changes, nothing starts in that project until you have compared the versions and approved the new one. Approving the file approves its text, nothing more: an activation that reads other files is trusting them. So mega finds its own programs before the activation runs, and remembers which variables the activation changes, asking again if a later start changes others. The project's page can also ignore the file, or run another activation on this machine.

## Running as a daemon

`mega daemon` runs the same service under a tray icon. Its menu opens the page, arms "shut down when idle", and quits; Quit goes through that gate, so a stray click never interrupts an agent. The icon turns gold while something needs you.

```bash
mega desktop integrate   # installs a desktop launcher and its icons under ~/.local/share
mega desktop remove      # removes exactly those files
```

The launcher runs `mega daemon` through your interactive shell, from wherever the binary was when you ran `integrate`; run it again if you move the binary. Nothing starts at login. Launching the daemon while mega is already running opens the running mega's page and exits.

## More

- [`docs/current/mega.md`](docs/current/mega.md) describes the system as built, in full.
- [`docs/future/mega.md`](docs/future/mega.md) is the design it grows toward, and `docs/future/roadmap/` holds the items on the way.
- [`docs/research/harness-capabilities.md`](docs/research/harness-capabilities.md) records what each harness was observed to do, with versions.

## License

MIT; see `LICENSE`.

The Claude Code, Codex and pi marks on the control bar say which harness a conversation runs on; they belong to their owners and imply no endorsement.
