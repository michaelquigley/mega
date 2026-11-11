---
title: path search follow-ups
state: inbox
created: 2026-11-10
tags: [enhancement]
---

Three small things left open when path search landed (terminus round `ef25761a8492`, clean):

- A file git still tracks but that has been deleted from the working tree can appear in the list. Drop entries that no longer exist.
- With only `@` typed, the shortest paths lead, so dot-entries such as `.claude/` come first. With an empty query, rank dot-prefixed entries last.
- A path containing spaces is inserted as is. Probe whether Claude's `@` mention needs quoting for such a path (`@"a b.txt"`) and insert the form that attaches; record the observation in the capability report.
