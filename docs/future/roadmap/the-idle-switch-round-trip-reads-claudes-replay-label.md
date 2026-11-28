---
title: the idle switch round trip reads Claude's replay label
state: researching
created: 2026-11-27
tags: [defect]
---

`TestClaudeModelSwitchWhileIdle` (`internal/e2e/switch_test.go`, about line 147) logs whether Claude echoed an idle model switch by looking for the native event text `user/replay` among the events the switch recorded. That label is how the Claude adapter names a native user frame it passes through, so the round trip reads the adapter's own encoding. Terminus `a549db2e5225` raised it, blocking under `protocol-confined-to-adapters`, as `idle-switch-interprets-claude-replay-label`, during the gates of `the-tested-ranges-reach-the-builds-in-use`, which did not change that line.

The fix terminus suggests: keep the idle, history and model assertions, and print the native events without interpreting their labels.
