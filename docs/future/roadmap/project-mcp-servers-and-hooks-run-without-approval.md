---
title: project mcp servers and hooks run without approval
state: researching
created: 2026-11-12
tags: [defect, spike]
---

Under mega, a Claude conversation starts every MCP server named in the checkout's `.mcp.json`, and runs the project's hooks, with no approval step and whether or not the checkout is trusted. In a terminal Claude asks once per server before starting one that a repository's `.mcp.json` defines `[docs]`; it never asks about servers in the operator's own configuration. Give the operator that approval back: a repository's MCP servers and hooks do not run under mega until he has approved their definitions, and a changed definition needs approving again.

## why

Observed in the MCP visibility spike (Claude Code 2.1.318, SDK 0.3.318, 2026-11-12): a `.mcp.json` server appeared with scope `project` and connected with its tools in a trusted and in an untrusted scratch checkout alike. The capability report already recorded that headless Claude loads project hooks "even in a folder you've never trusted". Both are Claude's documented headless behaviour, not something mega chose.

But the effect is that the same checkout is less protected under mega than in a terminal, which is what `permissions-never-weakened` exists to prevent, and it is the hazard the environment file was just guarded against: a file an agent can write, usually without asking, names a command that then runs outside any approval. An agent that adds a server to `.mcp.json`, or a hook to the project's settings, gets it started at the conversation's next runtime start. Codex does not have the gap: an untrusted checkout's project servers do not appear at all.

## how far this reaches today

Narrower than the title suggests, checked 2026-11-12. Every MCP server Michael uses is in his own configuration, user or local scope, and those start without a question in a terminal too. None of his repositories carries a `.mcp.json`, and none but the test fixture carries project hooks. So nothing he runs today behaves differently under mega. The terminal's question for `.mcp.json` servers is from Claude's documentation and has not been observed here. For hooks the terminal offers little either: in a trusted checkout a project hook runs without a separate approval, so mega is no worse there except in a checkout that was never trusted.

What remains is the write-then-run path: an agent adds a `.mcp.json` to a checkout, and under mega its server starts at the next runtime start with no question, where a terminal would ask. That is a real gap and a small one while mega is one operator on his own repositories. It grows when mega opens repositories he did not write.

## shape to explore

- **Approval bound to content, as for `.mega/environment.yaml`.** Mega reads the checkout's `.mcp.json` and the hooks in its project settings, shows the operator what they would run, and stores an approval tied to the full digest of each. Until approved, and again after any change, those servers and hooks do not run.
- **Withheld, not refused.** Unlike an unapproved activation, an unapproved server need not stop the conversation. Mega launches Claude with the unapproved project servers disabled and the unapproved hooks off, and says so on the project page and in the conversation's MCP list ("3 project servers not approved"). The operator's user-level servers are his own and are unaffected.
- **Per server.** The terminal approves project servers one at a time; the page should too.
- **Visible in the list that now exists.** Each server in the MCP list says where it comes from: the operator's own configuration, or this repository.

## spike

Recorded in the capability report with versions: which lever withholds a project server from an SDK session without losing the project's instructions and permission rules (the settings `disabledMcpjsonServers`, `enabledMcpjsonServers` and `enableAllProjectMcpServers`, the `strictMcpConfig` option, or a flag-settings overlay); whether an SDK session honours approvals the operator already gave in a terminal, which Claude records per checkout; whether project hooks can be withheld the same way; and what the terminal does on a changed `.mcp.json`.

## decision for Michael

Whether mega should add this approval at all, or leave Claude's headless behaviour as it is and only show plainly which servers and hooks came from the repository. The first matches the terminal and the environment file. The second is less work and honest, and leaves the gap open.
