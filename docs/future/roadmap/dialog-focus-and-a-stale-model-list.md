---
title: dialog focus and a stale model list
state: inbox
created: 2026-11-13
tags: [defect]
---

The register and new-conversation dialogs open with the focus in their first field, and the new-conversation dialog stops saying it is loading models once the harness has refused to list them.

## why

Found by `checks/firstrun.mjs` during the clean-machine rehearsal of 2026-11-13:

- **Focus.** `RegisterProjectDialog` gives the checkout field `autoFocus`, and `LaunchDialog` gives the opening prompt `autoFocus`. React focuses the field on mount, then the effect calls `showModal()`, which moves the focus to the dialog's first focusable control: the close button, "Cancel". A newcomer who opens Register and types a path types nothing.
- **The model list after a refusal.** When the harness cannot list its models, the dialog says "The harness did not list its models (internal error); a launch takes what your settings name." The Model select still reads "Loading models…", and Effort reads "Choose an effort". Launch stays enabled, and a launch for a harness that cannot run fails.
- **Height.** At a 1280×900 viewport the new-conversation dialog with a notice is taller than the window, and Launch sits below the fold.

## shape

- Focus the intended field after `showModal()`, or give it the `autofocus` attribute the dialog element honours.
- On a models refusal, the select says the list is unavailable, and a harness the refusal shows cannot run is treated as one marked "cannot run".
- `firstrun.mjs` records the focus today without checking it, and checks it once this lands.
