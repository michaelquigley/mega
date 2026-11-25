---
title: a checks suite empties its scratch before it knows its port is free
state: researching
created: 2026-11-24
tags: [defect]
---

`suite()` in `checks/lib/mega.mjs` removes `<checks root>/<suite>/` before `start()` asks whether something already answers on the port. A second `make checks` on the same machine, with the default `CHECKS_DIR`, therefore deletes the HOME, checkouts and data of a suite the first is running, and only then refuses for the port. Check the port first, and empty the directory only once the suite knows it will run.

## why

Found on 2026-11-24 while looking for the cause of `the-dialogs-suite-crashes-on-a-first-full-run`, which turned out to be something else. Nothing is known to have hit it. A gate run beside a terminus review, or beside another session's gate, is the way it would: the second run's suites each fail fast on the port and take the first run's scratch with them as they go.
