---
title: codex interrupt while a turn is starting
state: researching
created: 2026-11-20
tags: [defect]
---

An interrupt pressed while a Codex turn is starting is lost. Between sending `turn/start` and Codex naming the turn, by `turn/started` or the reply, the adapter has no turn id. `Interrupt` (`internal/harness/codex/runtime.go`) then returns success and sends nothing. The service has recorded the prompt and marked the interrupt under way, so the page shows it under way until the turn ends on its own. If that turn asks a question meanwhile, its card is raised, and the service refuses an answer that would let work go on during a stop.

Found by Terminus pass `95c8c2fa354c` on piece 1, `a-codex-question-is-a-message`. It is older than that piece. Michael vetoed it there on 2026-11-20, for this card.

Two ways to fix it:
- **Carry it through.** Keep the interrupt while a turn start is outstanding, and send `turn/interrupt` once the turn has an id. The send has to come from outside the connection's reader, which delivers the id.
- **Refuse it.** While a turn start has no id, refuse the interrupt, saying the turn is still starting, and leave the service's `interrupting` unset. Smaller, but the operator presses again.

Either touches `Prompt`'s path. Claude and pi should be checked for the same window.

## from the review of the plan, 2026-11-24

Piece 4, `a-turn-is-inferred-from-a-message`, is to be built so that a turn opens only when the harness says it has started. Under that rule no turn is open before Codex has named it, so there is nothing to interrupt in this card's window and neither fix above is needed. This card closes with piece 4 if Michael keeps that direction after trying it.
