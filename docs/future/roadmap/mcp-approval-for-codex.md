---
title: mcp approval for codex
state: building
created: 2026-11-09
tags: [codex, requests, mcp]
---

Codex cannot use an MCP tool from roundhouse. Michael hit it running a terminus review from a Codex conversation: the turn hung, and with native events shown the log read "codex asked 'mcpServer/elicitation/request', which roundhouse cannot answer; interrupt the turn to go on". Since the implementers' own review loop runs through terminus over MCP, this blocks the ordinary workflow for Codex conversations.

## What is happening (codex-cli 0.166.0)

Codex routes MCP tool approvals through a server request the adapter does not classify: `mcpServer/elicitation/request`. The adapter handles the command, file-change and permissions approvals and `item/tool/requestUserInput`, and by design leaves any other server request unanswered and logged. Codex then waits for the answer and the turn never ends.

The request's shape, from `McpServerElicitationRequestParams` in the generated schema and the frame Michael captured:

- Always: `serverName`, `threadId`, `turnId` (nullable), `message` ("Allow the terminus MCP server to run tool \"collect_review\"?").
- An approval carries `_meta.codex_approval_kind: "mcp_tool_call"`, `_meta.tool_description`, `_meta.tool_params`, `_meta.tool_params_display[]`, and `_meta.persist: ["session", "always"]` naming the remember-this-choice options Codex's own UI offers. Its `mode` is `form` with an empty `requestedSchema`.
- Other variants exist: `form` with a real `McpElicitationSchema` (typed fields: string, number, boolean, enums), `openai/form` and `openaiForm` with a free schema, and `url` (an `elicitationId` and a URL the operator must visit).

The response, `McpServerElicitationRequestResponse`: `{action: accept | decline | cancel, content?, _meta?}`. `content` carries form answers for an accepted form; `_meta` is "optional client metadata for form-mode action handling", which is where the persist choice presumably travels. The schema does not say how; the probe must.

## Shape

- **Probe first**, under `probes/codex/`, recorded in the capability report with the version. Give `codex-rt1` a `.codex/config.toml` naming one tiny stdio MCP server kept in the fixture (an echo tool is enough), drive a turn that calls it, and record: the exact request frame; that `{action: "accept"}` with empty `content` lets the tool run and `{action: "decline"}` stops it; what `_meta` the response needs for "approve for this session" and whether "always" writes the operator's config (the full schema bundle, regenerated with `codex app-server generate-json-schema --experimental`, is the place to grep for `persist` before guessing); what `turn/interrupt` does to a pending elicitation and whether `serverRequest/resolved` follows; and one `form` elicitation with real fields if the fixture server can raise one.
- **Classify `mcp_tool_call` elicitations as `permission` requests.** Tool named `serverName/tool`, the card showing the server, the tool, its description and the display parameters, with Allow and Deny. Allow answers `accept`; Deny answers `decline` with the operator's reason if the protocol carries one. Lifecycle identical to the other Codex approvals: service-minted id, native id private to the adapter, `aborted` on interrupt, release and runtime loss, `not_pending` for a late answer.
- **Persistence is the operator's choice, never the default.** If the probe shows how "this session" travels, offer it as a second affirmative on the card. "Always" writes the operator's own Codex config and stays out until it is understood and wanted; roundhouse never remembers an approval on its own, which is the `permissions-never-weakened` rule.
- **Typed `form` elicitations become `question` requests** when the schema is primitive fields and enums, rendered generically and answered as `content`. Free-schema and `url` modes stay unanswerable for now, but the recorded message says what was asked and that interrupt is the way out, as today.
- Unknown server requests keep today's behaviour: logged, never answered on the operator's behalf.

## Checks

Adapter tests with hand-written frames for classification, both answers, abort on interrupt and the late-answer rejection; the service's existing request tests cover the rest. `make e2e` gains a Codex turn that calls the fixture's MCP tool and proves the card arrives and the tool runs after Allow. Browser check with the fake runtime for the card's content.

## Not in this card

"Always" persistence; `url` elicitations; free-schema forms; Claude MCP approvals, which already arrive through `canUseTool` and work.
