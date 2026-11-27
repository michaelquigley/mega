---
title: the shutdown step can miss the asking page's banner
state: researching
created: 2026-11-26
tags: [defect]
---

The `shutdown` suite's step "the asking page says when" (`checks/shutdown.mjs`, line 134) failed once on 2026-11-26, during the gates of `adoption-reaches-any-session-in-the-checkout`, which touches none of this. The step before it had passed: Shut down ended the service with status 0. The asking page then never showed "mega was shut down from this page at …" within the wait, and the suite crashed there, at step 17.

## what was seen

- **The machine was loaded.** The full `make checks` run shared it with Go test builds and a Codex probe run.
- **It did not repeat.** Run alone three times, `shutdown` passed 35 of 35 each time, in about 3 s. It passed again in the final full run on the same tree.

## unconfirmed

The page has two banners for a shutdown it asked for (`ui/src/components/Gate.tsx`):

- "mega was shut down from this page at …", when it heard mega say the shutdown finished;
- "A shutdown was asked for from this page at …, and contact was lost", when contact was lost before the stream's last word.

The step accepts only the first. Under load, the stream may drop before the page hears that the shutdown finished, and then the page rightly shows the second. If so, it is the step that is wrong, not the page.

## what would settle it

What the banner read when the step failed. The crash saved each page's text and a screenshot (`crash-0.txt`, `crash-1.txt` in the suite's scratch). Those were removed with the scratch, and nobody read them. The next time it fails, read the banner before the scratch is removed.
