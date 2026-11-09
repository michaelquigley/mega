# Roundhouse: first-slice work order

Work order for the first slice of `docs/future/roundhouse.md`, drafted 2026-11-06 against the probes under `probes/` and the observed behavior in `docs/research/harness-capabilities.md` (Claude Code 2.1.318 with Agent SDK 0.3.318, codex-cli 0.166.0). The repository holds no product code yet; this is genesis, and the patterns are borrowed from siblings (`pane` and `reckon` for the Go binary shape and the embedded UI, the probes for the harness protocols).

The objective is narrow: an environment service running on `fortyfive`, reached on localhost, in which Michael can register projects from existing checkouts, launch and adopt Claude and Codex conversations, talk to them, answer their questions and approvals, see which ones need him, and hand one to a terminal and back. It is built to be lived in, then refined. Anything the spec defers stays deferred here.

## Decisions made in this work order

These are mechanism choices the spec left to planning. Each is the cheapest thing that keeps the spec's invariants; the refinement candidate is named where there is one.

- **One binary, `roundhouse`**, cobra root with `serve` (default) and `version`. Go module `git.hq.quigley.com/research/roundhouse`. Both the name and the path are provisional: the product name is expected to evolve, and the repository moves to the `products/` tree when it does, so nothing should bake the name into data paths or identifiers that would be painful to rename (the XDG directory names and the binary name are the known exceptions and are a single-constant rename). `df/dl` for logging, `df/dd` for YAML and JSON.
- **XDG directories.** Config at `$XDG_CONFIG_HOME/roundhouse/roundhouse.yaml` (default `~/.config/roundhouse/roundhouse.yaml`, overridable with `--config`); data under `$XDG_DATA_HOME/roundhouse/` (default `~/.local/share/roundhouse/`, overridable with `--data`). The data directory holds `registry.yaml` (the authority: projects and conversations) and `conversations/<id>/events.jsonl` (the event history, one append-only file per conversation). The registry is written atomically (temp file and rename) and only by the service. No database; SQLite is the refinement if queries outgrow a file scan, and the registry stays a readable file regardless.
- **Durability and exclusive ownership of the data directory** (settled by the stage 1 canon review, five rounds, all findings under `single-authority`). The port bind only keeps same-port instances apart, so after it and before the registry loads, `serve` takes a non-blocking exclusive `flock` on `<data>/lock`; a second instance that shares the data directory from another port exits with a clear message, and the lock file is never deleted (the kernel drops the hold on exit). Every store confirms only after the bytes are durable: the registry syncs the temp file and then the directory after the rename, the event log syncs each append and syncs the directory when it creates a new log, and a small `internal/durable` package holds `SyncDir` and a `MkdirAll` that syncs the parent of every directory it creates, used by the registry, the event log and the lock. In-memory state moves forward only after that confirmation. When a store cannot confirm and cannot undo (a registry directory sync that fails after the rename; an event-log append whose rollback truncate fails), it enters a degraded mode: memory keeps the last confirmed state, every later write to that store is refused with a sticky error naming the restart (logged once; the API returns it as `unavailable`, 503), reads keep serving only the confirmed state, and nothing is broadcast. One unconfirmed change may sit on disk until the service is restarted and reads whatever survived. Refusing reads in that window was considered and declined. The canon's `single-authority` boundary records this degraded mode as intended. Stage 2 generalized the same rule to the service's in-memory view: runtime state and any announcement to browsers move only after the event that justifies them is recorded, so a failed append leaves the screen on its last confirmed state and announces nothing. Every runtime state change goes through one service transition helper that records first and applies the state only on success; there is no forced path, not even for a failed launch (an unsaved launch failure leaves `starting` showing and the registry `running`, so the restart settles it `unsupervised`). Process actions are independent of that: a release, a shutdown or a wrong-session stop still stops the process when its event cannot be saved, since a storage fault must never trap a running agent, and the restart settles whatever the log could not record.
- **HTTP on `127.0.0.1:8470`**, hand-written JSON endpoints under `/api/`, one WebSocket at `/api/stream` for live events, and the UI embedded with `go:embed all:dist` behind everything else. A `Host` and `Origin` guard sits in front of everything (added in stage 1): the loopback bind keeps the network out, but a web page in the operator's own browser can reach `127.0.0.1` through DNS rebinding or a cross-site request, and a service that launches agents and answers approvals with no login cannot allow that. The stream also carries `{workspace_changed}` after any confirmed registry change and `{heartbeat}` on a timer, since page script cannot see WebSocket pings. Contract-first generation (ogen plus openapi-typescript, as in reckon) is the refinement once the surface settles; for the first slice a dozen handlers and a shared TypeScript type file are cheaper than the toolchain.
- **UI: React, Vite, TypeScript**, no state library, no router beyond a selected-conversation id in the URL hash. Plain CSS. Keyboard: one shortcut for next-waiting.
- **Claude through a shim**: `shim/claude/`, a Node package pinning `@anthropic-ai/claude-agent-sdk` 0.3.318, spawned by the service per runtime, speaking JSONL on stdio. It is `probes/claude/probe.mjs` with the scenario machinery removed and the relay protocol promoted to stdio. No policy lives in it.
- **Codex through a private `codex app-server` per conversation**, driven over stdio JSON-RPC from Go, exactly as `probes/codex/probe.mjs` does. One child per conversation keeps lifecycle identical to Claude (a runtime is a child process) and keeps a crash local. One shared app-server per environment, which also enables `codex --remote` attach, is the refinement.
- **Permissions inherited, never weakened.** Claude launches with `permissionMode: 'default'` and default `settingSources`, so project `CLAUDE.md`, hooks and MCP servers load and every approval reaches the UI. Codex launches with no `approvalPolicy` or `sandbox` override, inheriting the operator's config (`on-request`, read-only sandbox as observed). The service never sets `hasTrustDialogAccepted`; it reads `~/.claude.json` and shows a notice on the project when the checkout is untrusted, since project permission rules are ignored until Michael opens it interactively once.
- **Runtime lifecycle, long-lived mode only.** A conversation's runtime starts at launch (or at the first prompt after adoption, after a clean restart, or after a deliberate resume) and stays alive until Michael releases it, the service stops, or the harness exits on its own. Release is the terminal-handoff gesture: interrupt if a turn is active, then close input and wait for exit. Service shutdown releases every runtime the same way. There is no per-turn mode and no suspend operation in this slice; a service restart is the poor man's suspend, and the spec's explicit suspension builds on the same release path later.
- **Clean shutdown versus unclean restart.** The registry records each conversation's runtime state as the service last confirmed it: `none` (no runtime, or a release that the service saw exit) or `running`. An orderly `serve` shutdown releases every runtime and writes `none` for each as it exits. On start, any conversation still marked `running` was not seen to exit, and the capability report shows that means lost supervision rather than a stop: a Claude CLI can keep working, run another turn and persist it after its owner dies. Those conversations get a `runtime` event with state `unsupervised` and are shown as "supervision lost, execution uncertain", with their pending requests resolved `lost`. They stay that way until Michael deliberately resumes them, which first imports native history (so anything persisted after the restart appears) and only then starts a runtime, under the same close-before-handoff rule as a terminal return. The service does not try to find or kill the orphaned process; detecting foreign live processes is outside this slice per the spec.
- **Pending requests live in memory only.** They die with the runtime by definition, so on restart the service writes a `request_resolved` event with outcome `lost` for anything that was pending at the last write, and the UI never shows a stale approval as actionable. Late answers get an explicit `not_pending` error from the service; the harnesses drop them silently, so the service is the only place the rejection can be visible. Request ids are minted by the service and are unique across runtime incarnations; the native id (Claude's `requestId`, Codex's per-connection server request number, which restarts at 0 on every new app-server) stays inside the adapter, so a stale card from before a release or restart can never match a new request that reused the native number.
- **Native history import.** After a release and terminal visit, or after adoption, the conversation's cache is behind its native transcript. The service re-reads the native history (Claude: the shim's `messages` command, which wraps `getSessionMessages`; Codex: `thread/read` with `includeTurns`) and appends what the cache lacks as events marked `origin: native`. It runs on adoption, on resume after release, and on resume after an unclean restart; it is not run blindly at service start, because an orphaned runtime may still be writing. It never rewrites existing events.

