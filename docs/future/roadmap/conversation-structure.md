---
title: conversation structure
state: inbox
created: 2026-11-10
tags: [enhancement, ui, conversation-view]
---

Second slice of the look-and-feel cluster, after `working-indicators`. The conversation reads as one undifferentiated column: only the operator's prompts have a background, agent replies have none, tool actions sit flush left at the same indent as speech, and everything is 8px apart. Michael wants to see which block a piece of output belongs to, with the actions set apart, and he wants it subtle: a slightly darker tone than the page, not borders and boxes.

## Shape

- **Agent replies become blocks.** Each assistant message sits on the `--panel` tone (one step off the page background: slightly darker in the light theme, slightly raised in the dark one) with the same radius and padding the operator's blocks use. No border. The streaming reply wears the same block from its first character, so nothing shifts when it lands.
- **The operator's prompts keep their own tint** (`--selected`), so the two speakers are told apart by tone alone. Prompts from outside roundhouse keep their label and the same tint.
- **Code inside a block is inset.** Fenced code and tables currently use `--panel`, which would vanish on a `--panel` block; inside a message block they take the page background with the existing hairline, so code reads as set into the block. Inline code likewise.
- **Actions are indented and quiet.** Tool rows (and native rows when shown) are indented from the left by about 20px with a hairline rule down their left edge, no background. Consecutive actions between two pieces of speech form one group with tight spacing inside it, so a run of twelve tool calls reads as one indented passage rather than twelve peers of the reply.
- **More air between blocks.** The gap between blocks goes from 8px to about 16px; rows inside an action group stay at about 4px. Runtime and turn status lines stay unboxed, mono and muted, and the dashed turn separator stays.
- **Request cards are unchanged**; they already carry their own notice colours and must stay the loudest thing in the column.

Both themes, same rules. No new colours unless the panel tone proves too close to the page to see on Michael's display, in which case one new variable a touch further from the background, not a border.

## Ride-alongs from the working-indicators slice

- **A request row counts the wait, not the turn.** The elapsed label on a conversation waiting on a request shows how long the turn has run ("asking 18s"), but on a row that is waiting for the operator the number reads as how long it has been waiting. Show the time since the oldest pending request was raised instead. If the workspace read's pending entries do not carry the time the request was recorded, add it there (the request event's own timestamp; nothing new stored). Running rows keep the turn's elapsed time.
- **One working sentence, not two.** With the activity line in place, "Working. Wait for the turn to end, or interrupt it." appears both as the reply box placeholder and beside Send. Drop the one beside Send.

## Checks

The browser check with the fake shim over a turn with speech, a run of actions, more speech, a code block and a request: computed backgrounds of an assistant block, a prompt block and a code block inside a reply; the action group's indent; the larger gap; the streamed reply matching the landed one. Screenshots, light and dark, of a busy turn for Michael, since this slice is judged by eye.

## Not in this slice

Copy per block; moving controls to the input panel; the expanded-code-survives-the-swap nit; anything in the side panel.
