---
title: UI test runner
state: building
created: 2026-11-09
tags: [defect]
---

The interface's browser checks have a home in the repository but no way to run them as a suite, and all but one were lost. Build the runner, and rebuild the three suites that guard what can hurt. The rest come back as their surfaces are next changed.

## why

Every batch since stage 2 verified its interface work with a Playwright script kept in the implementer's scratch directory under `/tmp`. A reboot on 2026-11-12 erased about thirty of them. They were the interface's only regression suite: batch after batch reported that the panel, start and shutdown checks "still pass", and those checks no longer exist. A missing check looks exactly like a passing one. `checks/environment.mjs`, written after the reboot, is the one real check in the tree; `checks/rescued/` holds three scripts and a conversation generator saved from the last scratch directory.

How it happened, kept so it is not repeated: the planning agent chose a throwaway script over a test runner for one stage 2 fix, then wrote "run the browser check" into every later prompt and grouped browser scripts with scratch data in `AGENTS.md`.

## shape

- **One command.** A `make` target that builds mega, runs every check in `checks/` against the committed fakes (`shim/claude/fake/shim.mjs`, `internal/harness/codex/fake`), and reports per suite. Each check starts its own mega on a scratch port with scratch data and scratch XDG directories, as `environment.mjs` already does. No model turns, no real harness, never the operator's instance. It is its own target, not part of `make test`, per the Makefile convention.
- **The browser is declared, not assumed.** Today a check is handed the path of a `playwright-core` install and a Chromium binary on its command line. Make Playwright a pinned development dependency with the browser install documented, so the target needs no arguments, and fail with a clear message when the browser is missing.
- **Isolated from the operator's harness settings.** Since project environments landed, mega's own environment no longer reaches the harness processes it starts, so a check cannot point Claude at scratch settings by setting a variable on the scratch service. Give each scratch project an override activation that does. `environment.mjs` currently reads the operator's real Claude settings, read-only, for model labels; remove that.
- **Shared plumbing.** What `environment.mjs` does for itself (start mega, wait for it, drive the fakes, stop it, clean up) becomes a small helper the suites share, so a new suite is its steps and nothing else.
- **Only data is scratch.** Generated conversations, screenshots and logs go to a scratch directory; scripts never do.
- **A check that cannot be made reliable is worse than none.** Wait on conditions, never on delays; report any step that could not be made stable instead of leaving it flaky.

## ride-along: the build's warnings

`npm install` in `ui/` prints a deprecation warning and "7 moderate severity vulnerabilities" (seen 2026-11-12). None of it ships: `npm audit --omit=dev` finds nothing, and neither does the shim's package. But the repository is public, and a stranger's first build should not open with that.

- The seven are one advisory counted along its dependency path: a denial-of-service weakness in `minimatch` (GHSA-g3hv-75wg-m249), reached through `typescript-eslint` and its parser. The linter only ever runs on the project's own source.
- Do not run `npm audit fix --force`. The fix it offers is `typescript-eslint@8.36.0`, a downgrade, because the newest release is still in the affected range. Use an `overrides` entry pinning `minimatch` to a patched release if one is compatible with what depends on it; if none is, leave it and record why in the README's build section.
- ESLint 9 is out of support. Move `eslint` and `@eslint/js` to 10, with whatever the config and the plugins need, and fix any new lint findings properly, not by disabling rules.
- Take the patch-level updates that are waiting (`vite`, `@vitejs/plugin-react`).
- Leave TypeScript at 5.9. TypeScript 7 is a major change to the compiler and wants its own card.

Done when a clean `npm ci` in `ui/` and in `shim/claude/` prints no deprecation warning and reports no vulnerabilities, or says in the README exactly which remain and why.

## the suites to rebuild now

The three that guard what can hurt, rebuilt from what the journals recorded and from `docs/current`:

- **Requests.** A permission card and a question card each answer and clear; a request already answered elsewhere cannot be answered again; a docked request stays above the input panel and does not move under the pointer; an MCP approval and a typed form from the fake Codex; the history keeps its one-line row. (Lost scripts: `mcp-cards`, the request steps of `stage3`, `read-gate`.)
- **The input panel.** Resting and opening; sending closes it and goes to the end; a refused send keeps the draft; the refusal reasons on Send and Start; Start on an idle conversation; the mode, model and effort controls, including "next prompt". (Lost: `panel`, `panelscroll`, `start`, `modes`, `models`, `idle-mode`.)
- **The shutdown gate.** Arming refuses new work and names what it waits for; a second page sees it; cancelling lifts it; Shut down is offered only at idle and ends the service cleanly; the page that asked recovers when the service returns. (Lost: `shutdown`.)

Fold in `checks/rescued/`: its history check becomes the start of the conversation view suite or is kept as it is, its generator moves beside the helper, and the `rescued` directory goes.

## the rule for the rest

A surface's suite is rebuilt before a batch changes that surface. The remaining groups, each with the lost scripts that covered it, are rebuilt by the first batch that touches them:

- **Connection and honesty:** `refresh-race`, `refresh-fail`, `reconnect-sync`, `unreachable`.
- **Attention and reading:** `stage4`.
- **Conversation view:** `structure`, `markdown`, `unclipped`, `copy`, `codecopy`, `history`.
- **Path search:** `paths`.
- **Side panel and rail:** `side`, `indicators`, `background`, `project-remove`.
- **Favicon and title, adoption:** `favicon`, the adoption steps of `stage3`.

This card is done when the runner and the three suites pass three times running. It does not wait for the rest.

## not in this card

Unit tests for the page's pure functions; visual snapshot comparison; the harness round trips, which stay under `make e2e`; running the checks in CI, which follows once they have been stable locally for a while.