## Layout

```text
cmd/roundhouse/          cobra root; serve, version
internal/config/         roundhouse.yaml via dd; defaults; path expansion
internal/model/          Project, Conversation, Event, Request, Attention; no I/O, no wire knowledge
internal/registry/       registry.yaml load/save, ids, atomic synced write, data-directory lock
internal/events/         per-conversation JSONL log: synced append, tail, replay since seq
internal/durable/        SyncDir and a parent-syncing MkdirAll shared by registry, events and the lock
internal/harness/        Runtime and Discoverer interfaces, derived from the two adapters below
internal/harness/claude/ shim process management, JSONL protocol, native history import, discovery
internal/harness/codex/  app-server child, JSON-RPC framing, event mapping, steer, history import, discovery
internal/service/        the authority: registry + events + runtimes + pending set + attention + reading state
internal/server/         HTTP JSON API, WebSocket fan-out, embedded UI
shim/claude/             Node package: shim.mjs, package.json (SDK pinned), README
ui/                      Vite + React + TypeScript; embed.go and embedStub.go as in reckon
probes/                  unchanged; evidence and manual regression checks
```

Dependencies point inward: `server` depends on `service`, `service` on `harness`, `registry`, `events`, `model`; adapters depend on `model` and `events` only; `registry`, `events` and the lock depend on `durable`. `model` depends on nothing. That is the model/transport seam from the census made structural: nothing in `model` knows JSON-RPC, SDK message shapes, or HTTP.

