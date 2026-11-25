---
title: claude code 2.1.331 is past the tested range
state: researching
created: 2026-11-24
tags: [story]
---

Claude Code on `fortyfive` updated itself to 2.1.331 at 15:19 on 2026-11-24, and `harness.Tested` in `internal/harness/versions.go` ends at 2.1.326, so mega marks every Claude start as newer than it was tested with. Run the version probe (`probes/versions/claude.sh`) against 2.1.331, record what it saw, and widen the range if it passes.

## why

`make e2e` passed 20 of 20 on 2.1.331 that evening, with no skips, during the small batch. The round trips are not what the range claims, though: `probes/versions/README.md` says a build enters the range when the probe's steps pass on it. Until then the version note on every Claude start says the build is untested, which teaches the operator to read past it.
