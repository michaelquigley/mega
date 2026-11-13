---
title: mega on twentytwo
state: researching
tags: [story]
created: 2026-11-11
---

the other development host... we need to get `mega` deployed there.

`mega` is deployed on twentytwo currently by just building out of the repo on that host. we want to get this working as a `push`-delivered artifact.
## background

Noted by the planning agent on 2026-11-13. This card is the release story of the epic `a-coworker-can-install-mega`, with twentytwo as the proving ground. Mega has no tagged version yet; everything in `CHANGELOG.md` is under Unreleased. What it takes: a first tag, a release that `push` can deliver, and the installed binary finding the Claude shim without being stamped with the path of a checkout that must stay put. That last part overlaps `claude-shim-distribution`, whose embedded-shim idea may be the smallest way through.
