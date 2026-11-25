---
title: the codex round trips leave an empty /tmp/.git behind
state: researching
created: 2026-11-25
tags: [defect]
---

`make e2e` can leave an empty `/tmp/.git` on the host. Have the round trips remove it when they caused it: note before the run whether it exists, and after the run remove it if it did not exist before and is empty.

## why

Codex 0.166.0 creates an empty `/tmp/.git` while a sandboxed command runs under workspace-write, and takes it away when the command ends. A command that is cut off leaves it behind. `TestCodexReleaseDuringCommand` releases a runtime in the middle of a command, so a full run of the round trips leaves one almost every time. This was first recorded on 2026-11-09, and implementers have been removing it by hand after their runs since.

It is not harmless. Anything that walks up from a directory under `/tmp` looking for a repository stops at it. A Go build in a scratch directory under `/tmp` fails at VCS stamping. Mega's own trust tests failed on it on 2026-11-08. Michael, 2026-11-25: "it's wreaking havoc on other projects working on this same host."

## what this card does not fix

The round trips are one source. The other is ordinary use: any Codex session on the host, in mega, in a terminal or as a broker's reviewer, whose sandboxed command is cut off. The one found on 2026-11-25 was made at 13:45:41 while one of Michael's own Codex conversations was running commands, not by a test. Stopping that is a matter of Codex's own settings or version, which are Michael's.
