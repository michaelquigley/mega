---
title: a new conversation with no model list gets the Claude effort sentence
state: researching
created: 2026-11-26
tags: [defect]
---

When the model list cannot be read and the harness's settings name a model but no effort, `copyFor` reports `needEffort` for any harness, and `StartConversation` refuses with `errChooseEffort`: "the Claude settings name no effort level, so choose one for this conversation". For a Codex or pi conversation that sentence is untrue twice over. The settings are not Claude's, and the cause is the unread list, not a missing effort. A launch of a conversation that already exists now says the list could not be read (`errModelsUnread`, `an-older-log-is-read-as-todays-build-reads-it`). Creation could say the same where it cannot tell whether the model takes an effort.

It needs the settings read to succeed while the list read fails. Codex (`config/read`) and pi (`get_state`) read both from the harness's own process, so for them it is rare: one call failing or timing out while the other answers. Claude's settings are files, so for Claude it follows any failed list. There the sentence is true, but it names a missing effort where the cause is the list. Found while building `an-older-log-is-read-as-todays-build-reads-it`, and raised by terminus `a2bfdaa4ed6c` as a docs advisory, `launch-refusal-docs-conflate-creation-and-resume`. The code predates that card.
