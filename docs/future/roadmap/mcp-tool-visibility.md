---
title: mcp tool visibility
state: researching
created: 2026-11-11
tags: [feature, spike]
---

it would be really helpful to be able to see what MCP tools are bound into the agent harnesses.

## why

An agent's MCP servers decide what it can do, and whether one is connected is invisible from mega. Michael's own setup had seven configured for Claude and only three connected when the capability report was written; the rest were pending or needed him to authenticate. Today the only sign is a tool call that fails, or an agent that quietly works without a tool it should have had.

## what the harnesses offer (checked 2026-11-12)

- **Claude, SDK 0.3.318.** `mcpServerStatus()` on a running query returns each server's name, status (`connected`, `failed`, `needs-auth`, `pending`, `disabled`), the server's own name and version, an error when it failed, and its configuration. `system/init` lists the tool names and the servers with their status at the start of every turn, and announces servers added or removed. A started-but-idle session has its servers connected within about three seconds.
- **Codex, codex-cli 0.166.0.** `mcpServerStatus/list` returns each server with its tools (name, title, description), its authentication status and runtime status, and a `toolsError` when listing failed. A notification reports a server's status changing.

## shape

- **A normalized fact.** Each adapter reports the conversation's servers as plain data: name, a status from one shared small set (connected, connecting, failed, needs sign-in, disabled), an error if any, and each tool's name and one-line description. The service holds it in memory beside the runtime's other facts, never stored, gone when the runtime exits. No protocol shapes leave the adapters.
- **Shown where the operator looks.** A compact summary in the conversation header, such as "5 MCP servers, 1 needs sign-in", in the notice colour when any server is not connected. Pressing it opens the list: each server with its status, and its tools on expansion. A server that needs signing in says so and says where that is done, which is the harness's own terminal; mega does not start an OAuth flow.
- **Without a runtime** there is nothing to ask. The summary says the conversation is not running, and Start is one press away.
- **Not attention.** A failed server never enters the needs-you list. It is state the operator can see, like a background job.
- **Read only.** Reconnecting, enabling and disabling servers are offered by both harnesses and are a later card.

## spike

Recorded in the capability report with versions: whether Claude's `mcpServerStatus()` includes each server's tools or only the `system/init` tool names do; what an idle session reports before its servers finish connecting; how Codex's list behaves with no thread loaded and whether it differs per thread; the shape and timing of both harnesses' change notifications; and whether project-level servers appear only once the checkout is trusted.

## checks

Adapter tests from recorded frames for both harnesses; a service test that the fact appears with a runtime, updates on a change, and clears on exit; a browser check with the fakes for the header summary, the list, a server needing sign-in, and the stopped conversation. The fakes learn to report servers.
