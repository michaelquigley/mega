# Roundhouse as built

This describes what exists now, which is the first stage of the first slice in `docs/future/roundhouse-work-order.md`: the environment service with projects, and nothing yet that runs a harness.

## Running it

`make` builds the UI and installs the `roundhouse` binary into the project's `GOBIN`. `roundhouse` (or `roundhouse serve`) runs the service; `roundhouse version` prints build information. `--config PATH`, `--data DIR` and `--verbose` apply to both forms.

| Item | Default | Override |
| --- | --- | --- |
| Config file | `$XDG_CONFIG_HOME/roundhouse/roundhouse.yaml` (`~/.config/...`), optional | `--config`, which must exist |
| Data directory | `$XDG_DATA_HOME/roundhouse/` (`~/.local/share/...`) | `data` in the config, then `--data` |
| Listener | `127.0.0.1:8470` | `listen` in the config |

```yaml
listen: 127.0.0.1:8470
data: ~/.local/share/roundhouse
```

`listen` must be a loopback address; the service refuses to start otherwise, because this slice has no login. The service binds its port before reading any data, so a second instance on the same port exits without writing. It then takes an exclusive hold on `<data>/lock` before loading the registry, so a second instance on another port that shares the data directory also exits, and the directory has one writer. The hold is released by the kernel when the process exits; the file itself stays.

## Data

The data directory holds `registry.yaml`, the authority for projects and conversations, and `conversations/<id>/events.jsonl`, one append-only event history per conversation. Only the service writes either. Both are private to the operator (mode `0600`, directories `0700`).

The registry is plain YAML, readable with the service stopped. It is written to a temporary file, synced, and renamed, and the service's in-memory state only advances once that succeeds, so what a browser sees is what a restart would read. A registry with an unknown `version` is refused rather than guessed at. If the directory sync after the rename fails, the change was never confirmed: memory keeps the last confirmed document and reads continue, the condition is logged once at error level, nothing is announced to browsers, and every later change is refused as `unavailable` with `registry durability uncertain after a failed directory sync, restart required`. The file may hold that one unconfirmed change until a restart reads whatever reached the disk. A failure before the rename changes nothing and can be retried. Every directory the service creates under the data directory, including the data directory itself, has its parent synced, so a new workspace or conversation cannot vanish in a crash while its synced contents would have survived.

An event log assigns each event the next `seq`. An append returns only after the line is synced, and a failed write or sync is truncated back with the `seq` unchanged, so an event a browser has been sent survives a crash. If that truncate also fails, the log keeps serving every confirmed event, since reads never go past the confirmed length, but refuses every later append with `event log durability uncertain after a failed rollback, restart required`, logged once at error level. The leftover bytes stay on disk until a restart reopens the log, where an unterminated remainder is dropped as a torn tail. Creating a log also syncs the directory that names it. Only completed events are logged; streaming text is never written, so this costs a few syncs per turn. On open, an unterminated final line is a write torn by a crash and is dropped; any other unreadable line stops the open and leaves the file alone. Nothing produces events yet.

## HTTP surface

Everything is JSON with `snake_case` keys. Errors carry `{code, message}` with `code` one of `invalid` (400), `not_found` (404), `conflict` (409), `unavailable` (503, the service cannot make changes durable until it restarts), or `internal` (500, detail logged rather than returned).

| Route | Body | Result |
| --- | --- | --- |
| `GET /api/workspace` | | `{projects, conversations}` |
| `POST /api/projects` | `{name, checkout}` | 201 with the project |
| `PATCH /api/projects/{id}` | `{name}` | 200 with the project |
| `DELETE /api/projects/{id}` | | 204 |
| `WS /api/stream` | | frames below |

A project is `{id, name, checkout, untrusted}`. `checkout` must be an absolute path to an existing directory and may be registered once; an empty `name` becomes the directory name. Removing a project takes it and its conversations out of the workspace and touches nothing else: the checkout, native histories, and stored event logs stay where they are.

Every other path serves the embedded UI, falling back to `index.html`.

The stream carries `{workspace_changed: true}` after any confirmed registry change and `{heartbeat: <time>}` every 20 seconds so a page can see that a quiet stream is alive. It replays nothing: a client that reconnects re-reads confirmed state. A client that falls 256 updates behind is disconnected rather than allowed to slow the service.

Requests are refused with 403 unless their `Host` is loopback and any `Origin` matches that `Host`. The loopback bind keeps the network out; this keeps out web pages in the operator's own browser, which could otherwise reach `127.0.0.1` through DNS rebinding or a cross-site request.

## The trust notice

`untrusted` is computed from `~/.claude.json` on every read and never persisted, so a terminal visit clears it without a restart. It looks up `projects[<key>].hasTrustDialogAccepted`, where the key is the checkout's enclosing git root, or its real path outside a repository. That mirrors the check Claude Code 2.1.318 uses to gate project permission rules; it is an observation of that version, not a documented contract, and a linked worktree may read as untrusted. A missing file reads as untrusted, and an unreadable one shows no notice and logs a warning.

The service only ever reads this file. Trust is granted by the operator opening the checkout in `claude` interactively.

## The UI

A left column with a needs-you placeholder and the project list, and a right pane showing the selected project with its trust notice, rename, and remove. A dialog registers a project. It re-reads the workspace whenever the stream connects or reports a change, so two browsers converge on the service's confirmed state. `make test` builds and lints it; `npm --prefix ui run dev` serves it on port 5173 against a running service.
