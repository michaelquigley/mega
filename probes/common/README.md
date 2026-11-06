# Common reconnect shim

`relay.mjs` is the in-memory owner-side relay used by the reconnect tests: the probe (owner) keeps the harness connection, records every event in a ring buffer, tracks pending requests, and serves clients over a unix socket. `client.mjs` is the browser-equivalent client: it connects, receives a snapshot (state plus still-pending requests) and a replay of events after `--since`, and can answer one request, submit a prompt, or drop the connection.

Everything the relay provides (replay, pending-request snapshot, first-answer-wins) is shim behavior, not a harness guarantee, and it is lost if the owner process dies.

```
node client.mjs --socket /path/relay.sock [--since N] [--watch-ms N] [--disconnect-after-ms N] \
  [--answer-json '{"decision":"allow"}' [--answer-id ID] [--answer-delay-ms N]] [--prompt TEXT] [--close]
```

Each harness probe maps relay answers to its own protocol; see the probe READMEs for the `result` shapes.
