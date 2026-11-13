---
title: expanded states
state: evaluating
created: 2026-11-11
tags: [enhancement]
---

one the sidebar...

the projects marked favorite should default to being opened (expanded), showing their conversations.

the projects not marked favorite should default to being closed (collapsed), hiding their conversations.

## shape

- A project is unfolded by default when it is starred and folded by default when it is not.
- The operator's own choice wins and is remembered in the browser: folding a starred project or unfolding an unstarred one sticks until he changes it. The preference becomes three-valued per project: folded, unfolded, or no choice yet.
- Today's stored set of folded projects carries over as explicit "folded" choices. Everything else starts with no choice and takes the new default, which is the change Michael asked for.
- Starring or unstarring a project changes its state only if he has made no explicit choice for it.
- Navigating to a conversation still unfolds its project, and that counts as viewing, not as a choice: it does not overwrite the preference.
- A folded project keeps showing its rollup mark, as now.