Makefile per the house convention: `build` runs the UI build then `go install ./...`; `test` runs UI lint and build, `go test ./... -count=1`, `go vet ./...`; `clean` removes `ui/dist`, `ui/node_modules`, `shim/claude/node_modules` and the project's `GOBIN` contents. An `e2e` target runs the harness round trips against the disposable fixtures and is not part of `test`, because it needs the real harnesses and the operator's logins.

## The model

```go
type Project struct {
    ID, Name, Checkout string
    Untrusted bool            // claude trust notice, recomputed at load
}

type Conversation struct {
    ID, ProjectID, Name, Label string
    Harness  string            // "claude" | "codex"
    NativeID string            // claude session id | codex thread id
    Mode     string            // "long-lived" (the only value in this slice)
    Runtime  string            // "none" | "running" | "unsupervised", as the service last confirmed it
    ReadSeq  int64             // reading position
    LastSeq  int64             // last event seq, for replay; read from the log, never persisted, so
                               // recording an event never rewrites the registry
    OutputSeq int64            // seq of the latest agent output in a completed turn; drives unread
}

type Event struct {
    Seq    int64
    At     time.Time
    Kind   string              // see below
    Origin string              // "managed" | "native" | "service"
    Text   string              // user/assistant text, tool summary, request title
    Req    *Request            // for request / request_resolved
    NativeID string            // the harness's own message identity; import idempotence keys on it
    Status string              // runtime state or turn_end status
    Tool, ToolUseID string     // for tool_use / tool_result
    Facts  *RuntimeFacts       // on runtime started: session id, model, harness version
    Raw    any                 // harness-native payload, dd +opaque; kept for fidelity, shown collapsed
}

type Request struct {
    ID      string             // minted by the service, unique across runtime incarnations; the adapter
                               // maps it to the native id (claude requestId, codex request number) privately
    Kind    string             // "question" | "permission"
    Payload any                // options, tool name and input, suggestions; dd +opaque; rendered by the UI
    Outcome string             // "" while pending; "answered" | "aborted" | "lost"
}
```

Event kinds: `runtime` (started, stopping, stopped, unsupervised, error, with the native identity and init facts; the service also records `starting`, `resuming` and `resumed` (with the count of imported messages) so every transition has an event, and reports the transient `stopping`), `user` (prompt), `assistant` (text, final per turn; deltas are not stored), `tool_use`, `tool_result`, `request`, `request_resolved`, `turn_end` (status success, error, interrupted), `native` (anything else, passthrough). Deltas stream over the WebSocket for liveness but are never written to the log.

Attention is computed, never stored: `request` if the conversation has a pending request; else `running` if a turn is in flight; else `unread` if `OutputSeq` is past `ReadSeq`; else `idle`. `OutputSeq` advances only on agent output (`assistant`, `tool_use`, `tool_result`, and `native` events from the harness) once its turn has ended; `runtime`, `request_resolved`, and other lifecycle or recovery bookkeeping never create attention on their own, so reading a conversation and then releasing it, or restarting the service, leaves it `idle`. A runtime error or exit without a `turn_end` yields `error`. The needs-you list is every conversation in `request` or `unread`, ordered requests first, then by the time of the triggering event, oldest first; the next-waiting shortcut walks that list.

## The Claude shim protocol

