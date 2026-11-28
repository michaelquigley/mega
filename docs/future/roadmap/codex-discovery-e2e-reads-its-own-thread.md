---
title: codex discovery e2e reads its own thread
state: evaluating
created: 2026-11-10
tags: [defect]
---

`TestCodexDiscoveryAndHistory` reads whichever thread in the `codex-rt1` fixture is newest, so any probe or manual run that leaves a half-finished thread behind (an interrupted turn with no reply) makes it fail until a complete thread is newer. Seen during the MCP approval batch; noted in `docs/current` as a known fragility.

Fix: the test creates its own short thread at the start (one marker turn through the adapter) and discovers and reads that thread by id, so it depends on nothing another run left behind. Same pattern for any e2e test that currently picks "the newest" of anything in a fixture.

Also take the one-character quoting advisory from round `0891167f47c0` (the "cannot answer" message quotes the server's text with double quotes) with this.

**pi's discovery test has the same fault, seen 2026-11-19.** `TestPiDiscoveryAndAdoption` adopts the newest session in `pi-rt1` and expects a prompt and a reply in it. In the `make e2e` run for `a-codex-question-is-a-message`, pi's default model on eleven refused connections, so the pi round trips skipped as designed. But they had already left a session with a prompt and only connection errors, and that session was the newest. So the discovery test failed in the same run that skipped the round trips, and it will fail again until a complete session is newer. Fixed for pi on 2026-11-20: `TestPiDiscoveryAndAdoption` now has the adapter plant a copy of `testdata/p1-and-p4.jsonl` under a fresh id in a session root of its own, and adopts that one, the pattern this card asks for; the Codex test still reads the newest thread.

## as built

Closed on 2026-11-27 by `the-tested-ranges-reach-the-builds-in-use`. `TestCodexDiscoveryAndHistory` is gone, and `TestCodexRoundTrip` discovers and adopts the thread it made, by its id, so it depends on nothing another run left behind. pi's test was fixed on 2026-11-20. The quoting advisory needs nothing: both "cannot answer" sentences in the Codex adapter quote with single quotes.
