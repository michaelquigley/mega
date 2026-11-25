---
title: shown environment values can outlive their resolution
state: researching
created: 2026-11-25
tags: [defect]
---

Show values on a project's environment section keeps the changed variables' values in the page's memory, and the section draws them under whatever resolution the service reports now (`EnvironmentSection.tsx`, `values` beside `env.resolved`). They are never checked against it. Say Show values ran under activation A and the operator then saves override B. The section hides the table while B is not resolved. Once Resolve resolves B, A's table comes back beneath B's time, fingerprint and names, and the page shows an older result as part of the current one. Draw the values only while their source, fingerprint and names match the resolution the service reports, or drop them when it changes.

Raised by terminus `b355b84ff561` on `reading-the-workspace-starts-nothing` as blocking (`single-authority`). The table dates from `6903c64` (2026-11-12). Before that card, the background resolution brought B's result back without Resolve, so the card did not make this reachable.
