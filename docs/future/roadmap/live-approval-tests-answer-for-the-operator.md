---
title: live approval tests answer for the operator
state: evaluating
created: 2026-11-12
tags: [defect]
---

The live round trips in `internal/e2e` answer real harness approval requests with Allow to let a test proceed. These are the delayed approval and auto-mode residue in `claude_test.go`, and the approval and MCP approval in `codex_test.go`. Terminus round `f00d5b707e1e` (2026-11-12) blocked on them under permissions-never-weakened, which forbids approving to make a test finish, in disposable fixtures or not.

They predate the project-environments batch. The reviewer saw them because that batch changed two call signatures in those files, so the finding was left for this card rather than folded in.

## shape to decide

- Live command checks get a narrow allow rule in the fixture's own checkout instead of an answer, as the environment round trip does for `Bash(./envprobe.sh)`.
- Assertions about the approval flow itself move to the fake harness, where no real operation is approved.
- A live approval exercise that must stay waits for an explicit operator answer, which makes it interactive. Or it is dropped, its evidence being the capability report.

## the planning agent's recommendation (2026-11-12)

Keep the live approval tests, and fix the rule's wording instead. The canon sentence that tripped this, "never approves or denies to make a shutdown, suspension or test finish", was written to forbid getting past an approval so that something else can be tested. These tests are the other thing: the approval round trip is their subject. They are the only evidence, against real harnesses, that a request reaches the operator, that an answer is accepted, and that the tool runs only afterwards. The fake harness cannot show that, and a narrow allow rule in the fixture removes the request the test exists to see.

What separates the two, and what the canon should say: a test may play the operator when the approval path is what it tests, it runs in a disposable fixture, it answers through the same API a browser uses, and it asserts what it is about to approve (the tool and the exact command or MCP tool it expects) before answering, failing on anything else. A test that approves whatever arrives so it can get on with checking something unrelated is the violation, and for those the fixture's own allow rule is right, as the environment round trip already does.

If Michael agrees: amend the boundary of `permissions-never-weakened` in terminus-canon with that distinction, make each of the four tests assert the request's content before answering if it does not already, and rerun the review.

