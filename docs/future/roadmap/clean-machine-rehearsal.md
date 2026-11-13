---
title: clean machine rehearsal
state: researching
created: 2026-11-13
tags: [story, spike]
---

Follow the README, and nothing else, on a machine or account that has never had mega, and record every place it fails, stalls or assumes something. The output is a list of findings, each either fixed on the spot or turned into a card.

## why

Every install of mega so far was made by someone who already knew how. The README names an exact Claude Code version at an exact path, an exact Codex version, Node 24 and the Go in `go.mod`; the binary is stamped with its checkout's path; the launcher goes through an interactive bash. Nobody knows which of those a newcomer trips on first, and guessing is slower than trying.

## shape

- **Two passes.** An agent's pass on `fortyfive` in a scratch home with no configuration, no workspace, none of Michael's shell profile and no harness logins: it shows what a newcomer sees before anything is set up, and how well mega explains what is missing. Then Michael's own pass on his Windows machine under a fresh WSL2, with real logins, through to a working conversation.
- The agent's pass copies no credentials into its scratch home. Where a login is needed it stops and records what mega said.
- Whatever versions of Claude Code and Codex a fresh install gets today, not the pinned ones. What happens then feeds `harness-version-tolerance`.
- Walk the whole first hour: clone, build, start, register a project, launch a Claude conversation and a Codex one, answer an approval, stop and start mega again.
- Record the exact words of every error and every moment of "what do I do now". The empty workspace's first screen counts.

## rules

The account is disposable and so is everything in it. Nothing is installed or changed for Michael's own account, and his mega keeps running untouched.
