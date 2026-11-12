---
title: agents, roles and lifecycles in a project
state: horizon
created: 2026-11-12
tags: [epic, spike]
---

Work out, as a design conversation before any building, what the agents in a project are to one another: which roles exist, when each begins and ends, what passes between them, and what mega should know about any of it.

## why

Michael, 2026-11-12: a hand-off between conversations is probably bigger than a mechanism to pass work back and forth, and wants a wider conversation about the relationships between a project's agents and their lifecycles. The spec leaves the same question open: roles are optional labels, a project may gather conversations into a provisional work group, and the grouping interactions are "still to be worked out".

## what the last week showed

Evidence from building mega through mega, for whoever holds the design conversation:

- Two long-lived conversations did most of the work: a planning agent that wrote cards and prompts and reviewed returns, and implementers that built to them. The loop was card, prompt, build, review to clean, return, follow-up, commit.
- Implementers were replaced often and arrived cold; the planning conversation persisted for the whole week and carried the continuity. Their lifetimes are different in kind.
- Almost everything that moved between them was one of four things: a prompt, a return, a decision request, a decision. Michael carried each by hand.
- The brokers sat inside the implementer's turn: terminus rounds ran and were reported in prose, and their verdicts reached the planning agent second-hand.
- Short-lived helpers appeared and vanished inside a turn: subagents, one-off triage instances, a reviewer.
- The roadmap card was the unit the whole loop turned on, and it lived in ranger, outside mega.

## questions for the conversation

- Is a role a property of a conversation, of a position in a project, or only a label the operator reads?
- Does a project have a shape, such as one planner and its implementers, or only a set of conversations?
- What is the lifecycle of each kind: when it starts, what ends it, what it hands on when it does?
- Which of the things that pass between agents should mega carry, and which stay the operator's to move? The hub principle stands: mega carries, it never decides.
- How do the pipeline's phases in the grimoire, the roles in `agents/agent-roles`, and a mercurius or terminus review show up here, if at all?

## not this card

The interim "send to" mechanism (`hand-off-between-conversations`); building anything before the conversation has been had.
