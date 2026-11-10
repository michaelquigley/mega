---
title: central application
state: horizon
created: 2026-11-06
tags: [epic]
---

The central application on `apps.hq` at `roundhouse.hq.quigley.com`: local username/password login, per-environment enrollment and revocation, environment services dialing out over persistent authenticated connections, relay of commands and state, cross-project views assembled from several environments, and a last-seen cache that is never an authority. Brings `twentytwo` in as the second environment. Transport choice (plain outbound WebSocket through nginx or a private zrok share) is decided here.
