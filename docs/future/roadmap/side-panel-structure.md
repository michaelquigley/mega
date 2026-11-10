---
title: side panel structure
state: building
created: 2026-11-10
tags: [enhancement, ui, side-panel]
---

Fourth slice of the look-and-feel cluster. The side panel becomes something Michael can arrange: projects that sort, fold and can be starred, and a panel that resizes and collapses to a rail without ever hiding that something needs him. Decided with Michael on 2026-11-10: stars live in the workspace, and the collapsed panel is a thin rail with marks.

## Shape

- **Project names are bold** (weight 600) whether selected or not. Conversations underneath stay as they are, and selection is carried by the row background alone; check the selected row still stands out in both themes.
- **Projects are sorted by name**, case-insensitively, with a stable tiebreak on id. Conversations inside a project keep their current order.
- **Projects fold.** A disclosure control on each project row hides or shows its conversations. A folded project still shows its rollup mark from the working-indicators slice. Selecting a conversation through the needs-you list or the next-waiting shortcut unfolds its project, so navigation never lands on something hidden.
- **Starred projects** form their own group at the top of the tree, sorted the same way, with the rest below. A star toggle sits on each project row.
- **The panel resizes** by a drag handle on its right edge, within sensible bounds (about 200px to 480px).
- **The panel collapses to a thin rail.** The rail shows the needs-you count, one mark per project in tree order (starred first) with the project's name as its title, and the control to expand. Clicking a project's mark expands the panel with that project selected. The needs-you count uses the notice colour when it is not zero. Nothing that signals attention or activity is lost by collapsing.
- **Icons throughout**, in the vocabulary the input-panel slice set: fold and unfold, star and starred, collapse and expand, each with a `title` and an `aria-label`.

## Where each thing is remembered

- **Stars are workspace state.** A `starred` field on the project in the registry, written only through the service, so a star is the same in every browser where roundhouse is opened. `PATCH /api/projects/{id}` accepts `starred` beside `name`. The write follows the existing rules: memory moves after the confirmed registry write, and browsers learn of it through `workspace_changed`. An older registry without the field reads as unstarred; the registry version does not change.
- **Panel width, panel collapse, and which projects are folded are browser preferences**, kept in the browser like the reply box's height. They arrange one window and are not workspace state. A folded project that no longer exists is simply forgotten.

## Checks

A service test that starring writes the registry before memory moves, survives a restart, and that an old registry reads as unstarred; a server test for the `PATCH` field. The browser check with the fake shim: sort order with mixed case; fold and unfold with the rollup mark still shown; needs-you selection unfolding a folded project; a star moving a project to the top group and appearing in a second page without a reload of that page beyond the usual `workspace_changed` refresh; the dragged width and the fold state surviving a reload; the rail showing the needs-you count and one mark per project, a rail mark opening the panel on that project, and a working arc visible on the rail; every new control carrying a title and an aria-label. Screenshots, light and dark, of the panel with a starred group and a folded project, and of the rail with mixed states.

## Not in this slice

Reordering by drag; starring or sorting conversations; hiding projects; a keyboard shortcut for collapsing the panel; `@` path search; copy per block.
