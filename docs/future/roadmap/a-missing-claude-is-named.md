---
title: a missing claude is named
state: inbox
created: 2026-11-13
tags: [defect]
---

When the Claude Code that `claude.executable` names is not there, mega says so, in the same words and the same places it uses for a missing `codex` or `node`: the project's list of what is missing, the new-conversation dialog's "cannot run" mark, and a refusal coded `executable_missing`. Today the page says only "internal error".

## why

Found by the clean-machine rehearsal of 2026-11-13, on a scratch home with no Claude Code installed and the default `claude.executable` of `~/.local/share/claude/versions/2.1.318`:

- **The dialog.** The New conversation dialog selects Claude, as the only harness not marked. Then it shows "The harness did not list its models (internal error); a launch takes what your settings name." The Model list stays at "Loading models…". Launch is enabled, and pressing it shows "internal error". No conversation is made.
- **The API.** `GET /api/projects/{id}/models?harness=claude` and the launch both answer `500 {"code": "internal", "message": "internal error"}`.
- **The project.** Its `environment.missing` names Codex and not Claude.
- **The log.** The real reason is there and nowhere else, as a stack trace: "listing claude models: exit status 1: ReferenceError: Claude Code native binary not found at …/.local/share/claude/versions/2.1.318. Please ensure Claude Code is installed via native installer or specify a valid path with options.pathToClaudeCodeExecutable."
- **The README** says the opposite: "If a harness's executable cannot be found in a project's environment, the new-conversation dialog marks that harness as unable to run".

The cause is in `internal/harness/claude/claude.go`. `executable` looks a bare name up on the project's `PATH`, but an absolute path is passed through unchecked, since "a path that cannot be read is left to fail where it is run". So `Check` passes, and the failure arrives from the SDK as a generic error.

This is the commonest first experience a coworker will have. The pin means anyone without exactly 2.1.318 at exactly that path has a "missing" Claude, and a fresh install of Claude Code puts today's build under `versions/<that version>`. `harness-version-tolerance` decides what range mega accepts. This card is only that a missing one is named.

## shape

- `Check` stats an absolute `claude.executable` and returns a `MissingError` naming the path. A path that exists but is not executable gets the same.
- The reason says what to do: install Claude Code, or set `claude.executable` in the config file mega read, named by its path.
- The same check runs for `codex.binary` when it is a path.
- `checks/firstrun.mjs` records today's words without checking them. When this lands, it checks the mark and the refusal for Claude as it does for Codex.