The service spawns `node shim.mjs run --cwd DIR (--session-id UUID | --resume UUID) --exec PATH` with the executable path from config (default the installed `~/.local/share/claude/versions/2.1.318`, because the SDK does not bundle a Linux binary at this version; `npm ci` does pull an optional `claude-agent-sdk-linux-x64` package carrying a binary of the same size, which the shim ignores). The shim directory is stamped into the binary by the Makefile; `claude.node`, `claude.shim` and `claude.executable` in the config override it. The shim runs in its own process group. One JSON object per line in each direction.

Shim to service:

```text
{"type":"event","msg":<SDK message verbatim>}
{"type":"request","id":"<requestId>","kind":"permission"|"question","tool":"...","input":{...},"suggestions":[...],"toolUseId":"..."}
{"type":"request_resolved","id":"...","outcome":"aborted"}      // canUseTool signal aborted
{"type":"answer_accepted","id":"..."}                                     // the answer reached the waiting request
{"type":"answer_rejected","id":"..."}                                     // nothing was waiting under that id (lost a race with an abort)
{"type":"interrupted","response":{...}}
{"type":"exit","code":0}
```

Service to shim:

```text
{"type":"prompt","uuid":"<service-minted message uuid>","text":"..."}   // passed to the SDK as the user message uuid
{"type":"answer","id":"...","decision":"allow"|"deny","message":"..."}        // permission
{"type":"answer","id":"...","answers":{"<question>":"<label>"}}             // AskUserQuestion
{"type":"interrupt"}
{"type":"close"}                                                            // end input; shim exits when idle
```

Ids in shim frames are the SDK's own `requestId`; the adapter maps them to and from service-minted request ids. The adapter maps `system/init` to a `runtime started` event, `assistant` messages to `assistant` and `tool_use` events, `user` tool results to `tool_result`, `result` to `turn_end`, and everything else to `native`. `AskUserQuestion` arrives as a `canUseTool` call and is a `question` request; every other `canUseTool` is a `permission` request. Release sends `interrupt` if a turn is active, then `close`, and waits for `exit`; the capability report's 294-second hang only occurs when input is closed with a request still pending, which this ordering avoids. Backgrounded tools keep the CLI alive after `close` until they finish; release has no deadline, since the service owns the process, and reports `stopping` until exit; service shutdown waits 30 seconds and leaves stragglers recorded `running`, which come back `unsupervised`. The service records an answer only once the shim acknowledges it with `answer_accepted`; `answer_rejected` means the request was aborted first and the answer is recorded as such. A runtime whose `system/init` reports a session other than the conversation's native id is recorded as an error and stopped: resuming anything but the exact identity is never acceptable.

Discovery and import: `shim.mjs list --cwd DIR` wraps `listSessions`, `shim.mjs messages --session ID --cwd DIR` wraps `getSessionMessages`. Both run without a runtime.

## The Codex adapter

`codex app-server` as a child per conversation, stdio JSON-RPC with the framing `probes/codex/probe.mjs` uses: `initialize` with `clientInfo {name: "roundhouse"}` and `capabilities {experimentalApi: true}`, then `initialized`, then `thread/start {cwd}` or `thread/resume {threadId, cwd}`. Types are hand-written Go structs for the subset used, taken from `codex app-server generate-json-schema` for 0.166.0 and checked in under `internal/harness/codex/schema/` with the generating command recorded.

Mapping: `thread/start` or `thread/resume` response to `runtime started` (with model, approval policy, sandbox, instruction sources); `item/completed` with `agentMessage` to `assistant`, with `commandExecution` or `fileChange` to `tool_use` plus `tool_result`; `turn/completed` to `turn_end` with its status; `thread/status/changed` kept as the in-flight signal. Server requests `item/commandExecution/requestApproval`, `item/fileChange/requestApproval` and `item/permissions/requestApproval` become `permission` requests, answered with `{decision}`; `serverRequest/resolved` becomes `request_resolved`. An `agentMessage` item carrying `questions[]` is classified as a `question` request. It is synthetic: Codex emits no resolution signal for it, so the adapter owns its lifecycle. A successful answer resolves it, delivered through `turn/steer {threadId, expectedTurnId, input}` while the turn is active. Interrupt, release, a turn ending `failed` or `interrupted`, and runtime loss all resolve it `aborted`, so a late answer gets `not_pending` and never starts new work. Only a turn that ends `completed` with the question still unanswered keeps it pending, and then the answer goes through `turn/start` as the next prompt. `item/tool/requestUserInput` is handled if it ever fires but is not expected in default mode at this version.

