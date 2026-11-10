---
title: codex discovery e2e reads its own thread
state: researching
created: 2026-11-10
tags: [defect]
---

`TestCodexDiscoveryAndHistory` reads whichever thread in the `codex-rt1` fixture is newest, so any probe or manual run that leaves a half-finished thread behind (an interrupted turn with no reply) makes it fail until a complete thread is newer. Seen during the MCP approval batch; noted in `docs/current` as a known fragility.

Fix: the test creates its own short thread at the start (one marker turn through the adapter) and discovers and reads that thread by id, so it depends on nothing another run left behind. Same pattern for any e2e test that currently picks "the newest" of anything in a fixture.

Also take the one-character quoting advisory from round `0891167f47c0` (the "cannot answer" message quotes the server's text with double quotes) with this.
