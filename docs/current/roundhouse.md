# Roundhouse as built

This describes what exists now, which is the first two stages of the first slice in `docs/future/roundhouse-work-order.md`: the environment service with projects, and Claude Code conversations launched, prompted, answered, interrupted, released for a terminal, and resumed. Codex, adoption, and attention come in later stages.

## Running it

`make` builds the UI, installs the Claude shim's pinned dependencies (`npm ci` in `shim/claude`), and installs the `roundhouse` binary into the project's `GOBIN`, stamped with the shim's location in the checkout. `roundhouse` (or `roundhouse serve`) runs the service; `roundhouse version` prints build information. `--config PATH`, `--data DIR` and `--verbose` apply to both forms.

| Item | Default | Override |
| --- | --- | --- |
| Config file | `$XDG_CONFIG_HOME/roundhouse/roundhouse.yaml` (`~/.config/...`), optional | `--config`, which must exist |
| Data directory | `$XDG_DATA_HOME/roundhouse/` (`~/.local/share/...`) | `data` in the config, then `--data` |
| Listener | `127.0.0.1:8470` | `listen` in the config |
| Node | `node` on `PATH` | `claude.node` |
| Claude shim | `shim/claude` in the checkout `make` ran in | `claude.shim` |
| Claude Code | `~/.local/share/claude/versions/2.1.318` | `claude.executable` |

```yaml
listen: 127.0.0.1:8470
data: ~/.local/share/roundhouse
claude:
  node: node
  shim: ~/Repos/q/research/roundhouse/shim/claude
  executable: ~/.local/share/claude/versions/2.1.318
```

The executable defaults to 2.1.318 because the shim pins Agent SDK 0.3.318, which was matched against that build; `claude` on `PATH` may be newer.

`listen` must be a loopback address; the service refuses to start otherwise, because this slice has no login. The service binds its port before reading any data, so a second instance on the same port exits without writing. It then takes an exclusive hold on `<data>/lock` before loading the registry, so a second instance on another port that shares the data directory also exits, and the directory has one writer. The hold is released by the kernel when the process exits; the file itself stays.

## Data

The data directory holds `registry.yaml`, the authority for projects and conversations, and `conversations/<id>/events.jsonl`, one append-only event history per conversation. Only the service writes either. Both are private to the operator (mode `0600`, directories `0700`).

The registry is plain YAML, readable with the service stopped. It is written to a temporary file, synced, and renamed, and the service's in-memory state only advances once that succeeds, so what a browser sees is what a restart would read. A registry with an unknown `version` is refused rather than guessed at. If the directory sync after the rename fails, the change was never confirmed: memory keeps the last confirmed document and reads continue, the condition is logged once at error level, nothing is announced to browsers, and every later change is refused as `unavailable` with `registry durability uncertain after a failed directory sync, restart required`. The file may hold that one unconfirmed change until a restart reads whatever reached the disk. A failure before the rename changes nothing and can be retried. Every directory the service creates under the data directory, including the data directory itself, has its parent synced, so a new workspace or conversation cannot vanish in a crash while its synced contents would have survived.

An event log assigns each event the next `seq`. An append returns only after the line is synced, and a failed write or sync is truncated back with the `seq` unchanged, so an event a browser has been sent survives a crash. If that truncate also fails, the log keeps serving every confirmed event, since reads never go past the confirmed length, but refuses every later append with `event log durability uncertain after a failed rollback, restart required`, logged once at error level. The leftover bytes stay on disk until a restart reopens the log, where an unterminated remainder is dropped as a torn tail. Creating a log also syncs the directory that names it. Only completed events are logged; streaming text is never written, so this costs a few syncs per turn. On open, an unterminated final line is a write torn by a crash and is dropped; any other unreadable line stops the open and leaves the file alone.