Interrupt is `turn/interrupt`. Release is interrupt if active, then close stdin, then wait for exit; the capability report shows a `codex-linux-sandbox` child can survive an interrupt, so release reports `stopping` until a bounded process check comes back clean and otherwise surfaces the surviving pid in the runtime event rather than claiming a clean stop.

Discovery is `thread/list {cwd}`; import is `thread/read {threadId, includeTurns: true}`. Both work from a short-lived app-server with no thread loaded.

## The API

```text
GET    /api/workspace                           projects, conversations, attention per conversation, needs-you order
POST   /api/projects                            {name, checkout}  validates the path exists here; computes trust notice
PATCH  /api/projects/{id}                       {name}
DELETE /api/projects/{id}                       removes from the workspace only; never touches checkout or native history; 409 while any conversation has a runtime
GET    /api/projects/{id}/discover?harness=     native sessions in the checkout not already adopted
POST   /api/projects/{id}/conversations         {harness, name?, prompt}  launches and sends the opening prompt
POST   /api/projects/{id}/adopt                 {harness, native_id, name?}  imports history; no runtime until the first prompt
GET    /api/conversations/{id}/events?since=    event replay from seq
PATCH  /api/conversations/{id}                  {name, label}
POST   /api/conversations/{id}/prompt           {text}  starts a runtime if none (imports native history first, then resumes by exact id); 409 while a turn is in flight (one turn at a time, no queue) or while unsupervised until resumed
POST   /api/conversations/{id}/answer           {request_id, decision | answers}  409 with code not_pending when stale
POST   /api/conversations/{id}/interrupt
POST   /api/conversations/{id}/release          terminal handoff; runtime stops; conversation stays
POST   /api/conversations/{id}/resume           after the terminal or an unclean restart: imports native history, clears unsupervised, no runtime yet
POST   /api/conversations/{id}/read             {seq}  reading position
WS     /api/stream                              {conversation_id, event} for stored events, {conversation_id, delta} for streaming text, {workspace_changed} after any confirmed registry change or in-memory change (runtime state, turn flag, pending set), {heartbeat} on a timer
```

Every write goes through `service`, which is the one writer of the registry and the logs; `server` holds no state. The UI treats `workspace_changed` as a hint, never as data: refreshes are numbered and coalesced (one fetch in flight, one follow-up if more frames arrive meanwhile, only the newest response applied, and only one belonging to the current connection), so a slow response can never resurrect a resolved request as answerable. A store in its degraded mode (see the durability decision) answers writes with `unavailable` (503); stage 2 maps the event log's error the same way the registry's is mapped. Two browsers both receive the stream and both read the same confirmed state.

## The UI

Left column: the needs-you list at the top, then projects as a stable tree with their conversations, each conversation showing a small attention mark. Right: the selected conversation's header (name, label, harness, model, environment and checkout, runtime state), its events rendered as messages with tool activity collapsed by default and requests rendered as answerable cards while pending and as history once resolved, and a reply box. Dialogs for registering a project, launching a conversation (harness and opening prompt), and adopting a native session (a list from discover with first prompt and time). Buttons for interrupt, release and resume, with the handoff rule stated inline on release. The next-waiting shortcut moves selection; opening a conversation posts `read` with its last seq.

No side-by-side, no themes, no mobile. The layout sketch in the spec is the target; exact dimensions are whatever reads well on Michael's desktop.

## Stages

Each stage is gated by terminus to `clean`, then by Michael; each ends with its journal entry and `docs/current/` catching up.

**Stage 1: skeleton.** Module, Makefile, cobra, config, XDG directories, registry, event log, `model`, HTTP server with the embedded UI shell, WebSocket fan-out with a heartbeat, project registration and listing end to end, trust notice. `CHANGELOG.md` with its `## Unreleased` slot, `docs/current/roundhouse.md` started. Gate: `make test` green; a project registered against the Claude fixture appears in the UI and survives a restart.

**Stage 2: Claude.** The shim, the adapter, launch, prompt, requests (permission and question), interrupt, release, resume with history import, restart recovery (clean shutdown leaves `none`; an unclean restart leaves `unsupervised` with pending lost; roster intact either way), the conversation surface. Gate: by hand against the `claude-rt1` fixture, the C1 to C6 round trip from the capability report: marker turn, approval with a delayed answer, `AskUserQuestion`, release, terminal turn by Michael, resume showing the terminal turn, next prompt continuing the same session id.

