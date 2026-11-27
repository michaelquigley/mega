---
title: adoption reaches any session in the checkout
state: evaluating
created: 2026-11-23
tags: [defect]
---

Codex and pi list every session in a project's checkout for adoption, as Claude already does. The cut at the 50 newest is removed.

Part of `simplify-the-core`, built before `an-older-log-is-read-as-todays-build-reads-it`. The card is the whole plan; there is no work order.

## why

`Adopt` (`internal/service/adopt.go`) calls `Discover` and refuses a session that is not in the result. Codex and pi return at most 50 sessions for a checkout, the newest first (`discoverLimit` in `internal/harness/codex/codex.go` and `internal/harness/pi/pi.go`), and the sessions a conversation already holds are taken out after that cut. So a Codex or pi session that is not among the 50 most recently used in its checkout cannot be adopted at all.

Claude has no cut of mega's. The shim's `list` asks the SDK for the checkout's sessions with no limit, and the adopt dialog shows what comes back, with a filter on the first prompt.

Counted on Michael's machine on 2026-11-26, as numbers of session files only: one pi checkout holds 183 sessions, so 133 of them cannot be adopted today. One Claude checkout holds 436, asked for with no limit. Files were counted, not rows in the dialog. The largest Codex checkout held 38 on 2026-11-23.

The review of `simplify-the-core` leans on adoption as the fallback. Rulings 2, 3, 4 and 7 in `docs/future/simplify-the-core-inventory.md` each end with "remove the conversation and adopt its session again".

Michael, 2026-11-26: "i'm not sure what discoverability options there are for really old sessions... so if i don't have the id, it's probably just lost?" It is not lost. The session is in its harness's own store, and what cannot reach it is mega's cut. With the cut gone the session is in the list, and the filter finds it by its first prompt.

## the failure that becomes impossible

A session in the checkout can no longer be out of adoption's reach because of where it falls in a list.

## what changes

- **pi lists the whole checkout.** `discoverLimit` and the `limit` argument of `discover` (`internal/harness/pi/session.go`) go. pi's scan already reads every session file in the checkout's folder before it cuts, so nothing more is read.
- **Codex lists the whole checkout.** `Discover` returns every thread Codex has for it. `internal/harness/codex/mcp.go` already reads another Codex list page by page, with `cursor` and `nextCursor`. Whether `thread/list` pages the same way is the first thing found out, below.
- **The dialog's filter matches a session's id as well as its first prompt,** and its label and its empty line say so. An id copied from a conversation's header then finds its session with one paste.

## what does not change

`Adopt` and its check: a session must be one discovery lists for the checkout, and one no conversation holds. The list's order, the count of held sessions, and the look at a session before it is adopted. The routes. The Claude adapter and the shim. `internal/service` is not expected to change, and if it has to, that is a question for Michael.

## two routes not taken

- **Adopting by an exact id, past the list.** Today only the list says a session belongs to this checkout. pi's read by id falls back to any checkout's folder (`sessionFile`), and Codex's `thread/read` takes no checkout. Each adapter would need a new check, and a rule for a record that names no checkout.
- **"Show more".** It is a control on the page and a limit through the route and all three adapters, to hold back rows the dialog already shows for Claude.

## how it is shown

- a pi checkout with more than 50 sessions lists all of them, and the oldest is adopted;
- Codex's list is read to its end over more than one page, and a thread from a later page is adopted;
- an id typed into the filter leaves that session's row, and a first prompt still filters as before;
- the round trips that discover and adopt, for all three harnesses.

## how it is run

- **It starts by finding what Codex's `thread/list` gives past one page.** Ask it for a page of two against the fixture checkout, which holds more threads than that, and see whether a cursor comes back and whether following it reaches the rest. That needs no new sessions and no model turn. If Codex cannot list a whole checkout, the work stops and asks.
- **It confirms the SDK's `listSessions` has no cap of its own.** If it has one, the report says so and nothing is built for it.
- **Time.** Discovery has a minute (`discoverTimeout`). The report says how long Codex's whole list takes on the fixture checkout.
- **Documents:** "Discovery and adoption" in `docs/current/mega.md`, the harness capability report if it records the limit, and the changelog.
- **Tests and check steps:** new ones for the cases above, the filter's in the dialogs suite. A test or a check that fails for any other reason is a change in behaviour, and stops and asks.
- **The gates are the epic's:** `make test`, all of `make checks`, `make e2e` with the harness versions and every skip named, and terminus on the working tree, then Michael. Run them from a disposable copy of the tree, so the Makefile's reinstall of the shim leaves Michael's running mega alone.
- **A terminus finding in code this card did not touch and did not make newly reachable** is matched to an existing card or captured in inbox, and listed at handoff.
- **A finding that is only wording,** or a fix that only changes this card's own check or test, is made and reported at handoff.
- **A finding whose fix would add state, a hold or an ordering rule** is brought to Michael as a question of whether to fix it at all. Anything else this card caused comes to him one at a time.
- **Size.** What is counted to change: pi loses a constant and a parameter, Codex gains a loop like the one in `mcp.go`, and the page changes one test and two strings. `internal/service` was 6,096 non-test lines when piece 7 was built. The report gives the figures afterwards.

## as built

Built on 2026-11-26. `internal/service` is unchanged at 6,096 non-test lines. The Codex adapter went from 2,708 to 2,738, pi from 1,748 to 1,744, and `ui/src` from 7,963 to 7,964.

- **Codex pages by `updated_at`, 100 a page, to the end.** Codex 0.166.0 returns at most 100 threads a page whatever is asked. Under its default sort, `created_at`, the cursor is a whole second, and a page that ends inside a second skips the rest of it: pages of one, two and three returned 340 or 341 of the fixture's 344. Under `updated_at` the cursor carries milliseconds and every page size read all 344. That is observed and not promised by Codex. Michael ruled for it on 2026-11-26. The capability report has the observation with its version, and the `pages` command in `probes/codex/probe.mjs` is what to run again when Codex is upgraded.
- **The read has no page bound.** It runs under discovery's one-minute deadline, so a list that never ends fails and is never cut short. The fixture's 344 threads came back over four pages in 1.4 s.
- **A thread already listed is not listed again.** A thread updated while the list is read can be missed that once, and reopening the dialog reads the list again. Nothing was built for it.
- **Codex's own defaults still decide what is listed.** A thread made by `codex exec`, or one archived in Codex, is not listed and cannot be adopted, as before.
- **The SDK's `listSessions` has no cap of its own** at 0.3.318: 436 rows for 436 session files.

The last terminus review, `71eba39f3e59`, was `clean`. An earlier one raised `codex-pagination-outside-adapter` against the probe. The probe stayed as it was, and the canon's `protocol-confined-to-adapters` now says a probe speaking a harness's protocol is not flagged.

The full `make checks` passed after the first round of review fixes, and `dialogs` alone was run again on the final code. `environment` and `panel` also open discovery and were not run again.

Captured during the gates: `the-shutdown-step-can-miss-the-asking-pages-banner`.
