---
title: per-turn runtime mode
state: horizon
created: 2026-11-06
tags: [feature, spike]
---

Make runtime mode a real per-conversation setting with a per-harness default and a global switch: in per-turn mode the harness process exits at turn end and the next prompt resumes by exact id, so idle conversations have no managed writer and need no release before a terminal handoff. Depends on mercurius and terminus being restartable (session state rebuilt from disk, reviewer rounds detached) before conversations that drive them can opt in; that work lives in those repositories.
