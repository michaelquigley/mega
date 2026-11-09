---
title: Remove a conversation from the workspace
state: inbox
created: 2026-11-09
tags: [ui, workspace]
---

The first slice removes projects but not individual conversations, so test and abandoned conversations accumulate in the tree. Michael asked for this on the day the slice gated, while clearing fixture clutter.

Shape, settled by the project case: removal is workspace-only, refused while the conversation has a runtime, and never touches the checkout or the native session, which stays discoverable under adoption. One endpoint (`DELETE /api/conversations/{id}`), one service method that drops the registry entry and the conversation's event log under the single-authority rules, and a control in the conversation header with the usual confirm. The needs-you list and selection must cope with the selected conversation disappearing.

Candidate for the first polish batch.
