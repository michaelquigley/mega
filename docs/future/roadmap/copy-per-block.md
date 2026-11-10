---
title: copy per block
state: evaluating
created: 2026-11-10
tags: [enhancement]
---

Fifth slice of the look-and-feel cluster: each block in the conversation can be copied with one click, as markdown or as plain text, plus the small leftovers on the same surface.

## Shape

- **Two copy controls on every message block**, the operator's prompts and the agent's replies alike, at the block's top-right, shown on hover or focus so the column stays quiet:
  - **Copy as markdown** copies the message's source text exactly as recorded.
  - **Copy as plain text** copies what is rendered, without markdown syntax: headings and emphasis as their words, list items on their own lines, code as its text, links as their text followed by the address in parentheses, tables as tab-separated rows.
- **One copy control on an expanded tool result**, copying the whole output regardless of the scroll height.
- **The streaming reply** offers no copy until it lands, so a half message is never what gets copied.
- Icons from the existing `Icon` component with a `title` and an `aria-label`; a brief confirmation on the control itself after a copy (the icon changing to a check for a moment), no toast.
- Code blocks keep their own Copy control, unchanged.

## Leftovers on this surface

- **An expanded code block stays expanded when its reply lands.** The landed message replaces the streamed one and the block collapses again; the expanded state should survive the swap.
- **The adopt dialog's radio button sits beside its session text**, not above it.
- **Codex can return to inherit while running** (the `codex-can-return-to-inherit-while-running` card): inherit is selectable on a running Codex conversation and applies at the next prompt like any other Codex change, with the bar showing "next prompt". Claude stays refused.

## Ride-along from the side panel slice

- **Conversations are indented under their project.** Since the fold chevron was added, a conversation's mark sits exactly under its project's mark, so the tree reads as a flat list. Indent conversation rows one level, so a conversation's mark starts under the project's name. The row's selection and hover background still span the panel's full width; only the content moves. The needs-you list is not a tree and stays flush.

## Checks

The browser check with the fake shim: both controls on a prompt and on a reply; the markdown copy equal to the recorded text byte for byte; the plain copy of a reply with a heading, a list, a link, inline code, a fenced block and a table matching the expected text; no control on a streaming reply; the tool result copy yielding the whole output; a code block expanded mid-stream still expanded after landing; the adopt dialog's radio aligned beside its text; inherit chosen on a running fake Codex conversation showing "next prompt" and sending the config's values on the next turn. A service test for the Codex inherit change.

## Not in this slice

Copying a whole turn or a whole conversation; selection-aware copy; `@` path search.
