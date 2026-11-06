# Roundhouse: follow-up harness probes

## Assignment and context

Continue the existing investigation using the probes and disposable fixtures already established. Read `docs/research/harness-capabilities.md` first. This is a bounded follow-up, not permission to implement Roundhouse or redesign the adapters.

The initial investigation demonstrated exact-session round trips across structured and native terminal interfaces for Claude Code, Codex, and pi. Resolve three remaining questions: client reconnect, clean project suspension, and Codex explicit question handling. Update the existing capability report and probe usage notes; preserve earlier evidence and label new observations by version.

## Operator decision: early ownership discipline

Michael has decided that operator care is sufficient for session ownership in the earliest versions. Do not build leases, native-process exclusion, terminal wrappers, or cross-process locking in this follow-up. The concurrent-writer hazards remain real and must be documented, but the early operating procedure is explicit handoff: close the previous writer before opening another. An idle terminal is still a writer; idle is not closed.

Tests must not accidentally resume a session still held by a different process. Codex's intentional multi-client test against one shared app-server is a distinct case, not permission to run independent writers against the same stored session.

## Additional product context

Roundhouse should preserve the working arrangement across outages: projects, conversations, native IDs, machine/checkouts, and reading positions. Recovery should restore visibility automatically, reconcile native history and surviving processes, and avoid automatically continuing uncertain work.

Michael also wants deliberate project suspension: stop the project's harnesses while retaining the involved native session identities, then reopen those conversations when ready. Resuming a project is not an instruction to repeat the last prompt, approve pending operations, or continue work automatically. Suspended, interrupted, and unreachable are different states.

These are design inputs. Do not build the project registry, browser UI, or production recovery service to test them.

## 1. Client disconnect and reconnect

The local owner must remain alive and continue draining the harness protocol while the browser-equivalent client disconnects. Use a tiny local shim or split the existing probe into owner and client if necessary; do not introduce a general service framework. Local IPC is sufficient. Any TCP listener must be loopback-only.

For each harness:

1. Start a persisted test conversation through a local owner and a separate client.
2. Disconnect the client during work. Confirm whether the harness keeps working and whether the owner receives its output.
3. Reconnect with a fresh client process. Recover the missed conversation events and current state without resubmitting the original prompt.
4. Repeat with an explicit input request pending: Claude question or approval, Codex approval, pi extension dialog. Reconnect, show the still-current request, answer it once, and verify the effect.
5. If a request is cancelled or resolved while the client is away, verify that reconnect does not offer it as actionable. A retained historical request is not necessarily pending.

Use request and event IDs where available. Distinguish guarantees supplied by the harness from buffering/replay supplied by the shim. In-memory buffering is sufficient for this test, provided the report says so; do not claim crash durability from it.

Record whether outputs, requests, and responses require different reconnect treatment. Do not turn this into exhaustive transport fault injection or an exactly-once delivery project.

## 2. Clean suspension and reopening

Test each harness independently, then perform one small grouped exercise with two idle managed sessions to approximate suspending a project's session roster. A static test manifest containing native IDs is sufficient; no project model is needed.

### Idle case

- Reach a completed turn, record exact native identity and working directory, and stop the managed harness cleanly.
- Verify process exit rather than assuming that closing the probe stopped the harness. Check known child processes and background work as far as the supported interfaces allow; describe limits.
- Reopen the same session with no new work prompt if supported. Determine whether history and readiness can be restored without starting model work. If the integration requires a prompt, distinguish reading stored history from activating the conversation and report the limitation.
- Send one explicit marked follow-up and verify continuity without a fork.

### Pending input and active work

- Attempt orderly suspension with a question/approval/dialog pending. Determine the safe sequence: resolve or cancel the request, interrupt, close input, or wait.
- For pi, specifically investigate cancelling the dialog through its supported response protocol before aborting. Also test a bounded dialog timeout if the installed extension API supports it. Record whether cancellation denies the fixture operation and whether abort/shutdown then completes.
- Exercise one harmless active tool operation and record how stop differs from wait-until-idle. Do not approve a tool merely to unblock shutdown.
- Verify whether any known harness or tool process remains alive. Give waits bounded deadlines; if a process persists, report it and obtain approval before escalating to a forced stop where needed.

For the grouped exercise, stop both idle conversations, retain their IDs, and reopen both with the same histories. The point is to establish lifecycle facts, not to build transactional all-or-nothing suspension. Report partial failure explicitly rather than calling the whole group suspended.

## 3. Codex questions and steering

Use the installed version's generated schema and supported collaboration modes.

- Reproduce the default-mode async question behavior only as far as necessary to identify the active turn and question representation.
- Test sending the answer with `turn/steer` into that existing turn, rather than interrupting/resuming or starting a replacement turn. Confirm whether the agent consumes the answer and stops its waiting behavior.
- Determine which supported collaboration mode enables `item/tool/requestUserInput`, if any. Test a genuine structured question, its pending-state signals, and a correlated answer.
- Record whether using that mode changes other behavior relevant to our planning and implementation agents. Do not recommend changing every agent's mode merely to obtain a convenient question UI.

If the mode or behavior is unavailable, report that result and stop. Do not patch Codex, depend on private interfaces, or force the model through repeated expensive attempts.

## Report corrections

Make these narrow corrections while updating `docs/research/harness-capabilities.md`:

- Replace claims that cold handoff is safe when the other side is merely idle with the observed condition: the previous writer is closed.
- Replace pi's label `fail-open hang` with wording that accurately describes a blocked run. The original evidence did not demonstrate authorization being bypassed.
- Distinguish process-owner termination experiments from whole-machine power loss. No tested result should claim power-loss durability.
- Record Michael's early-version operator-care decision separately from technical findings. Keep stronger ownership enforcement as a possible later direction, not an immediate requirement.

## Deliverables and completion

Update the existing capability report with a compact follow-up section: scenario, pass/partial/fail/blocked, exact versions, reproduction, observed outcome, and implications. Keep observed behavior, documentation claims, and proposed design responses distinct. Include small sanitized event excerpts where they establish a result.

Retain runnable probe changes and concise usage instructions. Add durable notes to `docs/journal/YYYY-MM-DD.md`. Summarize what the findings permit us to promise for reconnect and suspension, and any remaining blockers. Do not extend the investigation into unrelated open questions.

Continue using disposable fixtures, existing authorized authentication, and harmless operations. Do not modify real-work sessions, upgrade harnesses, change global configuration, or add spending without permission. Michael commits; leave changes uncommitted. Run `unfurl -i` on edited markdown, including this brief: the design environment could write it but could not run the formatter.
