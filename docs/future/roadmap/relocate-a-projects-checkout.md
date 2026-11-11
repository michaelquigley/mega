---
title: relocate a project's checkout
state: researching
created: 2026-11-11
tags: [feature, spike]
---

Give the operator a deliberate way to tell mega that a project's checkout has moved, so the registry never has to be edited by hand. The project keeps its conversations, reading positions and star; only its path changes.

## why

The spec says moving a project is a deliberate operation, not a field edit, but no such operation exists. The cutover to mega needed a hand edit of `registry.yaml` with the service stopped, and the first attempt set the wrong path.

## what makes it more than a field

A conversation's native session is found through the checkout's path. Claude keeps sessions in a store directory named after the path, so its conversations resume from a new path only once that directory has been renamed to match (probed 2026-11-11, `probes/claude/moved-store.mjs`). Codex records the working directory on the thread and takes a new one at resume. Trust is recorded per path in both harnesses.

## shape to explore

- An operation on a project with no runtime in any of its conversations: verify the new path exists and looks like the same repository, write the registry, and report what the operator must still do by hand.
- Mega never renames a harness's own store or edits its trust; it says exactly which directory needs moving and shows the conversations as unable to resume until it has been.
- The spike: whether a moved Codex thread resumes cleanly with a new working directory, and what each harness does with trust at the new path.
