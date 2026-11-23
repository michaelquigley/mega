---
title: harden requests, stops and restarts
state: researching
created: 2026-11-19
tags: [enhancement, spike]
---

Review the full tree with terminus and work through every finding until it is clean, so that a review of ordinary work comes back clean unless the work itself is not. Most of what it will find is in three places: the life of a request, what happens when a runtime is stopped, and what is counted when one is lost and resumed.

## why

Between 2026-11-18 and 2026-11-19, five reviews of five different batches each confirmed the batch and then blocked on five or six older findings in the files it touched. Every finding was real. Twenty-odd were fixed, in two rounds built for nothing else, and the sixth review returned six more. Michael, 2026-11-19: "we need to figure out how to clean up these recurring terminus findings... should we do full-tree terminus until clean?"

Three things explain the pattern:

- **The faults cluster.** Nearly all sit in `internal/service/conversation.go` and `live.go`, the adapters' stop paths, and the page's sense of whether it is current. Each concerns one of a few sequences: answering a request, stopping a runtime, resuming after a loss.
- **Those sequences are written out by hand each time.** Record, then change memory, then tell readers; check for a stop, then start work. Each place that does it can get the order wrong in its own way, so fixing one finding leaves its siblings.
- **A reviewer samples.** Each pass reads what it has room for and reports what it sees. Fixing after every pass means never seeing the whole list.

The second is the one to act on. It has a precedent here: stage 2 went four rounds without a clean review until every runtime state change was put through one helper, `Service.transition`, and then it converged.

## the method: the full tree, to clean

Michael, 2026-11-19, on the planning agent's first answer, which argued against this: "terminus always eventually converges, in every other project? by working through all of the findings and either adjusting the code or the qualities, eventually we get to `clean`. i don't understand why we wouldn't just do that here?"

He is right, and the first answer was a prediction against the evidence. Terminus has converged on every other project of his, and on this one: each of the four stages of the first slice reached clean, the second after a helper was built in its fifth round. The planning agent's worry, that fixing after each pass has no reason to end, is not what has happened anywhere.

A clean full tree is also the direct cure for what prompted the question. Reviews of ordinary batches have been blocking on older findings in the files a batch touches. With nothing older left to find, a batch's review is clean unless the batch itself is not.

So: review the full tree, work through every finding, and repeat until it is clean.

- **Each finding ends one of two ways:** the code changes, or the quality does. A finding that is a fault is fixed. A finding that is a matter of judgement goes to Michael as a proposed change to the canon, with the reason, and the canon is his to amend.
- **Nothing is listed and left.** The rule of the last few rounds, that an older finding is reported once and not touched, does not apply to this work.
- **A fix is proven before it is kept:** a test that fails first, then `make test` and `make checks`.

What survives from the first answer is advice on how, not whether: where several findings share one sequence of steps, fix the sequence in one place, which is what made stage 2 converge.

## where one change may settle several findings

- **A request's life in one place.** Pending, being answered, answered, aborted, lost. One function decides each move, records it, and only then changes what readers see. Four of the fixed findings and three of the six below are about this order.
- **One fence for a stop.** Once an interrupt or a release has begun, one check decides what may still be delivered, for every harness. Fixed for answers on 2026-11-19; a Codex answer already accepted still gets through.
- **One sense of "current" in the page.** The footer, the rail, the shutdown banner and the held controls each ask in their own words whether the page has caught up. Three findings were the same question answered differently.
- **Turns that the harness declares.** `a-turn-is-inferred-from-a-message` is this card's fourth candidate, already written.

## the seed list: six older findings from the review of 2026-11-19

