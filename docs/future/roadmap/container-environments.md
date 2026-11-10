---
title: container environments
state: horizon
created: 2026-11-06
tags: [epic, spike]
---

A container per project as an environment: the service runs inside it, enrolls under its own identity, keeps its registry on a mounted volume, and dials out with no published ports. The real work is provisioning what a harness needs outside the checkout (Claude login and trust state, Codex login and config, pi trust and model settings, the native session stores).
