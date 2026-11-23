---
title: macos application signing and notarization
state: inbox
created: 2026-11-20
tags: [story]
---

Sign `mega.app` with a Developer ID and notarize it, so an operator can download a built application and run it, rather than build it from a clone.

## why

`make app` signs ad hoc, which runs only on the machine that built it: a copy downloaded from elsewhere is quarantined, and Gatekeeper refuses it. An ad hoc signature also changes with every build. macOS keys its privacy answers to the signature, so every rebuild asks the operator again for whatever their shell start-up reaches (journal, 2026-11-20). A stable identity fixes both.

## background

- It needs an Apple Developer account, whose identity would sign every build. Whose account that is, Michael's or another's, is a decision for him.
- Notarization needs the hardened runtime (`codesign --options runtime`). mega loads no plug-ins, but the window process uses WebKit and the shim runs `node` from the operator's shell. Check that neither needs an entitlement under the hardened runtime.
- The shim's `node_modules` is in `Contents/Resources`. Notarization rejects unsigned Mach-O files anywhere in a bundle. `npm ci --omit=optional` leaves none today; a future SDK that ships one would need it signed.
- Once there is a signed build, it fits `first-tagged-release`: a tagged release could carry the `.app` for macOS beside the Linux binary.
