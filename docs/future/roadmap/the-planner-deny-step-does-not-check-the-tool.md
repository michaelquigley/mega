---
title: the planner deny step does not check the tool
state: researching
created: 2026-11-27
tags: [defect]
---

The `side` suite's last-instructed steps (`checks/side.mjs`, about line 436) send Deny for the planner's pending request after asserting its kind and its command, `echo fake`, but not that its tool is `Bash`. Terminus `5fe90cc137aa` raised it, blocking under `permissions-never-weakened`, as `planner-denial-does-not-verify-tool`, during the gates of `the-tested-ranges-reach-the-builds-in-use`, which changed only that suite's tested-range line.

The fix terminus suggests is one condition: `plannerAsk.payload.tool === 'Bash'` in the existing `must` before the Deny is posted.
