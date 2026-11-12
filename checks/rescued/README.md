# Rescued browser checks

Saved from an implementer's scratch directory on 2026-11-12, after a reboot erased every earlier browser check. These are kept as written, not yet runnable from the repository: they still expect scratch paths, a scratch port and a Playwright install the caller supplies.

- `history.mjs`: the check for the recent-tail batch (first content, "show earlier", "show all", the native toggle, a streamed reply on a large conversation).
- `timing.mjs`, `bar.mjs`: the timing harness and a helper from the same batch.
- `gen/gen.py`: generates a large conversation log (about twenty thousand events) into a scratch data directory.

The roadmap card `ui-test-runner` is where these become real checks. Delete this directory when that card lands.
