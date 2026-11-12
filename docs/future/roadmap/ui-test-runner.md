---
title: UI test runner
state: researching
created: 2026-11-09
tags: [defect]
---

The interface has no tests in the repository. Give its browser checks a home in the tree and one command that runs them, and rebuild the checks that were lost.

## why

Every batch since stage 2 verified its interface work with a Playwright script, and every one of those scripts lived in the implementer's scratch directory under `/tmp`. A reboot on 2026-11-12 erased all of them, about thirty. They were the interface's only regression suite: batch after batch reported that the panel, start and shutdown checks "still pass", and those checks no longer exist. Nothing is broken today, but the next change to the page has no net, and a missing check looks exactly like a passing one.

How it happened is worth keeping: in stage 2 the planning agent chose a throwaway script over a test runner for one fix, then wrote "run the browser check" into every later prompt and lumped browser scripts in with scratch data in `AGENTS.md`. The scripts were treated as spikes while they became the suite.

## shape

- **Checks live in the tree.** A `checks/` directory at the repository root, outside the UI's lint gate or given its own lint settings, holding one script per surface. The fakes they drive are already committed: `shim/claude/fake/shim.mjs` and `internal/harness/codex/fake/`.
- **One command.** A `make` target builds mega, starts it on a scratch port with scratch data and scratch XDG directories, runs every check against the fakes, and stops it. No model turns, no real harness, nothing touching the operator's instance. The Makefile convention keeps environmental checks out of `make test`, so this is its own target, and `AGENTS.md` requires it for any batch that touches `ui/`, the server's API, or the stream.
- **The browser is declared, not assumed.** Playwright as a pinned development dependency, with the browser install documented, and a clear message instead of a silent skip when it is missing.
- **Only data is scratch.** Generated conversations, screenshots and logs go to a scratch directory; scripts never do.
- **CI runs it** in the existing workflow once it is stable locally.

## what to rebuild

From the journals, grouped by surface so thirty batch-shaped scripts become a handful of suites. Each journal entry records what its check asserted.

- **Connection and honesty:** the refresh race, a failed refresh, reconnect sync, the never-loaded and could-not-read states (`refresh-race`, `refresh-fail`, `reconnect-sync`, `unreachable`).
- **Attention and reading:** the needs-you order, the next-waiting keys, reading never resolving a request, the read gate (`stage4`, `read-gate`).
- **Conversation view:** blocks and action groups, markdown and its safety rules, unclipped messages and the code collapse, copy as markdown and plain text, the recent tail (`structure`, `markdown`, `unclipped`, `copy`, `codecopy`, `history`).
- **Input panel:** resting and opening, the scroll that follows it, sending, docked requests, path search, Start, the mode, model and effort controls (`panel`, `panelscroll`, `paths`, `start`, `modes`, `models`, `idle-mode`).
- **Requests:** permission and question cards, MCP approvals and typed forms (`mcp-cards`, and the request steps of `stage3`).
- **Side panel and rail:** sorting, folding, stars, the rail, working and background marks, a missing checkout, project removal (`side`, `indicators`, `background`, `project-remove`).
- **Workspace-wide:** the shutdown gate, the favicon and title states, adoption (`shutdown`, `favicon`, and the adoption steps of `stage3`).

`checks/rescued/` holds the three scripts and the conversation generator that survived, from the recent-tail batch; fold them in and delete that directory.

## not in this card

Unit tests for the page's pure functions, which can follow in the same directory with Node's own test runner; visual snapshot comparison; the harness round trips, which stay under `make e2e`.
