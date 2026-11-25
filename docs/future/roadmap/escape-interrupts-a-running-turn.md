---
title: escape interrupts a running turn
state: researching
created: 2026-11-25
tags: [enhancement]
---

Bind Escape to Interrupt in the web interface, as it is in the harnesses' own terminals.

Michael, 2026-11-25, while trying piece 4 of `simplify-the-core`: "i really think we should also bind `esc` to interrupt in the web ui."

## to settle

Escape already means four things on the page, and each has to keep working or give way on purpose:

- in the reply box, it leaves the box and hands the keys to the history column;
- it closes the `@` path list;
- it closes the **More** menu;
- it closes a dialog.

So the binding needs a rule for when Escape interrupts: only with the keys in the history column and a turn running, a second Escape, or something else. An interrupt is not undoable, so a key that also leaves a text box should not fire it by accident.
