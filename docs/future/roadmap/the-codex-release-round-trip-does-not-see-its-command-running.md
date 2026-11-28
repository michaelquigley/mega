---
title: the codex release round trip does not see its command running
state: researching
created: 2026-11-27
tags: [defect]
---

`TestCodexReleaseDuringCommand` (`internal/e2e/codex_test.go`) releases a conversation while a sandboxed command is meant to be running, and checks that the release reports stopped only when the app-server's process group has emptied. It never checks that the command had started before it releases. A release that lands before the command starts would pass the test without showing what it is for.

Raised by terminus as an advisory on 2026-11-27, during the gates of `the-tested-ranges-reach-the-builds-in-use`, which did not change the test. It matters more than it did: the round trips now admit a build to the tested range, and the map in `internal/e2e` credits this one with a release during a command.

The fix is in the test alone: wait for the command's tool call in the log before the release.
