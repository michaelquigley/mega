---
title: the tested ranges reach the builds in use
state: evaluating
created: 2026-11-24
tags: [story]
---

A harness build is admitted to mega's tested range by `make e2e` passing on it. The round trips are first made to cover everything admission needs, that cover is kept by the ordinary gate, and the three ranges are then widened to the builds in use.

Part of `simplify-the-core`, the last card before the full-tree review. Until 2026-11-27 it was `claude-code-21331-is-past-the-tested-range`. The card is the whole plan; there is no work order.

## why

All three harnesses are past their ranges on `fortyfive` on 2026-11-27: Claude Code 2.1.331 against a range ending at 2.1.326, Codex 0.172.0 against 0.169.0, and pi 1.6.1 against 1.5.2 alone. Claude Code moved 13 patch builds in about three weeks, by itself. So mega marks every start as newer than it was tested with, which teaches the operator to read past the note.

Admitting a build is heavy today. It is a roadmap card each time. It is three procedures: `probes/versions/claude.sh` wants a build's path and a port, `codex.sh` wants a whole install, and pi has no runner, so `probes/pi/rerun.sh` leaves logs that are compared by reading. And what it saw goes into the capability report as prose.

The round trips already run on the installed builds as a gate, and the version probe's own driver says it mirrors them. The probe is separate because it can run a build that is not the installed one, safely.

Michael, 2026-11-27: "validating updated harnesses is going to be an ongoing maintenance item for mega, so we should probably figure out how to embrace it and build routine process around it." And on what admits a build: "if `make e2e` covers everything that needs to be tested to admit a new version, then yes... that should admit it. we need to make sure that `make e2e` covers everything meaningful that needs to be validated, and then maintain that."

## what "everything" means here

The round trips cover at least what admits a build today, and every call in the adapter interface. That is three lists, each finite:

- every method of `harness.Harness`, `harness.Runtime` and `harness.Looker`, and every callback of `harness.Sink`, for each harness that has it;
- every step the two version drivers check (`probes/versions/driver.mjs`, `codex-driver.mjs`);
- every scenario `probes/pi/rerun.sh` runs, with `close-with-dialog.mjs`.

A first look, by searching and not by reading every test, found three of the drivers' checks missing from the round trips: a Codex interrupt with an approval pending, that Claude's exact-id resume imports no event, and that Claude's local `/model` command stays out of speech after adoption.

## what changes

1. **A map of what the round trips cover.** One row for each entry of the three lists, with the round trip that makes it against the real harness, or the reason there is none. It lives in `internal/e2e`, in a form the ordinary gate can read.
2. **The gaps are filled,** as Michael rules on them: a new round trip, or an assertion on one there is.
3. **The ordinary gate keeps the map.** `make test` fails when a method or callback of those interfaces has no row, and when a row names a round trip that does not exist.
4. **`make e2e` ends by saying where each range stands:** the build each harness ran on, how many of its round trips passed, failed and were skipped, the range, and whether the build is admitted. A build is admitted when every one of its harness's round trips passed and none was skipped.
5. **What a range claims is rewritten** wherever it is stated: the comment on `harness.Tested`, `probes/versions/README.md`, `docs/current/mega.md`, the glossary and the README. An upper end is the newest build `make e2e` admitted. The version probe is for a build that is not installed: a lower end, or a control when a round trip fails.
6. **The routine is written into `AGENTS.md`,** in "Working rules", in the wording below. Michael approves it at handoff.
7. **The three ranges are widened** to the builds this card's last `make e2e` admits.

## the routine, as `AGENTS.md` is to say it

- A harness build enters its tested range when `make e2e` admits it. Run it when mega's start line calls a build newer than tested, when a change touches an adapter, and before a tag. Widen `harness.Tested` in the same change, and put the summary line in the day's journal.
- A round trip that fails on a new build is tried on `HEAD`. The same failure there is the build's: capture a card and leave the range.
- A change to what mega asks of a harness adds or changes its round trip, and its row in the map in `internal/e2e`.

## what does not change

The adapters, the service and the page. How a version is read and compared, and the sentences for a build outside a range. Each range's lower end. The version probe and the pi probes, which stay as they are. What `make e2e` needs to run, and that it is run by hand.

## when a round trip fails on a new build

The range is not widened for that harness. The report says which round trip and what it saw, and whether it fails on `HEAD` too. One harness can be widened without the others.

