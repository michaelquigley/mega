---
title: claude shim distribution
state: inbox
created: 2026-11-11
tags: [feature, spike]
---

A downloaded binary cannot run Claude conversations. Design and build how the Claude shim and its Agent SDK reach a machine that did not build from source, so releases can be published.

## why

Claude is driven through Anthropic's Agent SDK, a Node library, so each Claude conversation runs a small Node script that relays to the Go service. The binary finds that script through a path stamped at build time, and the script needs the pinned SDK installed beside it and a matching Claude Code. Built from a checkout, all of that is on the machine. A release built on GitHub carries a path that existed only on the build machine. Codex conversations would work; every Claude conversation would fail to start. This is what keeps the first public version source-only.

## shape to explore

- Embed the shim script in the binary and write it into the data directory at first use, so the script always matches the binary.
- A `setup` command that installs the pinned Agent SDK into the data directory with npm, verifies Node and the installed Claude Code version, and says exactly what is missing. The service refuses Claude launches with the same message until it has been run, and never installs anything by itself.
- The pinned versions shown in `version` output and in the conversation header, so a mismatch is visible.

## spike

What the SDK's licence allows for redistribution, which decides between installing on the user's machine and bundling; how far the adapter tolerates a Claude Code newer than the pinned SDK, since Claude updates itself (the stage 2 gate already crossed one version); whether the SDK's optional platform package, which bundles a Claude binary, can stand in for a separate install.

## not in this card

Replacing the shim altogether, which waits for a Go SDK or a stable control protocol, as the spec's seam census records.
