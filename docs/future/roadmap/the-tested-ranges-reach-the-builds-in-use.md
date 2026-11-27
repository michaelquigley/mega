---
title: the tested ranges reach the builds in use
state: building
created: 2026-11-24
tags: [story]
---

Run the probes that admit a build to mega's tested ranges against Claude Code 2.1.331 and pi 1.6.1, record what they saw, and widen each range if its probe holds.

Part of `simplify-the-core`, the last card before the full-tree review, so that a release states tested ranges that are true. Until 2026-11-27 this card was `claude-code-21331-is-past-the-tested-range`; pi was added when Michael updated it. The card is the whole plan; there is no work order.

## why

`harness.Tested` in `internal/harness/versions.go` ends at 2.1.326 for Claude Code and names 1.5.2 alone for pi. Claude Code on `fortyfive` updated itself to 2.1.331 on 2026-11-24, and Michael updated pi to 1.6.1 on 2026-11-27. So mega marks every Claude start and every pi start as newer than it was tested with, which teaches the operator to read past the note. Codex is 0.166.0, inside its range.

The round trips are not what a range claims. `make e2e` has passed on 2.1.331 since 2026-11-24, and all five pi round trips passed on 1.6.1 on 2026-11-27. `probes/versions/README.md` says a build enters a range when its probe's steps pass on it, and for pi that the range is the build `probes/pi` ran.

pi's step is the larger one. 1.5.2 to 1.6.1 is a minor release, and the last time pi moved, from 1.1.0 to 1.5.2, a recorded observation changed: a pending dialog began to hold pi open when its input closes.

## what changes

- **Claude Code:** `probes/versions/claude.sh` is run against the 2.1.331 build. If every step passes, the range's upper end becomes 2.1.331.
- **pi:** `probes/pi/rerun.sh` is run, every scenario, and `probes/pi/close-with-dialog.mjs` and `probes/pi/inspect.mjs` with it, against 1.6.1. Each result is compared with what the capability report records for 1.5.2. If every recorded observation the adapter rests on holds, the range becomes 1.5.2 to 1.6.1.
- **The capability report** (`docs/research/harness-capabilities.md`) gains a dated entry for each build with what was seen.
- **Whatever states a range** is brought in step: the comment on `Tested`, `docs/current/mega.md`, the glossary, the README and the probes' own READMEs, where they name one.

## what does not change

The adapters, the service and the page. How a version is read and compared, and the sentences for a build outside a range. Codex's range. A test or a check that names a version as its own fixture keeps it.

## when a probe does not hold

- **A Claude step fails on 2.1.331:** the range is not widened. The report says which step and what it saw, and the work stops and asks.
- **An observation about pi changed at 1.6.1:** it is recorded with its version, as `AGENTS.md` asks. If the adapter rests on it, the range is not widened, and the work stops and asks: what the adapter should do about it is a card of its own.
- **One harness can be widened without the other.**

## how it is run

- **The probes run real sessions on Michael's own login,** against the disposable fixtures only, as their READMEs say. A Claude run costs six model turns. The pi scenarios run on pi's own default, which is his local model, and no model is passed.
- **Before a pi scenario runs, check what pi's default is.** If it is a hosted model, stop and say so without running a prompt.
- **Trust is Michael's.** The pi fixture's trust was saved from his own terminal and was honoured at 1.5.2, and the round trips loaded its extension at 1.6.1. If pi asks again, stop and ask him. No `--approve` is passed, and no trust file, setting or credential is written or opened for its values.
- **The Claude run needs the shim** in a disposable copy of the tree (`make shim` there), so the Makefile's reinstall leaves Michael's running mega alone. `claude.sh` stops if the installed builds change during the run. Say so if it does.
- **Tests:** whichever assert the ranges are updated to the new ends. A test or a check that fails for any other reason is a change in behaviour, and stops and asks.
- **The gates are the epic's:** `make test`, all of `make checks`, `make e2e` with the harness versions and every skip named, and terminus on the working tree, then Michael. Run them from a disposable copy of the tree, and run all of `make checks` on the final code.
- **A terminus finding in code this card did not touch and did not make newly reachable** is matched to an existing card or captured in inbox, and listed at handoff.
- **A finding that is only wording,** or a fix that only changes this card's own check or test, is made and reported at handoff.
- **A finding whose fix would add state, a hold or an ordering rule** is brought to Michael as a question of whether to fix it at all. Anything else this card caused comes to him one at a time.
- **Scratch:** the probes' logs are git-ignored and stay out of the repository. Prompt text and replies stay out of the report: steps, counts and what held.
- **Size.** `internal/service` was 5,955 non-test lines when `an-older-log-is-read-as-todays-build-reads-it` was built, and this card is not expected to change it.
