---
title: the failed model list sentence overstates the refusal
state: researching
created: 2026-11-25
tags: [defect]
---

When the page's model list fails with anything but `executable_missing`, the launch dialog says "A launch takes the model and effort your settings name, and is refused if they name none" (`LaunchDialog.tsx`), and `docs/current/mega.md`, "A refused list", says the same. The page's read failing does not mean the service cannot list the models when it handles the launch. `service.copyFor` fetches the list again and copies its default model, and the model's default effort where it names one. It refuses only for a value it cannot fill: no model to copy, or a Claude model that takes levels with no `effortLevel` in the settings. The `panel` check's failed-list launches show it. They fail only the browser's read, and both launches start, though the scratch settings name no model. The sentence should say what the service does: it fills what is unchosen from the settings and from the list if it can get one, and refuses a launch only when it cannot fill a model, or an effort the model needs.

Raised by terminus `58c8d4886d0d` (advisory, `docs-current-sync`) on `a-failed-model-list-launches-claude-in-default-mode`. The sentence predates that card.
