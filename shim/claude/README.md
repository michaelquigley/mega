# Claude shim

A policy-free bridge between the roundhouse service and the Claude Agent SDK, speaking one JSON object per line on stdio. The protocol is documented at the top of `shim.mjs`. The service spawns one `run` process per runtime, and runs `messages` and `list` without a runtime.

- `@anthropic-ai/claude-agent-sdk` is pinned to 0.3.318, matched against Claude Code 2.1.318. The service passes the executable explicitly (`claude.executable` in its config).
- `make` installs the locked dependencies (`npm ci`), and `make test` runs `shim.test.mjs`, which round-trips the protocol against a fake SDK.
- The shim decides nothing. It launches with `permissionMode: 'default'` and the default setting sources, relays every request, and waits for the service's answer.
- `fake/shim.mjs` stands in for the shim in browser checks that need no model turns. It speaks the same frames, acts on prompt text, and never runs Claude; its header says how to point a scratch service at it.