**Stage 3: Codex and adoption.** The Codex adapter, approvals, the async question classified and steered, interrupt and release with the sandbox-child check, discovery and adoption for both harnesses with history import. Gate: the X1 to X7 round trip against `codex-rt1`, plus adopting a session Michael created in each native terminal.

**Stage 4: attention.** Attention computation, needs-you list, next-waiting shortcut, reading position, unread marks, naming and labels. Gate: the circuit scenario from the spec with two projects and three conversations, including that opening a conversation never resolves a request.

After stage 4 Michael lives in it, and the refinement rounds and the roadmap items take over.

## Tests

Unit tests in each package. `registry` round trips, atomic writes, memory staying on the last confirmed document when the post-rename directory sync fails (forced through a package-private function variable), and the data-directory lock refusing a second holder; `events` append, tail and replay since, a failed append rolled back (forced with a lowered file-size limit), and the degraded mode when the rollback itself fails (a read-only descriptor with planted bytes); `service` attention computation (including read-then-release and read-then-restart staying `idle`) and the restart recovery rules (pending becomes lost; a conversation released before shutdown comes back `none` while one still `running` at the last write comes back `unsupervised`; resume after `unsupervised` imports before starting a runtime; reading positions survive); the Claude adapter's mapping from recorded SDK messages to events, using small sanitized excerpts from the fixture sessions as testdata (the fixture logs are from disposable sessions, but keep the excerpts short and marker-only); the Codex adapter's JSON-RPC framing and mapping with hand-written frames, including the `questions[]` classification, its synthetic lifecycle (interrupt then a late answer yields `not_pending`; natural completion keeps it pending for `turn/start`), and `serverRequest/resolved`; and a `service` test where a request is answered after a release and a new runtime has issued a request with the same native number, asserting `not_pending` for the old id and no effect on the new request. Native history import gets an idempotence test in `service`: managed output, then a native terminal turn, then two imports; the second adds nothing, two distinct native messages with identical text stay distinct (matching is by native message identity, never by text), and the managed user prompt appears exactly once, since the service mints the prompt uuid, the shim passes it to the SDK as the user message uuid, and the adapter correlates on it when the native history comes back. A fake-runtime integration test in `server` covers browser reconnect while the service and runtime stay alive: a client drops, output arrives, a request is created, another is resolved; the reconnecting client replays the missed output from its last seq, sees exactly the still-pending request, does not see the resolved one as actionable, and nothing is resubmitted. The `service` transition tests cover an unsaved release still stopping the process exactly once with nothing announced, an unsaved wrong-session stop, an unsaved resume refused before any history is read, and an unsaved launch failure leaving `starting` with the busy flag cleared. The shim has a tiny Node test that round-trips its own protocol with a fake SDK. The UI has no test runner in this slice (roadmap: `ui-test-runner`); the refresh race is checked by a scratch Playwright script recorded in the journal. `make e2e` runs the stage gates' round trips against the fixtures and is run by hand.

## Risks and what to watch

- **Version drift.** The shim pins the SDK and the service records `claude_code_version` and the Codex version from `initialize` in the runtime event, so a mismatch is visible in the conversation header rather than silent.
- **Claude trust and settings.** Default `settingSources` is what makes project hooks and `CLAUDE.md` load; do not pass `['user']`. The trust notice is informational only.
- **Backgrounded Claude tools.** `close` does not end the CLI while a background task runs; the release path must wait and say so.
- **Codex memory.** One app-server per conversation is fine for a handful of conversations; watch it, and move to the shared server when it matters.
- **Branching.** Resuming a Claude session while a terminal holds it open forks the transcript silently. The UI states the handoff rule on release and resume; nothing in this slice detects a live foreign process, which the spec accepts.
- **Scope.** Everything beyond the four stages goes to the roadmap, not into a stage.

## Critical files

| File | Role |
| --- | --- |
| `docs/future/roundhouse.md` | the spec this order implements |
| `docs/research/harness-capabilities.md` | the observed behavior the adapters are written against |
| `probes/claude/probe.mjs`, `probes/common/relay.mjs` | the shim's starting point and its protocol's ancestor |
| `probes/codex/probe.mjs` | the Codex adapter's protocol usage, frame by frame |
| `internal/service/` | the single authority; every invariant in the census lands here or in the adapters |
| `shim/claude/shim.mjs` | the only TypeScript, and the only place SDK shapes are known |
