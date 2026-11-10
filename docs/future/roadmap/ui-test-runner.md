---
title: UI test runner
state: horizon
created: 2026-11-09
tags: [enhancement]
---

The UI has lint and a type-checked build but no tests. Add a runner (vitest is the likely fit with Vite) during the polish rounds, starting from a real case: the stage 2 workspace refresh race, where an older workspace response landing after a newer one brought a resolved request back as an answerable card. That was fixed by numbering and coalescing refreshes in `ui/src/App.tsx` and checked by hand with a scratch Playwright script that held back a workspace response (journal, 2026-11-09). A first test would drive `refresh` with a delayed fetch and assert that no stale pending set is applied.

Polish batch 1 (2026-11-09) added a second seed: a fake Claude shim that speaks the documented shim protocol and acts on prompt text, which let the batch's whole browser check run with no model turns. It lives in the implementer's scratchpad and should move into the repo (beside the shim's own protocol test, or under `internal/e2e`) as the fixture the UI tests drive.
