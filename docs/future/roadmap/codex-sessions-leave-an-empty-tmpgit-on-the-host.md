---
title: codex sessions leave an empty /tmp/.git on the host
state: researching
created: 2026-11-25
tags: [defect, spike]
---

Find out how to stop Codex leaving an empty `/tmp/.git` behind in ordinary use, and settle whether mega does anything about it for the runtimes it starts.

Michael, 2026-11-25: "it's wreaking havoc on other projects working on this same host", and, of carding it: "it's something we should try and address."

## what is known

Codex 0.166.0 creates an empty `/tmp/.git` while a sandboxed command runs under workspace-write and removes it when the command ends. A command that is cut off leaves it. Codex treats `/tmp` as a place sandboxed commands may write, which is why it guards a `.git` there. The one found on 2026-11-25 was made at 13:45:41 while one of Michael's own Codex conversations in mega was running commands. `/tmp/.codex`, `/tmp/.aws` and `/tmp/.agents` are left by the same sandbox.

Anything that walks up from a directory under `/tmp` looking for a repository stops at it: a Go build there fails at VCS stamping, and tools take `/tmp` for a project root.

## to find out

- whether this Codex has a setting that leaves `/tmp` out of the places a sandboxed command may write, and what a command that expects to write there then does;
- whether a newer Codex still leaves the directory;
- which stops leave it: an interrupt, a release, mega's shutdown, or only a killed process.

## to settle

Mega never overrides a harness's sandbox today. A setting passed for the runtimes mega starts would narrow the sandbox and not widen it, but it is still mega changing native behaviour, and the same directory comes from Codex in a terminal and from the review brokers. So the first answer may be the operator's own Codex configuration and a note in mega's documents, with nothing built.

`the-codex-round-trips-leave-an-empty-tmpgit-behind` is the round trips' own part of this.
