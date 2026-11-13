---
title: first run explains itself
state: inbox
created: 2026-11-13
tags: [enhancement]
---

A newcomer learns what mega needs, and which of it is missing, before they try to launch something: at start-up in the terminal, on the empty workspace's first page, and on a project's page as soon as it is registered.

## why

In the clean-machine rehearsal of 2026-11-13, mega built from the README on a scratch home with neither harness installed. It said nothing about that until the new-conversation dialog:

- **At start-up.** The terminal shows three lines, the same at both starts: mega's own `PATH`, "project environments resolve through '/bin/bash', within 10s", and "listening on 'http://127.0.0.1:18470'". Nothing about Claude Code or Codex, found or not, or what version is expected.
- **The first page.** "Register a project from an existing checkout to begin." The side panel says "No projects yet." The control it means is an unlabelled folder icon beside PROJECTS. The text does not point to it, and nothing on the page mentions harnesses.
- **The project page, just registered,** shows two things:
  - "Claude has not been trusted in this checkout, so it ignores the project's permission rules. Run claude here once and accept the trust dialog; mega will not accept it for you." This appears though Claude Code is not installed, so it cannot be followed.
  - Under Environment: "Not resolved yet: it resolves when something starts here, or on request." What is missing appears only after something resolves the environment, such as opening the dialog, which asks Claude for its models.
- **After a restart** the project's `missing` list is empty and the page says "Not resolved yet" again, though nothing changed.
- **New conversation** is another unlabelled icon, beside Adopt.

None of these is wrong; together they mean the first useful explanation arrives at the third click, inside a dialog, and for Claude not at all (`a-missing-claude-is-named`).

## shape to explore

- Resolve a project's environment and its harness check when it is registered and at start-up, in the background, so `missing` is known before anyone asks.
- One line per harness in the start-up log: found where, or missing and why, and its version once `harness-version-tolerance` gives a range.
- On the empty workspace, a short "what mega needs" block: each harness, found or missing, and where the README says to get it. It names the register control in words.
- The trust notice waits until Claude Code can run here, or says to install it first.
- The register and new-conversation controls carry a word as well as an icon while the workspace is empty, as `IconButton`'s `word` allows.