## how it is run

- **It starts with the map, and stops there.** The map and its gaps come to Michael as one list before any round trip is written. Each gap has a recommendation, fill it or leave it with the reason, and what filling it adds to every `make e2e` in model turns.
- **The round trips run real sessions on Michael's own login,** against the disposable fixtures only. Before a pi round trip runs, check that pi's own default is the local model, and stop without running a prompt if it is a hosted one.
- **Trust and approvals are as they are.** A new round trip answers only an approval it has asserted, as the others do. If pi asks to trust the fixture, stop and ask Michael.
- **Codex 0.172.0 has not run the round trips.** If one fails, it is tried on the earlier build only if that is still installed. Nothing is installed or downgraded to find out.
- **The gates are the epic's:** `make test`, all of `make checks`, `make e2e` with the harness versions and every skip named, and terminus on the working tree, then Michael. Run them from a disposable copy of the tree, and run all of `make checks` on the final code.
- **A terminus finding in code this card did not touch and did not make newly reachable** is matched to an existing card or captured in inbox, and listed at handoff.
- **A finding that is only wording,** or a fix that only changes this card's own check or test, is made and reported at handoff.
- **A finding whose fix would add state, a hold or an ordering rule** is brought to Michael as a question of whether to fix it at all. Anything else this card caused comes to him one at a time.
- **Scratch and reports:** prompt text and replies stay out of the report: steps, counts and what held.
- **Size.** `internal/service` was 5,955 non-test lines when `an-older-log-is-read-as-todays-build-reads-it` was built, and this card is not expected to change it. The round trips grow. The report says by how much, and what a full `make e2e` now costs in model turns.

## as built

Built on 2026-11-27. The card's last `make e2e`, on the final code, admitted all three builds: 27 of 27 round trips passed and none was skipped. The ranges are 2.1.318 to 2.1.331 for Claude Code, 0.163.0 to 0.172.0 for Codex and 1.5.2 to 1.6.1 for pi. `internal/service` is unchanged at 5,955 non-test lines, and `internal/e2e` grew from 1,919 lines to 3,371.

- **The map is `internal/e2e/map.go`,** and it reaches further than the card's three lists: all six adapter interfaces, `Locator` and `Versioner` among them, and every answer decision.
- **The guard is `map_test.go`,** with no build tag. It also fails when a round trip counts toward no harness.
- **A build that changes during a run is not admitted.** The summary reads each build once before the first round trip and once after the last. Terminus asked for a reading at every round trip, and Michael ruled for the two.
- **A round trip answers an approval only when the approval is its subject,** which is the canon's rule. Two that Michael had ruled otherwise were changed when terminus raised them. `TestClaudeBackgroundJob` places `bgprobe.sh` and a `settings.local.json` allowing exactly that script, for its own life, and answers nothing. `TestPiReleaseWithADialogPending` asserts the gate raised again after a resume and releases with it unanswered.
- **pi 1.6.1 sets its process title,** which removes its arguments from `/proc`. A round trip now finds its pi as the one child of the test's own process whose working directory is the fixture. mega's own check for leftover processes reads process groups and was not affected.
- **Beyond the lists, by Michael's ruling:** an operator's Deny that reaches Claude and Codex, and Codex's `allow_session` in place of Allow on the MCP approval.
- **`TestCodexDiscoveryAndHistory` is gone.** `TestCodexRoundTrip` discovers and adopts its own thread, which closes `codex-discovery-e2e-reads-its-own-thread`.
- **Cost:** a full `make e2e` is 49 model turns against 39, and took 376 s against 286 s. Claude takes 19 turns, Codex 15 and pi 15, one of pi's on the hosted model.
- **Two round trips failed on what a model did,** not on a harness: a pi gate came back after it was allowed, in two of five full runs, and `TestCodexDeny` once got a turn that never asked. Each now fails at once and says what it saw.

Terminus ran nine rounds, and the last, `a549db2e5225`, was `not_clean` on three blocking findings in code this card did not change: `request-stays-pending-during-delivery`, which `harden-requests-stops-and-restarts` holds, and two captured as `the-planner-deny-step-does-not-check-the-tool` and `the-idle-switch-round-trip-reads-claudes-replay-label`. An advisory on older code is `the-codex-release-round-trip-does-not-see-its-command-running`.

`make test` and all of `make checks`, 612 steps, passed on the final tree.
