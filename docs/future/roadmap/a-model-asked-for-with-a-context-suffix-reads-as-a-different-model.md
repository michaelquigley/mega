---
title: a model asked for with a context suffix reads as a different model
state: researching
created: 2026-11-24
tags: [defect]
---

A conversation whose model is asked for with Claude Code's context suffix, such as `claude-fable-5-2[1m]`, shows every turn as having run on another model than it asked for. In `sameModel` (`internal/service/choice.go`), compare the model without a trailing bracketed suffix, so the turn line stays quiet when only the suffix differs. The line's title keeps the exact strings asked for and reported.

## why

Michael saw it on one of his other machines on 2026-11-24: every turn line stayed on screen in the notice colour, reading "Ran Fable 5.2, asked claude-fable-5-2[1m] · xhigh · auto". That is the turn line doing what it does when what ran differs from what was asked.

Claude reports the model's own id, without the suffix. `sameModel` resolves both sides through Claude's model list, which has no row for the suffixed id, so the two never compare equal. On `fortyfive` no conversation asks for a suffixed model, and none of 555 recorded turns says the model differed.

A mark that is on for every turn teaches the operator to ignore it, and it is there for the turn that really did run on something else.

## what it does not settle

Whether the larger context window was in fact in effect. Claude's report names the model either way, so mega cannot tell, and this card does not claim it.