The [November 19 journal](../../journal/2026-11-19.md#batch-loose-ends-around-stops-and-restarts-implementation-agent) records these findings from the preceding batch.

1. Release does not fence a Codex answer already accepted, which can start a turn just after Release returns; the adapter then interrupts it.
2. A harness's own resolution of a request that cannot be recorded leaves the request answerable.
3. An answer the adapter rejects without settling it, such as a choice pi did not offer, is recorded as aborted, removing a dialog pi still waits on.
4. pi's history never marks an open turn, as Claude's did not until that round.
5. A dropped connection after a shutdown request reads as a completed shutdown, in the banner, the footer and the rail.
6. Resume logs a failure to save the unread marker, then clears "unsupervised" anyway.

## also for terminus

A card in terminus's own roadmap proposes a baseline of known findings, so a carded finding stops coming back as new. It was written before Michael's answer above. A clean tree needs no baseline, so it matters only for a project that chooses not to go to clean.

## after two passes, 2026-11-19

The seed list and two full passes are worked: five of the six seeds and ten findings fixed, each with a test seen to fail first. Two changes of shape came out of it, as hoped: every move of a request out of the pending list goes through one function that records first, and every runtime has one stop fence, latched in all three adapters, that refuses anything but Deny or Dismiss. The fence came back once because its first version was too narrow, which is the one finding that recurred.

The count went five, five, five. The implementer's reading, which the planning agent shares: these are not the same findings returning. Each pass reads a different part of a large tree, and the three areas still hold more. It should fall as they are worked through.

Decisions, as the planning agent recommended them to Michael:

- **The shutdown banner.** Fix it without changing what an ordinary shutdown says: the service tells the page when a shutdown has finished, as its last word before it closes. A page that heard it says mega was shut down. A page that asked and then lost contact without hearing it says a shutdown was asked for and contact was lost.
- **A request whose settlement could not be recorded** stays on the page as last confirmed, unmarked, with further answers refused. The canon's `single-authority` gained a line saying so, so two reviews do not disagree about it again.
- **Go on,** reporting after each pass.

One thing noticed on the way, not yet a finding: when a write has failed and the service refuses further writes, nothing on the page says so until the operator tries something. A sign that mega can no longer record would say it once, for everything.

## after three passes, 2026-11-19

Pass 3's five are fixed, and both rulings are built: the service's last word on the stream says a shutdown has finished, so an ordinary shutdown reads as before and a lost one says contact was lost; and a settlement that could not be recorded now refuses a further answer with the reason. Pass 4 returned six. The counts are five, five, five, six.

**What the passes show when read together.** The same kind of finding has been arriving one harness at a time:

- "Is this the session that was asked for": pi importing a file by name (pass 1), Codex's history read unchecked (pass 2), Claude's check running only once (pass 3), pi checking one read and importing another (pass 4).
- "What was left running": Codex and pi before this card, Claude in pass 3.
- "A test that allows a request without looking": two fixed on 2026-11-19, one more in pass 4.

Each was fixed where the review pointed, and its sibling turned up a pass later. The count will fall faster if a fix is made for the kind and not the instance: when a finding names one harness or one call site, the same question is asked of the other two harnesses and every other caller before the next review.

**A decision for Michael, from pass 4.** If the registry write that marks a finished turn's output as unread fails, the output is never counted and nothing tries again. The planning agent's recommendation: do not retry while the registry refuses writes, which is the rule, and reconcile the marker from the event log at the next start. The log is the record and the registry's marker is derived from it, so start-up can always rebuild it.


## after four passes, 2026-11-19

Pass 4's six are fixed, and this was the first pass worked by kind and not by instance. For each finding the implementer asked the same question of the other harnesses and callers, and fixed what it found that the review had not named: every environment resolution checked as it finishes, shutdown waiting for a launch already under way, a failed launch's request settled, pi's runtime and Claude's history each checking whose session they read, and about thirty tests that answered a request without looking. Pass 5 returned five, four blocking, the first fall: five, five, five, six, five. One of the five is a hole in pass 4's own fix for the unread marker.

One thing left as it is, by the planning agent's recommendation: if the registry write of a runtime's state fails after a stop the log recorded, the next start shows the conversation as unsupervised. That is the safe side, and it is what `honest-runtime-state` asks of a state the service cannot vouch for.

## after five passes, 2026-11-19

Pass 5's five are fixed; the race detector is clean on the three adapters. Pass 6 returned five, four blocking. The counts are five, five, five, six, five, five.

**What is in the count now.** The findings have moved from the common paths to the corners, and about a third of each pass is a hole in an earlier pass's fix. One fix in particular has now needed three passes: rebuilding the unread marker from the event log at start-up. It was added in pass 4, found to depend on a fact not in the log in pass 5, and found in pass 6 to treat every runtime error as the end of a turn.

**The planning agent's reading of that one.** It keeps failing because it works out, from other events, what the service had decided. Each pass finds another event it reads wrongly. Recording the decision itself would end that: when the service counts output as unread, it logs that it did, and start-up takes the last such record. Nothing is inferred, so there is nothing to infer wrongly.

**A decision for Michael, as recommended.** When mega itself stops a runtime because of an error, such as a runtime on the wrong session, the stop ends as stopped and that clears the error. A stop mega forced because something was wrong is not a clean stop. The error should keep showing until a runtime starts, so the operator learns that something was wrong. A stop the operator asked for still clears it. `docs/current/mega.md` says an error shows "until a runtime starts or stops cleanly"; it wants the difference stated.

Michael's direction the same day: simplify the core where it can be, instead of covering every corner. That is the card `simplify-the-core`. The hardening passes continue to be the measure, on a core with fewer corners in it.

## paused, 2026-11-19

Michael stopped the passes here: "i think the implementer should stop now, and we should commit what they've done." Pass 5's fixes are the last work under this card until the simpler core is in. The work is the sequence of cards under `simplify-the-core`.

**Pass 6's five findings are not worked.** Two of them live in code the simplification removes. The other three stand on their own, are small, and can be fixed as a short batch at any time or when the passes resume. Each is checked again then.

- **`recovery-counts-non-exit-errors`** (blocking): the start-up rebuild of the unread marker treats every runtime error as the end of a turn. The rebuild goes with `unread-is-output-beyond-where-you-have-read`.
- **`answer-clears-concurrent-settlement-hold`** (blocking): when the adapter rejects an answer as invalid, the service drops the "takes no further answer" mark on a request settled meanwhile. That mark exists for a settlement that could not be recorded, and `a-failed-write-stops-mega` removes it.
- **`pi-cache-loses-native-identity`** (blocking): when pi cannot find a message's session entry, the message is stored without an id, and if the entry turns up later an import adds the same message a second time. It stands on its own.
- **`turn-record-shared-between-subscribers`** (blocking): copying an event for each stream subscriber does not copy its turn record, so one subscriber could change what another sees. It stands on its own.
- **`http-contract-omits-pi`** (advisory): the HTTP reference lists only `claude` and `codex`, and omits Dismiss for pi's dialogs. A documentation fix. Fixed on 2026-11-23 in `a-first-launch-holds-nothing-back`: the answer route now names `dismiss` where the request offers it, as each pi dialog does, and the workspace read's `harness` is `claude`, `codex` or `pi`.

**The two decisions above are carried into that sequence.** Whether a stop mega forced clears an error is settled on `one-fold-for-what-happened`. Recording the unread decision instead of inferring it is overtaken: the plain unread rule leaves nothing to rebuild, and the one fold makes "record the decision" the rule for every decision.
