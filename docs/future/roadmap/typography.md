---
title: typography
state: evaluating
created: 2026-11-09
tags: [ui, typography]
---

Metawoo applications set body text in Source Serif 4, and Nordwand Mono is the monospace face going forward. Roundhouse still uses `system-ui` and `ui-monospace`. Bring it onto the house faces.

## Precedents in the house

- **Source Serif 4**: pane bundles it through `@fontsource-variable/source-serif-4` (the `opsz.css` and `opsz-italic.css` imports, so the optical-size axis is live), declares `--serif: 'Source Serif 4 Variable', Georgia, 'Times New Roman', serif`, and sets `body` in it with a 1.8 line height. gloss ships the variable TTFs under `ui/public/fonts/` instead. The fontsource route is smaller (woff2, latin subset) and already proven in a Vite build embedded in a Go binary, so use it.
- **Nordwand Mono**: the web sites (metawoo, quigleycom, woodenstarship) self-host it under `assets/static/fonts/nordwand-mono/`: six woff2 files (Regular, Medium, Bold and their italics), `LICENSE-FONT.txt` (SIL Open Font License 1.1, source `https://github.com/tywr/Nordwand-Mono/releases/tag/1.3.3`), and six `@font-face` rules in `web/metawoo/assets/input.css` with `font-display: swap`. It is not on npm; copy those files and rules, license file included.
- The constellation design note gives the monospace its role: it is the register for text that is *labelling, not speaking*. Body prose speaks; metadata, status, identifiers, code and tool lines label.

## Shape

- **Faces embedded, never fetched.** Both come through the Vite build into `ui/dist` and the binary's `go:embed`. No `@import` of a remote stylesheet, no runtime font request; the loopback-only page makes no outbound requests, and this must not be the first.
- **Roles.** `--serif` for everything that speaks: message bodies (the markdown renderer's output), dialog prose, the handoff explanation, button labels, headings. `--mono` for everything that labels: the conversation header's fact line (harness, model, version, mode, checkout, session id), attention list metadata, runtime and turn status lines, timestamps, tool summary lines, request card parameter blocks, native event summaries, inline and fenced code, the needs-you shortcut hint.
- **Sizes.** A serif reads smaller than `system-ui` at the same pixel size: raise the body from 14px to 15 or 16px with a line height around 1.6 for message prose, and keep the mono register at 12 to 13px. Headings step down inside messages as they do now. Weights stay 400 and 600, italic available in both faces; no other weights are loaded.
- **Light and dark** are untouched; this is faces and sizes only. Check that the mono register stays legible on the dark panel at 12px.
- **Code blocks** in Nordwand Mono with the existing collapse and Copy behaviour.

## Checks

A browser check with the fake shim asserting the computed `font-family` of a message body, the header fact line, and a fenced code block; that the page makes no request off 127.0.0.1 while loading; and a screenshot of a conversation with a long reply, a request card and the sidebar for Michael to look at. `make test` covers the build. The license file ships beside the woff2 files in `ui/public/fonts/nordwand-mono/`.

## Not in this card

A display face or wordmark (roundhouse has none and the name is provisional); the favicon, which is its own card; colour or spacing changes beyond what the new sizes force.
