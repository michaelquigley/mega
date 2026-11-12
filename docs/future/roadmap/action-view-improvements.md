---
title: action view improvements
state: researching
created: 2026-11-11
tags: [enhancement]
---

i love the compact view of actions in the conversation. keeping the collapsed and truncated makes the conversation easy to follow.

wonder if maybe expanding them should reveal more of the truncated line?

we should do a round of improvements on the action rendering.

## background

Noted by the planning agent on 2026-11-12, for when this card is worked. An action's line is shortened twice: the adapter caps the summary at 200 bytes when it records the tool call, and the row then cuts it to one line with an ellipsis. Expanding the row shows the result only. So the full command or input is never visible in the conversation; it survives only in the event's raw payload. Expanding an action should show what was invoked in full, then what came back.

"Action" and "action group" are the agreed words for these rows; see `docs/current/glossary.md`.

