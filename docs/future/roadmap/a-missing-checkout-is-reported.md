---
title: a missing checkout is reported
state: researching
created: 2026-11-11
tags: [defect]
---

A project whose checkout directory does not exist looks normal until a harness fails to launch in it. Check each project's checkout when the workspace is read and say so where the operator will see it: a notice on the project, a mark in the tree, and a launch, prompt or Start refused with "the checkout is missing: <path>" instead of the harness's own failure.

## why

During the cutover to mega on 2026-11-11, the registry's path for this project was set to `research/mega` instead of `products/mega`. Mega validated the path once, at registration, and never again, so nothing complained until Claude refused to start, and the error was Claude's. Michael triaged it with a separate agent instance. The store knew enough to explain it at once.

## shape

- Computed at read time, never stored, like the Claude trust notice: `checkout_missing` on the project in the workspace read.
- Launch, adopt, prompt and Start for a conversation in such a project are refused with a distinct message naming the path. Reads, removal and the project page keep working, so the operator can see what is registered and fix or remove it.
- Path search and discovery return nothing for it, quietly.
- A checkout that exists but is not readable is reported the same way, with the reason.
