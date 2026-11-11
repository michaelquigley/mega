---
title: roundhouse -> mega
state: evaluating
created: 2026-11-10
tags: [epic]
---

we're going to be renaming this project from `roundhouse` to `mega`... and at that point, we'll be moving it into a `git.hq.quigley.com/products/mega` repository.

## audit for going public (2026-11-10)

Michael wants the renamed project on `github.com`, as infrastructure to share. The repository and its whole history were checked before planning the move.

- **Secrets:** none. No key, token or credential shape in any tracked file or anywhere in the 27 commits of history.
- **Transcripts and logs:** none were ever committed. Probe logs are ignored; the only files ever deleted from history are retired roadmap cards, the work order and one UI component. The four adapter `testdata` files are scrubbed frames.
- **Account facts:** no MCP server names, API key sources or account identifiers in tracked files. The one `"email"` hit is a field name in the Codex schema.
- **Identity:** the only email in history is the commit author's, the same one on Michael's other public repositories.
- **Dependencies:** every Go module is already on GitHub, including `push` and `df`. Nothing private is needed to build.
- **Internal names that would become public:** the module path (`git.hq.quigley.com`, 131 lines, all removed by the rename itself); `apps.hq` and `roundhouse.hq.quigley.com` in the spec and one roadmap card; the workstation names `fortyfive` and `twentytwo` in about ten places; `/home/michael` paths in probes, the journal, `AGENTS.md` and two e2e tests. None is sensitive; they are a matter of taste.
- **Mentions of private tooling** (terminus, mercurius, the grimoire, the canon): present in `AGENTS.md`, docs and journals, as they are in Michael's other public repositories.
- **Precedent:** ranger, scry, pane and df are public with their journals, `AGENTS.md` and `docs/future` tracked, an MIT licence where one exists, and a GitHub Actions `ci.yml` that builds on `v*` tags with push's stamping scripts and drafts a release.

## what going public adds to the rename

- A `LICENSE` and a `README.md`; the repository has neither.
- The two e2e tests hard-code the fixtures under `/home/michael`; they need an environment variable and a skip when it is unset.
- A GitHub Actions workflow on the house pattern.
- An attribution line for the checked-in subset of the Codex app-server schema.
- **Distribution of the Claude shim is unsolved.** The binary finds the shim through a path stamped at build time, and the shim needs the pinned Agent SDK installed beside it. A release artifact alone cannot run Claude conversations. Until that is designed, building from source is the only supported install, and CI should build and test without publishing binaries.

## decisions (2026-11-11)

- The project becomes **mega**, spoken of as "metawoo mega", following the pattern of metawoo reef and metawoo flo. Binary `mega`, module `github.com/michaelquigley/mega`, config at `~/.config/mega/mega.yaml`, data at `~/.local/share/mega`, checkout at `products/mega`, the port unchanged.
- **MIT licence.** Michael's interest is maintenance, services and consulting around it, and its place in the portfolio, not selling access.
- The full history is carried to GitHub; the audit above found nothing to scrub.
- Workstation and `apps.hq` names stay in the living docs. Journals and the CHANGELOG stay as written, with a dated note recording the rename.
- The first public version builds from source only; distributing the Claude shim is its own card.
- This project's two conversations come across the move by moving Claude's session store for the checkout, after a probe on a fixture. The fixtures directory stays at `research/roundhouse-fixtures` for now.
- Automatic delivery of GitHub builds to HQ is a card on push's roadmap, not part of this.

## phase a: rename in place (an implementer batch)

Done in the current checkout, without moving any directory and without touching Michael's live data.

- Module path, imports, `cmd/roundhouse` to `cmd/mega`, the application name constant that drives the config and data directory names, the config file name, the cobra command, the version command, the Makefile's stamped symbol path, the shim package name, the Codex client identity, and every user-facing string, with the display name lowercase `mega` and the page title "metawoo mega".
- `docs/current/roundhouse.md` to `docs/current/mega.md` and `docs/future/roundhouse.md` to `docs/future/mega.md`, with `AGENTS.md` and the living text renamed. Journals, the CHANGELOG's past entries, probe notes and roadmap cards are left as written.
- A `README.md` whose first line says metawoo mega and what it is, with what it needs (Go, Node, the pinned Claude Code and Codex versions), how to build and run it, and its safety posture: loopback only, no login, agents run with the operator's own permissions.
- A `LICENSE` (MIT, Michael Quigley, 2026).
- The two e2e tests take their fixtures from an environment variable and skip when it is unset; `AGENTS.md` records Michael's value.
- A GitHub Actions workflow on the house pattern that builds and tests, with no release job yet.
- An attribution line in the Codex schema README.
- **A start-up guard:** when the new data directory does not exist and `~/.local/share/roundhouse` does, `mega` refuses to start and says the workspace has not been moved yet, naming the cutover document. It never creates an empty workspace beside a full one.
- **A probe,** recorded in the capability report: copy a fixture checkout to a new path, move its Claude session store directory to the name that path maps to, and confirm a resume by exact id works from the new path with history intact.

## phase b: cutover (Michael, by hand, service stopped)

`docs/future/mega-cutover.md` holds the checklist with exact commands.

