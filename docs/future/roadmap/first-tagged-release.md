---
title: first tagged release
state: researching
tags: [story, spike]
created: 2026-11-11
---

Mega gets its first version: a tagged release that `push` delivers to Michael's own hosts, and that someone else can install without a checkout that has to stay where it was built.

## why

Michael, 2026-11-11, when this card was called "mega on twentytwo": "the other development host... we need to get `mega` deployed there. `mega` is deployed on twentytwo currently by just building out of the repo on that host. we want to get this working as a `push`-delivered artifact."

A week on, the same need has three more reasons:

- Mega runs on three of his machines, each built by hand from a checkout, each at whatever commit was pulled last.
- External operators are building it from source, and one has sent a pull request.
- Every build still calls itself `v0.0.x`, a developer build. The version panel exists to show a real one, and `version-metadata-in-web-ui` waits in evaluating for exactly that.

## what it takes

- **A first tag,** with everything under Unreleased in `CHANGELOG.md` becoming that version's entry.
- **A release `push` can deliver.** CI already builds a stamped linux/amd64 binary and checks its stamp; it publishes nothing.
- **The binary finding the Claude bridge by itself.** Today the binary is stamped with the path of the checkout it was built in and runs `shim/claude` from there, with the pinned Agent SDK installed beside it. A binary built on GitHub carries a path that existed only on the build machine: Codex and pi conversations would work, and every Claude conversation would fail to start. This is what has kept every build source-only.

## the bridge, folded in from "claude shim distribution"

That card was written on 2026-11-11 and joined this one on 2026-11-18, since neither can finish without the other.

- **Embed the bridge's script in the binary** and write it into the data directory at first use, so the script always matches the binary.
- **A `setup` command** that installs the pinned Agent SDK into the data directory with npm, checks Node, and says exactly what is missing. The service refuses Claude launches with the same message until it has been run, and never installs anything by itself.
- **Already done:** the versions mega was tested with, and the ones it finds, show in `mega version`, the version panel and the first page.

## spike

What the Agent SDK's licence allows for redistribution, which decides between installing on the operator's machine and bundling. Whether the SDK's optional platform package, which bundles a Claude binary, can stand in for a separate install. One question from the old card is answered: a Claude Code newer than the pinned SDK works, across the tested range.

## not in this card

Replacing the bridge altogether, which waits for a Go SDK or a stable control protocol, as the spec's seam census records. Prebuilt binaries for macOS, which wait on `mega-on-macos-and-windows`.

## from the review of 2026-11-23

Two things lapse at the first tagged release, and come out with it:

- **Ruling 2 of `docs/future/simplify-the-core-inventory.md`,** that a log written by an older build is read as the current build reads it. From the first release, an older format is owed a conversion or a rule.
- **The paragraph that states it in the canon,** at the end of the `boundary` section of `honest-runtime-state` and `bookkeeping-never-creates-attention` in terminus-canon. Michael, 2026-11-23: "we'll clean it up once we're past that part of the journey."

