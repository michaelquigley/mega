---
title: pi adapter
state: horizon
created: 2026-11-06
tags: [feature]
---

Add pi as a third harness: JSONL RPC over stdio from Go, extension dialogs as requests, discovery by session-directory scan, native history import from the session file. The probes under `probes/pi/` and the capability report hold the observed behavior at pi 1.1.0, including that there is no built-in permission model and that an unanswered dialog blocks `abort` but not shutdown.
