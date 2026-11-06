# Roundhouse: harness integration investigation

## Assignment

Determine what Claude Code, Codex, and pi actually support for a custom conversational interface, native terminal handoff, adoption, and recovery. Produce small reproducible probes and an evidence-backed report. Do not build Roundhouse yet.

This is a research brief, not a converged product spec or implementation work order. The architecture below supplies context for the experiments; it is not permission to build a service framework.

## Why this exists

Michael works across several independent projects, typically with a design/planning agent and an implementation agent in separate terminals for each project. Much of his coordination time goes into paging between windows, finding questions, recovering context, and answering agents to keep work moving.

Roundhouse should provide one network-owned place to work directly with that gang. It is not a coordinating model through which every conversation passes. A project groups multiple conversations; agents remain identifiable by harness, machine, and checkout. The interface should distinguish unread activity from requests that actually require attention, preserve full conversational room around decisions, and leave project truth in repositories rather than create competing copies of specs and memory.

## Agreed direction

- A bespoke web workspace is an acceptable primary interface.
- Normal agent launches should happen through Roundhouse.
- Existing native sessions should be adoptable, with limitations honestly exposed.
- Sessions should be usable in native terminals and return to the workspace without being replaced by a fresh agent given a summary.
- A machine-local session service is the architectural anchor. Browser availability should not govern agent execution.
- Native harnesses remain responsible for their agent loops and semantics. Roundhouse must not become a replacement coding harness.

## Provisional suggestions, not settled requirements

- A central application groups projects and conversations and tracks reading position and attention; machine services supervise local execution and translate native protocols.
- Prefer structured harness integration over terminal scraping.
- Observe from multiple surfaces but make active input ownership explicit.
- Treat native saved-session continuity as the baseline portability candidate; use live attachment where supported.
- Distinguish switching to a terminal from fully releasing a session from management.
- Keep harness endpoints local and put cross-machine access behind Roundhouse's eventual authenticated service boundary.

These suggestions must be revised if the experiments contradict them. Native terminal switching does not automatically preserve Roundhouse supervision or observability.

## Scope and safety

Start on one machine. Record OS, installed executable versions, executable locations, SDK versions if used, and relevant configuration sources. Do not dump secrets or whole configuration files. Use the operator's existing authorized authentication without extracting credentials into fixtures. If the integration requires different billing or authentication, stop and explain it before spending money or changing configuration.

Use disposable repositories and new test sessions only. A simulated externally created session counts as an adoption test; do not experiment on Michael's active work. Do not upgrade installed harnesses or change global settings without permission. Bind any experimental listener to loopback or a local socket. Do not expose it on the LAN.

Use harmless fixtures: a small text file, a known marker in project instructions, a short read-only command, and a cancellable sleeping command where appropriate. Trigger a genuine approval with scoped test configuration rather than a destructive command. Preserve native sandbox and approval behavior. Ask Michael to perform native terminal interactions when necessary; mark unavailable tests as blocked rather than inventing a result.

## Documentation leads

These sources were inspected during design. They are leads, not proof of installed behavior; upstream documentation and repository main branches may describe newer versions. Record source dates and exact tested versions, and use version-matched schemas or source when possible.

### Claude Code

- Programmatic operation: https://code.claude.com/docs/en/headless
- SDK sessions: https://code.claude.com/docs/en/agent-sdk/sessions
- Permissions: https://code.claude.com/docs/en/agent-sdk/permissions
- User input: https://code.claude.com/docs/en/agent-sdk/user-input
- Remote Control: https://code.claude.com/docs/en/remote-control

The documentation exposes structured streaming, SDK permission callbacks, `AskUserQuestion`, persisted session IDs, and session discovery. Native CLI resumption is a candidate bridge to the terminal. Live attachment of the native terminal to our own managed SDK process is not established. Anthropic Remote Control uses Anthropic's hosted interface; its existence is not proof of a supported self-hosted attachment protocol. Check which project instructions, hooks, MCP servers, and settings load in programmatic mode; do not assume parity with interactive launch or use bare mode indiscriminately.

