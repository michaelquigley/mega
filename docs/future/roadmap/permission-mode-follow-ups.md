---
title: Permission mode follow-ups
state: evaluating
created: 2026-11-09
tags: [permissions, claude]
---

Left open when the permission-mode batch gated clean (round `3159118a5aa3`), none worth reopening the tree for.

- Quote the two names in messages in `internal/service/permission.go` (advisory from the clean round).
- A refused switch restores the choice's previous value, which may be "no choice, inherit", rather than pinning the last reported mode. The current behaviour leaves an inheriting conversation with an explicit choice equal to its running mode after a refusal; harmless, but not what was meant.
- `permissions.defaultMode` is read from the checkout directory, not its git root, so a default set in a parent's `.claude/settings.json` is not inherited when a checkout sits below its repository root. Safe direction only (the conversation runs in manual mode). Confirm how Claude itself resolves project settings for a sub-directory cwd before changing the lookup, and note the limit in `docs/current` meanwhile.
- The header's mode control renders only while the runtime is running, so a stopped conversation cannot be set to auto ahead of its next prompt; it has to run one turn in manual mode first. Let the control set the stored choice while no runtime exists (write the choice, show "next launch: X", no harness call), and consider showing it for an idle conversation as the first thing Michael wants after a restart.
