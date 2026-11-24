---
title: a conversation gets its session once
state: evaluating
created: 2026-11-23
tags: [story]
---

A new conversation is launched once, when it is created, and the session id that launch returns is saved in the registry, which alone binds a conversation to its session. A `started` line in the log still records the id, as history, and nothing reads it back. A conversation that comes out of that launch with no session in the registry never gets one: it shows the error, refuses a prompt and Start, and can be removed.

A removal from the review of `simplify-the-core` on 2026-11-23, built before piece 3. It is rulings 4 and 7 in `docs/future/simplify-the-core-inventory.md`, and the spec's section "A simpler core" has the rule.

## why

Today a first launch that fails is sorted into two kinds. One is proven to have failed before any session could exist, recorded `not_started`, and may be retried. The other leaves the session unknown and the conversation refused. Each adapter supplies the proof, and the service reads the conversation's whole log to check it before it allows a fresh session. Beside that, a session id the registry does not hold yet is kept in memory, seeded from the log at start-up, and saved by the next launch.

On Michael's own workspace over two weeks, `not_started` was recorded zero times and the unknown case once. Adoption is the fallback for both: a session no conversation holds is listed under "Adopt a session".

Michael, 2026-11-23, on the retry: "if it's a new conversation... there's no loss in not having a retry. so jettisoning the failed conversation, correcting the issue, and re-launching seems like the simplest path?" And on keeping the session id only in the registry: "A sounds simpler."

## the failure that becomes impossible

A conversation's session can no longer be known in one place and missing from another, and whether a conversation may start a fresh session is no longer worked out from its log.

## what changes

- **One rule replaces the proof.** `Prompt` and `Start` refuse a conversation with no session in the registry. Only `StartConversation` launches one, when it creates it.
- **Removed:**
  - `freshAllowedLocked`;
  - the `not_started` status and `harness.ErrNotStarted`, with the wrapping each adapter does to supply it;
  - the `session` and `sessionRecorded` fields of `live`, their seeding from the log's last `started`, and `sessionOf`, whose callers read the registry's `NativeID`.
- **A failed first launch records one `error`,** whatever failed and whenever: mega's own check before the harness starts, the harness's failure, or a session id that could not be saved. A runtime that is up is stopped, as now.
- **A launch of a conversation that has a session is as it was,** except that a failure records `error` where it recorded `not_started`. It can be launched again.
- **The refusal says what to do:** remove the conversation, and if the harness made a session, adopt it under "Adopt a session".
- **The page's two special cases for `not_started` go.** A `not_started` line in an older log shows as any runtime line does, with its text, and nothing reads it. That is ruling 2.

## what does not change

Everything else: the check in `started` against the bound session, discovery and adoption, Resume, removal, and everything a conversation with a session does. The canon needs no change. `exact-id-resume` already flags a new session started for an existing conversation, and this never starts one.

## how it is shown

- a first launch that fails before the harness starts, and one the harness fails, each record one `error`; a prompt and Start are then refused with the message above, across a restart too, and the conversation can be removed;
- a first launch whose session id could not be saved stops its runtime and ends the same way;
- a log that holds a `started` line with a session, for a conversation whose registry entry has none, is not resumed after a restart: the conversation refuses a prompt, and discovery lists that session, which can be adopted;
- a conversation with a session whose launch fails records `error` and can be launched again, resuming the same id;
- no path starts a second session for a conversation.

## how it is run

- **It starts by reading how each adapter fails a launch and what `ErrNotStarted` wraps,** so that removing it changes nothing else an adapter does. If an adapter relies on it for more than the retry, the work stops and asks.
- **Tests of the removed behaviour go, or are rewritten to the rule.** The report names each and says which. A test or a check that fails for any other reason is a change in behaviour, and stops and asks.
- **`docs/current/mega.md`:** the paragraph on a conversation's session in "Conversations and runtimes" is rewritten to the rule, and the shutdown paragraph no longer names `not_started`.
- **The gates are the epic's:** `make test`, all of `make checks`, `make e2e` with the harness versions and every skip named, and terminus on the working tree, then Michael.
- **A terminus finding in code this card did not touch and did not make newly reachable** is matched to an existing card or captured in inbox, listed at handoff, and not worked. Anything this card caused comes to Michael one at a time.
- **A finding whose fix would add state, a hold or an ordering rule** is brought to Michael as a question of whether to fix it at all, before any fix is proposed.
- **Size.** `internal/service` was 6,547 non-test lines at `ef13d06`. The report gives the figure afterwards, and what was deleted outright.

## as built

Built on 2026-11-23. `internal/service` went from 6,547 non-test lines to 6,447, `internal/harness` lost 25 and `internal/model` 4. `ErrNotStarted` was wrapped at ten sites in the three adapters and read only by the retry check, so nothing else an adapter does changed.

- **The refusal states the fact and no cause:** "this conversation has no recorded session, so mega will not launch it; remove it, and if the harness made a session, adopt it under 'Adopt a session'". After a power loss the launch had succeeded and only the binding was missing, so naming a failed launch would have been wrong.
- **A binding whose directory sync failed may be on disk at the next start.** The registry renames its file into place before it syncs. The running service shows no session it could not confirm, and at the next start the registry on disk alone decides.
- **An older log's `not_started` line no longer sets the error mark,** so such a conversation shows idle and is still refused a launch. Michael left it, as ruling 2.
- **The session may not be offered for adoption.** Codex and pi list the 50 newest sessions in a checkout. The card `adoption-reaches-any-session-in-the-checkout` holds that.

The last terminus review, `cb783ac469c0`, was `not_clean` on one finding in code this card did not touch, `failed-binding-exit-shows-running`: a second failed write leaves a stale `running`, which `a-failed-write-stops-mega` ends, and a forced stop clears the error, which `one-fold-for-what-happened` carries.