### Codex

- App-server reference: https://developers.openai.com/codex/app-server
- Implementation: https://github.com/openai/codex/tree/main/codex-rs/app-server

The inspected documentation exposes threads, turns, structured events, approvals, user-input requests, resumption, and interruption. It also documents `codex --remote` connecting the native terminal UI to `codex app-server`. This is a promising live-switching path, not yet a verified same-thread handoff. The inspected documentation labels WebSocket transport experimental and unsupported. Check availability in the installed version, connection lifetime, thread subscriptions, multiple-client approval routing, and authentication behavior. Do not infer that an independently started terminal process is automatically reachable through another app-server.

### pi

- RPC: https://github.com/badlogic/pi-mono/blob/main/packages/coding-agent/docs/rpc.md
- CLI: https://github.com/badlogic/pi-mono/blob/main/packages/coding-agent/docs/cli.md
- Extension UI: https://github.com/badlogic/pi-mono/blob/main/packages/coding-agent/docs/rpc-extension-ui.md

The inspected documentation exposes a long-lived JSONL RPC subprocess, session files/IDs, resumption, and extension dialogs. Some terminal-specific extension UI methods are unsupported or no-ops in RPC mode. Dialog transport is not itself a universal permission system; record the actual installed permission/extension behavior. Live native-terminal attachment is not established. RPC examples using `--no-session` are inappropriate for persistence tests. Respect stdout backpressure and distinguish accepted prompts from settled runs using the installed protocol.

## Execution order

### 1. Inventory and minimal probes

Write only enough harness-specific code to launch a persisted session, send input, consume events, respond to explicit requests, inspect identity/history, and shut down. Prefer supported APIs over undocumented wire internals. Separate probe implementations are acceptable. Do not extract a common adapter interface yet.

Record startup configuration and effective working directory. Verify a harmless marker from project instructions, and enumerate loaded tools/configuration where the harness exposes it. Keep raw experimental logs local and sanitized; retain only small useful excerpts in the report.

### 2. Claude round trip first

1. Launch a managed structured session in the test checkout; record its native session ID.
2. Exchange at least two turns with unique markers that later establish continuity.
3. Trigger and answer an explicit user question and a real permission request where supported. Record request IDs, cancellation behavior, and whether the agent actually waits.
4. Reach a safe idle boundary, shut down cleanly, and reopen that exact session in the native terminal.
5. Read the earlier history and add a uniquely marked terminal turn.
6. Stop the terminal process and reopen the exact session through the structured integration.
7. Verify the new turn, original history, working directory, instructions, model/settings, and relevant permission behavior. Record anything reloaded, reset, missing, or unverifiable.
8. Repeat the entry path with a new session first created independently in a native terminal, then adopted after an explicit handoff.

Do not use newest-session selection when an exact ID is available. Multiple conversations in one directory are normal for this practice. Do not fork unless testing a fork explicitly; an accidental fork is not a successful same-session handoff.

### 3. Codex and pi equivalents

Run the same basic round trip for each harness using its native identity and persistence mechanisms.

For Codex, additionally test the documented remote terminal UI against a managed app-server: connect to the same thread, observe from both clients, alternate input, and inspect what happens when an approval is pending. Determine whether the harness offers exclusive input control or whether Roundhouse would need to impose it. A custom terminal client is not a substitute for demonstrating the native terminal interface.

For pi, test a simple extension dialog and document any relevant extension behavior that differs between RPC and terminal modes. Do not build a general extension renderer.

### 4. Disconnection, interruption, and recovery

Run these as distinct cases, using fresh test sessions when necessary:

