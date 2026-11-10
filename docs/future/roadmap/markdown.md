---
title: markdown
state: evaluating
created: 2026-11-09
tags: [ui, conversation-view]
---

Michael needs markdown rendered in the conversation. Today `ui/src/components/EventList.tsx` drops assistant and user text into the page as plain text with `white-space: pre-wrap`, so headings, lists, tables, fenced code and emphasis arrive as their source characters. Both harnesses write markdown by habit, and the terminals render it, so a conversation in roundhouse currently reads worse than the same conversation in a terminal.

## Shape

**What renders as markdown.** Assistant text, whether managed or imported from outside roundhouse, and user prompts, since Michael writes them in markdown and both harnesses read them that way. The streaming delta renders through the same path, so a reply looks the same while it arrives and after it lands; the renderer must tolerate partial input (an unclosed fence shows as code until it closes, never as a parse error). Tool results, native passthrough events, request payloads and runtime text stay as they are: preformatted, collapsed, verbatim.

**Dialect.** CommonMark plus the GitHub extensions the agents actually use: tables, task lists, strikethrough, autolinks. No raw HTML; it is dropped, not escaped into view. No footnotes, no math.

**Safety.** Message content is untrusted text from a model, so: no raw HTML; link schemes limited to http, https and mailto; links open in a new tab with `rel="noopener noreferrer"`; images are rendered as links, not fetched, so nothing in a message makes the operator's browser load a remote resource. A pasted transcript that happens to contain `<script>` or an `onerror` attribute must not become live.

**Code.** Fenced blocks in a monospace block with the language shown and a copy control, since copying a command out of an agent reply is the commonest gesture. Inline code keeps its monospace. Syntax highlighting is deferred: it is the one piece that adds real weight to the bundle and it can be a later card once the plain rendering is in daily use.

**Typography.** Rendered inside the message, not the page: headings step down so an agent's `#` is a message heading and not a page title, tables scroll horizontally inside the column rather than widening it, long words and URLs wrap, and lists keep roundhouse's text size. Light and dark both already exist as plain CSS and get the same treatment.

**Library.** `react-markdown` with `remark-gfm` is the fit: React-native output with no `dangerouslySetInnerHTML`, HTML skipped by default, URL schemes sanitised by default, tolerant of partial input, and an ordinary pinned dependency in `ui/package.json`. Anything built on `marked` plus a sanitiser means owning the sanitiser; anything heavier (MDX, a full editor toolkit) is more than a renderer. Bundle cost is a few tens of kilobytes gzipped, acceptable for a localhost page.

**Interaction with what exists.** The scroll-follow rule and the reading position are unaffected, but a rendered reply is taller than its source and that height changes as it streams, so the follow-at-bottom check must hold through re-renders. The "Newer activity below" control must still appear when a rendering pushes content past the viewport.

## Checks

A scratch browser script with the fake shim, driving a reply through fenced code, a table, a nested list, a heading, a link, an inline `<script>`, an image reference and an unterminated fence mid-stream; asserting the rendered elements, the absence of any `script` or `img` element, the link attributes, the copy control, and that the view stays at the bottom while the reply streams. The reading position must still land on the last event.

## Not in this card

Syntax highlighting; a per-message "show source" toggle (the native events checkbox already exposes raw payloads); rendering markdown inside tool results; mermaid blocks in replies.
