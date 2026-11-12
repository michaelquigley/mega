---
title: each project runs in its own environment
state: evaluating
created: 2026-11-11
tags: [defect, spike]
---

Every process mega starts for a project runs in that project's own shell environment, resolved the way the operator activates it in a terminal, and never in the environment mega itself was started from. A launch whose environment cannot be resolved is refused.

## why

This is a hole in the model, not a convenience. In Michael's practice every project session starts through `devprj`, which gives the project its own Go path under `~/local/gopaths/<project>` and puts that project's bin first on PATH. The grimoire's Makefile convention is built on it: `make clean` clears the whole Go bin directory because that directory belongs to one project, and it says the isolation "is the environment's job rather than the Makefile's".

Mega hands every agent in every project the environment mega was started in. So today:

- an agent in any other project that runs `make clean` clears mega's own Go bin, deleting the binary mega runs from;
- an agent that runs `make` elsewhere installs that project's binaries into mega's bin;
- the project's own bin is not on PATH, so a freshly built tool is not what the agent then runs.

Found 2026-11-11, when the first launch from the desktop failed for want of `node` and the question of whose environment an agent inherits was looked at properly.

## how devprj works

`devprj`, in the tools repository, reads `.devprj` in the current directory or falls back to the template in `~/.devprj`, substituting the directory's name. The file names the environment, sets variables and prepends to path-like ones; Michael's template sets a Go path per directory and prepends its bin. The command prints shell code, and a shell function evaluates it. It refuses to activate while another environment is active, so activation must start from an unactivated shell. Activation depends only on the directory it runs in.

## shape

- **Resolution.** For a project, mega runs the operator's interactive shell in the checkout, applies the activation, and captures the resulting environment. The base is a fresh shell environment, not mega's own, which also makes a launcher-started mega behave like a terminal-started one.
- **The definition lives in the repository.** `.mega/environment.yaml` in the checkout describes what the project needs from the place it runs. Its first key is `activate`, a short shell snippet run in the checkout; later keys can describe more, such as a container. Michael's Go projects carry a one-line file naming `devprj`; his C++ audio plugins carry none, because `devprj` is tooling for the Go convention. No file means the fresh shell environment and nothing more. This follows the spec's rule that project truth stays in repositories: the activation travels with the project to another machine and shows up in review, where a registry field would be set once per machine and lost on re-registering.
- **The approval lives in the workspace.** Mega executes the activation in the operator's shell, outside any harness's permission model, and the file sits where agents and strangers' repositories can write it. So mega honours the file only after the operator approves it, and stores a fingerprint of the approved content on the project, through the service. A file that differs from its approved fingerprint is not run: launch, prompt and Start are refused until the operator has seen the change and approved it, with the old and new content shown side by side. This is the same idea as `direnv allow`.
- **A local override.** The operator can tell mega to ignore the repository's file for a project, or to use a different activation locally. That is workspace state too. Without it, someone who clones a repository whose file names a tool they do not have could never launch there.
- **Everything for the project uses it.** Harness launches and resumes, and mega's own helpers that run in a project's name: reading native history, discovery, the model list, path search.
- **Resolved at each runtime start, never stored.** A running runtime keeps the environment it started with; a change takes effect at its next start.
- **Fail closed.** If the activation errors or times out, the launch, prompt or Start is refused with the activation's own message. An agent never starts in the wrong environment.
- **The launched environment is state, and is treated as state.** Each runtime start records what it launched with: the project's activation, the names of the variables the activation changed against the base, and a fingerprint of the whole resolved environment. The conversation header shows the fingerprint and says when it differs from the project's current resolution, so a runtime still running in an older environment is visible. The project page shows the activation, the variables it changes, and when it last resolved.
- **Values are never recorded.** An environment carries credentials. The event log and the registry hold names and a fingerprint only; the values of the changed variables are shown on the project page from a live resolution, on the operator's own machine, and are not stored or sent anywhere else.
- The harness preflight from the launcher card checks executables inside the resolved environment.

## decisions (2026-11-11)

- This is the most important next issue, though not a crisis.
- `devprj` and `noprj` are tooling for the Go convention. The C++ audio plugins do not use them. So the activation is a property of each project, not a workspace-wide default, and the real subject of this card is that the environment a project's work launches in is an important piece of state.
- Michael proposed keeping the definition in the repository as `.mega/environment.yaml`. Adopted, with approval bound to the file's content as the condition that makes it safe.
- Once resolution exists, mega ignores the environment it was itself started in. It does not refuse to run from an activated terminal.
- Existing projects have no file and so no activation. Michael adds the file to each Go project and approves it; nothing is guessed.

## spike

Recorded in the capability report with versions: whether Claude Code passes its process environment to the commands it runs, given that it sources a snapshot of the operator's shell for each one; whether Codex does, or filters it through its shell environment policy; whether a resumed session picks up a changed environment; how long resolution takes through an interactive shell; and how `devprj`'s refusal when already active is best avoided, from a scrubbed base or by deactivating first.

## checks

A service test with a fake activation that sets a marker variable per project: two projects launch with different markers, a failing activation refuses the launch, an unset activation yields the base environment, the start event carries names and a fingerprint and no value, and a changed activation shows the running conversation as launched in an older environment. A file changed after approval refuses the launch and shows both versions; approving it lifts the refusal; an override to ignore the file yields the base environment. With the fixtures and a scratch `.devprj` in each: a Claude turn and a Codex turn that print their Go path and the first PATH entry, each matching its own project and neither matching the other's or mega's.
