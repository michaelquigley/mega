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

## what the first week of daily use showed

Michael, 2026-11-13, after a week on two hosts: mega "has removed a couple of different classes of bottlenecks in my own work", and what remains is "agent organization and lifecycle work that needs to happen in mega, and the structures around the agents... things like the grimoire/lore, and ranger need to continue to evolve to keep removing friction."

One concrete case from the same day. With a demo a week out and two independent pieces of work ready, an adapter and a round of polish, he chose to run them one after the other: "parallelism is one of the things mega needs to grow to help with." Two implementers on one repository meant coordination that fell on him: who has which files, whose review is running, what order things land in. Mega could show both conversations and did nothing to make running them together safe or legible.

## questions for the conversation

- Is a role a property of a conversation, of a position in a project, or only a label the operator reads?
- Does a project have a shape, such as one planner and its implementers, or only a set of conversations?
- What is the lifecycle of each kind: when it starts, what ends it, what it hands on when it does?
- Which of the things that pass between agents should mega carry, and which stay the operator's to move? The hub principle stands: mega carries, it never decides.
- How do the pipeline's phases in the grimoire, the roles in `agents/agent-roles`, and a mercurius or terminus review show up here, if at all?

## not this card

The interim "send to" mechanism (`hand-off-between-conversations`); building anything before the conversation has been had.
