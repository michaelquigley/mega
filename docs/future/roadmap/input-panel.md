---
title: input panel as the control cluster
state: evaluating
created: 2026-11-10
tags: [enhancement, ui, operator-input-panel, conversation-view]
---

Third slice of the look-and-feel cluster. The operator input panel becomes a first-class component where the conversation's controls cluster, at the bottom, where the action is. Decided with Michael on 2026-11-10.

## Layout, bottom of the conversation, top to bottom

1. **Pending requests docked above the panel.** An approval or question card leaves its place in the scrolling history and sits fixed directly above the input, always in view. The history keeps its one-line row ("Approval for Bash: waiting for you", then "answered"). There is exactly one actionable place for a pending request. Several pending requests stack, oldest first.
2. **The activity line**, as built.
3. **The reply box.** One line tall when not in use; it enlarges when the operator interacts with the panel and collapses back when focus leaves the panel.
   - Collapsed, it shows the placeholder, or the first line of an unsent draft so a draft is never hidden.
   - Expanded, it opens to the remembered height and grows with its content up to about 60% of the viewport, then scrolls. A drag handle on its top edge sets the remembered height: one size for every conversation, kept in the browser (a preference, not workspace state).
   - "Leaving the panel" means focus leaving the whole cluster; using the control bar or the menu does not collapse it. Drafts survive collapse, conversation switches and reconnects, as now.
   - The placeholder says what is true: "Reply (Ctrl+Enter to send)" when a prompt can be sent; when it cannot, the same words the activity line shows ("Waiting for your approval", "Working…"), never a stale one.
4. **The control bar**, part of the panel and always visible, collapsed or not.
   - One side: Send, and Interrupt while a turn is in flight.
   - Other side: the permission mode, model and effort controls, moved down from the header.
   - A "more" menu: Release, Resume when it applies, Name and label, Remove from workspace, and the native events toggle.
5. **Release opens a confirmation** that carries what the stray handoff paragraph says today: the exact resume command with a copy control, and the rule that the terminal must be closed before resuming or prompting here. The paragraph itself is removed from the page.

The header becomes read-only identity: name and label, harness, the model and mode as the harness reports them (with "next prompt" where a stored choice differs), version, checkout, session id, and runtime state.

## Icons

Icons replace text on buttons wherever a control's meaning is conventional. Follow the house precedent in `products/screeningroom/ui`: `@material-design-icons/svg` pinned exactly, each icon imported as `?raw` SVG into one `Icon` component, embedded in the build with no font and no runtime fetch.

- Every icon button has a `title` and an `aria-label` naming the action, and takes the current text colour.
- Icon only: Send, Interrupt, Copy, the more menu and its entries (with their words beside the icon inside the menu), the code block's expand and collapse, dialog close, Register, Launch, Adopt, and the equivalents on the project page.
- **Allow and Deny keep their words**, with an icon beside each. An approval is the one place where a misread glyph has a cost.
- Release, Resume and Remove are in the menu with icon and word; destructive or consequential entries keep their confirmations.

This slice converts every button that exists today, so the vocabulary is set once; the side panel slice then adds its own controls in the same style.

## Checks

The browser check with the fake shim: the panel one line tall at rest, expanded on focus, collapsed on leaving, not collapsed by the control bar or menu; a draft's first line visible when collapsed; the dragged height remembered across a reload and across conversations; a pending request docked above the panel and absent from the history position, with its row present; two pending requests stacked oldest first; the mode and model controls working from the bar; the release confirmation showing the command and the rule, with the paragraph gone; every icon button carrying a title and an aria-label; no request leaving 127.0.0.1. Screenshots, light and dark, of the bottom cluster at rest, expanded, and with a docked approval.

## Not in this slice

`@` path search (its own slice; it needs a service endpoint); copy per block; side panel structure and its controls; keyboard shortcuts beyond what exists.
