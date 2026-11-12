---
title: version metadata in web ui
state: researching
created: 2026-11-11
tags: [enhancement]
---

we need some place in the web ui to see the output of `mega version`.

## shape

- A discreet version label in the side panel's footer, beside the connection status, and the same on the rail as a title. Pressing it opens a small panel with what `mega version` prints: the version, whether it is a developer build, the Go version and target, and where the log file is. It also names the pinned Claude SDK version the shim was built against.
- The service answers from its build information on a small read-only endpoint; nothing about it is workspace state.
- **A page older than the service says so.** The page's own code is embedded in the binary that served it, so after a rebuild and restart an open tab is running the previous build's interface against the new service. The page carries its build's identity, compares it with the service's on every workspace read, and when they differ shows "a newer mega is running; reload this page" where the connection status is. It does not reload itself.