- Disconnect a browser-equivalent client while a local probe continues to own and drain the harness connection. Reconnect and recover new output and pending requests.
- Restart the central-client simulation while the local owner remains alive. Do not replay the prompt automatically.
- Terminate the local owner/probe. Observe whether the harness exits, continues, blocks, or becomes inaccessible. Report this separately from browser resilience.
- Interrupt an active model/tool turn and observe final state, tool side effects, and whether the next turn is accepted.
- Attempt handoff while a question or permission request is pending. Determine whether it can transfer, must be resolved first, or is cancelled and recreated.
- Resume after a controlled harness termination. Determine what persisted and what requires explicit operator recovery.
- Send a response after its request was cancelled or resolved, in an isolated test. Record native rejection or other behavior; never assume stale approvals are harmless.

Observe before retrying uncertain operations. Command acceptance, turn completion, and durable persistence are different outcomes. Do not claim exactly-once delivery from a successful happy-path exchange.

## Questions the report must answer

1. Can we build a faithful direct conversation surface without parsing terminal display output?
2. Which signals reliably mean working, explicit input required, permission required, turn ended, failed, or unknown? Keep ordinary prose questions distinct from explicit protocol requests.
3. Can a saved session cross between structured and native terminal modes in both directions without history replacement or an accidental fork?
4. What changes across that transition: configuration, tools, permissions, background tasks, queued input, extension state, or model context?
5. Can live attachment work? Under which launch conditions and version constraints?
6. Can an independently started terminal session be discovered and safely adopted? Distinguish discovery, stored-history access, cold resumption, and live control.
7. How do multiple clients interact? Who receives and can resolve a pending request?
8. Which failures are recoverable from native history, and which leave an uncertain operation or lost runtime state?
9. Does a terminal-controlled session remain observable by a machine-local service without unsupported techniques?
10. What minimum common behavior can Roundhouse honestly promise, and which differences must remain visible?

## Deliverables

- `docs/research/harness-capabilities.md`: findings, tested versions, source references, recommendations, and open questions.
- `probes/claude/`, `probes/codex/`, `probes/pi/`: minimal rerunnable probes and usage notes where needed. Record dependencies and reproducible installation instructions without modifying the operator's environment silently.
- `docs/journal/YYYY-MM-DD.md`: concise durable notes, blockers, and decisions made during execution.

For each scenario report: result (`pass`, `partial`, `fail`, or `blocked`), observed behavior, continuity preserved/lost, exact reproduction steps, and relevant sanitized event excerpts. Label documentation-only claims and inferences explicitly. A probe exiting zero is not sufficient evidence of conversation continuity.

End with a short handoff: what works, what does not, what remains uncertain, and the smallest experiments that would resolve the remaining architectural questions. Leave all changes uncommitted. Run `unfurl -i` on authored or edited markdown before handoff, including this brief and `AGENTS.md`; the originating design environment could write files but could not execute that formatter.

## Stop conditions

Stop the affected experiment and report when it requires unapproved global changes, new spending/authentication, destructive operations, a private or unsupported integration, or mutation of real sessions. Continue independent safe tests where useful. Do not silently replace an unavailable capability with a weaker one.

The investigation is complete when the core scenarios have evidence-backed outcomes, including honest blocked or unsupported results, and the remaining decisions can be discussed with Michael. Universal success is not the completion criterion.

## Deferred (and why)

- Web interface and visual design: basic integration behavior is not yet established.
- Cross-machine transport and deployment: first separate harness lifecycle problems from network problems.
- Shared adapter abstraction: derive it from observed differences rather than inventing it upfront.
- Production process supervision and crash resilience: characterize failures now; choose machinery afterward.
- Automatic question classification: explicit protocol signals and unread responses are sufficient for investigation.
- Agent-to-agent messaging, delegation, and workflow automation: not required for direct collaboration.
- Shared-worktree concurrency policy and commit UI: consequential product decisions, but not needed to test session portability.
- Changes to mercurius, terminus, or project conventions: Roundhouse should initially accommodate the practice, not redesign it.