An event is `{seq, at, kind, origin, text, native_id, status, tool, tool_use_id, req, facts, raw}`. `kind` is `runtime` (with `status` starting, started, stopping, stopped, unsupervised, resuming, resumed or error; a `started` event carries the runtime's `facts`: session, model, version, permission mode, cwd), `user`, `assistant`, `tool_use`, `tool_result`, `request`, `request_resolved`, `turn_end` (`status` success, error or interrupted), or `native` (anything else the harness emits, kept in `raw`). `origin` is `managed` for what a runtime managed by the service produced, `native` for what an import read from the harness's own history, and `service` for the service's own bookkeeping. `native_id` is the harness's identity for the message an event came from; an event whose `native_id` the log already holds is never recorded again, which is how an import adds only what is missing.

## Conversations and runtimes

A conversation is bound to one native session. Launching starts a runtime for a new session, records the session id, and sends the opening prompt. Every later runtime resumes exactly that id; nothing picks a session by recency, forks one, or starts a fresh one in its place. A runtime that reports a different session than the one it was launched for is recorded as an error and stopped.

The registry records each conversation's `runtime` as the service last confirmed it: `none`, `running` (written before the process starts), or `unsupervised`. The service also reports transient states: `starting` until the runtime reports the session it holds, `stopping`, and `resuming` while it reads native history.

Every change to the displayed runtime state is a recorded `runtime` event, and the change is shown and announced only once that event is recorded. If the log cannot record it, the last confirmed state stays on screen. Whatever the change does to the process happens anyway: a release, a shutdown, or the stop of a runtime bound to the wrong session still stops the process, because a storage fault must never trap a running agent, and the next start settles what the log could not record. There are no exceptions: a launch that failed shows no runtime once its `error` event is recorded, and if the log cannot record it, the last confirmed `starting` stays on screen, the conversation stays recorded `running` so a restart settles it, and the log's refusal of later writes (`unavailable`, restart required) is what tells the operator. Operations are guarded internally, apart from what is displayed, so a start or a resume that ended unrecorded never blocks the next one, and a runtime is asked to stop at most once, however many releases or shutdowns ask.

- **Prompt** records the text as a `user` event with a service-minted uuid and passes that uuid to the harness, which records it as the message's identity (observed with Agent SDK 0.3.318), so an import recognizes the prompt. One turn at a time: a prompt while a turn is in flight is refused. A prompt to a conversation with no runtime first imports native history, then resumes the session by its id.
- **Requests** (a permission or an `AskUserQuestion`) get a service-minted id and join the conversation's pending set, which lives only in memory. The harness's own request id stays inside the adapter. An answer takes the request out of the pending set before it is delivered, so the first answer from any browser wins and every later one gets `not_pending`. An answer the harness has already settled, for example on an interrupt, also gets `not_pending`. A denial without a reason is sent as `the operator denied this in roundhouse`.
- **Interrupt** stops the active turn; the harness aborts any pending request.
- **Release** is the terminal handoff. If a turn or a request is outstanding it interrupts first, then closes input, and the conversation shows `stopping` until the process is seen to exit, when it records `none`. Claude Code 2.1.318 exits with status 1 when input closes after an interrupted turn; a requested release with no turn in flight is still recorded as `stopped`, with the status in the text. An exit nobody asked for, or one in the middle of a turn, is recorded as `error`.
- **Resume** imports native history, appending what the log lacks (for example turns made in the terminal) as `native`-origin events, records `resumed` with how many it added, and clears `unsupervised`. It starts no runtime; the next prompt does. A resume whose `resuming` event cannot be recorded is refused before anything is read.
- **Shutdown** (SIGINT or SIGTERM) releases every runtime the same way and waits up to 30 seconds for each to exit. Each that exits records `none`; any still running stays `running`.
- **Start** settles the previous run without replaying anything: every request the log shows unresolved gets `request_resolved` with outcome `lost`, and every conversation still recorded `running` gets a `runtime unsupervised` event and becomes `unsupervised`, because its runtime was never seen to exit and may still be working. Its history is not imported until the operator resumes it, and prompts are refused until then.

A project with a conversation that has a runtime cannot be removed until that runtime is released.

## The Claude shim

`shim/claude/shim.mjs` is a policy-free bridge between the service and `@anthropic-ai/claude-agent-sdk` 0.3.318, speaking one JSON object per line on stdio; its header documents the protocol. `run` starts or resumes a session with `permissionMode: 'default'` and default setting sources, so every approval reaches the operator and the checkout's instructions, hooks and permission rules load as in a terminal. Each request waits for the service's answer, and the shim confirms every answer as accepted or rejected, so an answer that lost a race with an abort is reported `not_pending` rather than as answered. A request becomes answerable only once its event is recorded. On `close` it waits for an interrupt in flight before ending input, because ending input with a request pending keeps Claude alive for minutes. `messages` wraps `getSessionMessages` and `list` wraps `listSessions`. The service runs the shim in its own process group, so a terminal interrupt aimed at the service does not cut short the orderly release.

## HTTP surface

Everything is JSON with `snake_case` keys. Errors carry `{code, message}` with `code` one of `invalid` (400), `not_found` (404), `conflict` (409), `not_pending` (409, an answer to a request that is no longer pending), `unavailable` (503, the service cannot make changes durable until it restarts), or `internal` (500, detail logged rather than returned).

| Route | Body | Result |
| --- | --- | --- |
| `GET /api/workspace` | | `{projects, conversations}` |
| `POST /api/projects` | `{name, checkout}` | 201 with the project |
| `PATCH /api/projects/{id}` | `{name}` | 200 with the project |
| `DELETE /api/projects/{id}` | | 204 |
| `POST /api/projects/{id}/conversations` | `{harness, name, prompt}` | 201 with the conversation |
| `PATCH /api/conversations/{id}` | `{name, label}` | 200 with the conversation |
| `GET /api/conversations/{id}/events?since=N` | | `{events}` after seq `N` |
| `POST /api/conversations/{id}/prompt` | `{text}` | 202 |
| `POST /api/conversations/{id}/answer` | `{request_id, decision, message}` or `{request_id, answers}` | 202 |
| `POST /api/conversations/{id}/interrupt` | | 202 |
| `POST /api/conversations/{id}/release` | | 202 |
| `POST /api/conversations/{id}/resume` | | 202 |
| `WS /api/stream` | | frames below |

A project is `{id, name, checkout, untrusted}`. `checkout` must be an absolute path to an existing directory and may be registered once; an empty `name` becomes the directory name. Removing a project takes it and its conversations out of the workspace and touches nothing else: the checkout, native histories, and stored event logs stay where they are.

A conversation is `{id, project_id, name, label, harness, native_id, mode, state, turn_active, read_seq, last_seq, output_seq, model, version, permission_mode, pending}`, where `pending` is the current pending set as `{id, kind, payload}`. A request's payload is plain data for display: `tool` and `input` (plus `title`, `description`, `decision_reason`, `blocked_path` where Claude gives them) for a permission, `questions` for a question. `harness` is `claude`; `codex` is refused as unavailable until stage 3. Accepted commands answer 202, and their effects arrive on the stream.

Every other path serves the embedded UI, falling back to `index.html`.

The stream carries `{conversation_id, event}` for each recorded event, `{conversation_id, delta}` for streaming text (never stored), `{workspace_changed: true}` after any confirmed change to the workspace (the registry, a runtime's state, a pending set), and `{heartbeat: <time>}` every 20 seconds so a page can see that a quiet stream is alive. It replays nothing: a client that reconnects re-reads confirmed state. A client that falls 256 updates behind is disconnected rather than allowed to slow the service.

Requests are refused with 403 unless their `Host` is loopback and any `Origin` matches that `Host`. The loopback bind keeps the network out; this keeps out web pages in the operator's own browser, which could otherwise reach `127.0.0.1` through DNS rebinding or a cross-site request.

## The trust notice

`untrusted` is computed from `~/.claude.json` on every read and never persisted, so a terminal visit clears it without a restart. It looks up `projects[<key>].hasTrustDialogAccepted`, where the key is the checkout's enclosing git root, or its real path outside a repository. That mirrors the check Claude Code 2.1.318 uses to gate project permission rules; it is an observation of that version, not a documented contract, and a linked worktree may read as untrusted. A missing file reads as untrusted, and an unreadable one shows no notice and logs a warning.

The service only ever reads this file. Trust is granted by the operator opening the checkout in `claude` interactively.

## The UI

A left column with a needs-you placeholder and the projects with their conversations, each marked with its runtime state (pending request, working, idle, transitioning, supervision lost). The selection lives in the URL hash. A project shows its trust notice, conversation list, rename and remove, and a dialog launches a conversation with an opening prompt. A conversation shows its harness, model, version, mode, permission mode, checkout and session id; its history with tool activity collapsed and native events hidden behind a toggle; streaming text while a turn runs; an answerable card for each request in the pending set (approvals with allow and deny, questions with their options or a written answer); interrupt, release and resume; the handoff rule with the exact `claude --resume` command; and a reply box (Ctrl+Enter) whose draft stays in the browser. Requests in the history are only history; a card exists only while the service lists the request as pending.

The UI re-reads the workspace whenever the stream connects or reports a change. Re-reads are numbered and coalesced: at most one is in flight, changes reported meanwhile get one follow-up, and only the newest response sent is applied, so an older response can never bring back a request the service no longer lists. It also fetches the selected conversation's events after the last seq it holds on every reconnect, so two browsers converge on the service's confirmed state. From the moment the stream drops until the current connection's own workspace refresh lands, runtime state on the page is labeled last known: the header adds `(last known; reconnecting)`, marks dim, and an in-flight tool reads `running when last seen`. A refresh still out when its connection dropped cannot mark a later connection up to date. `make test` builds and lints it; `npm --prefix ui run dev` serves it on port 5173 against a running service.

## Tests

`make test` runs the shim's protocol test against a fake SDK, the Go unit tests (the service against a fake harness whose request numbers restart in every runtime), and the UI lint and build. `make e2e` runs `internal/e2e` against the `claude-rt1` fixture with the real Claude Code and the operator's login: a marker turn, a question, a delayed approval, release, an import that must add nothing, and a resumed turn that recites every marker. It costs a few model turns and is run by hand.
