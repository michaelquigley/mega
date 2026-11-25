---
title: the register dialog's prefill can land on what is typed
state: researching
created: 2026-11-24
tags: [defect]
---

The register dialog fills its Checkout field with the projects root when its first listing arrives, unless something has been typed by then (`RegisterProjectDialog.tsx`, `setCheckout((current) => current || ...)`). The guard reads React's state, not the field. When the root arrives while the field is empty and text is just going in, the text lands after the root: a path typed or pasted in full becomes `~/Repos//home/…/secret`, and the service refuses it as not accessible. Make the prefill unable to land on text that is going in, so a path typed the moment the dialog opens is the path registered.

## why

It was the cause of `the-dialogs-suite-crashes-on-a-first-full-run`. Playwright's `fill` selects the empty field and then inserts, in two steps, and the root landed between them. `probes/register-click` reproduced it 8 times in 80 registrations made the moment the dialog opened. The suite now waits for the field to hold the projects root before typing, which steps around the race; the page still has it.

An operator meets it only by typing or pasting within the moment between the dialog opening and its first listing arriving. They would see the doubled path and the refusal, and nothing is registered wrongly.
