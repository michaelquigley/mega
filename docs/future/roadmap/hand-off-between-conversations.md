---
title: hand-off between conversations
state: researching
created: 2026-11-12
tags: [feature]
---

Let the operator send a message, a reply or a selection from one conversation to another as a prompt, without leaving mega and without the clipboard. The first step of the hub epic.

## why

Building mega through mega, Michael carried some forty implementer reports to the planning conversation by hand and carried the planning agent's prompts back. That is transport, the tax mercurius exists to remove between a design agent and a reviewer, and mega is where both ends already are.

## shape

- A "send to" control beside the copy controls on a message block, and on a fenced code block, since a prompt for another agent is usually written as one. It opens a picker of conversations, the current project's first.
- The text lands in the target conversation's reply box as a draft, with a line saying where it came from. The operator reads it, edits it if he wants, and sends it himself. Nothing is sent on his behalf, and nothing is sent to a conversation that cannot take a prompt.
- The source keeps a small row recording that the message was handed to another conversation, so the trail is visible from both sides.
- Mega carries; it never decides. No automatic forwarding, no routing rules, no agent choosing the recipient.

## not in this card

Agents addressing each other; forwarding a report automatically when a turn ends; a review round as a first-class object, which is the mercurius and terminus piece of the hub epic.
