---
title: shutdown
state: building
created: 2026-11-10
tags: [feature]
---

now that we're starting to have a lot of projects running in parallel in roundhouse, and we're also working on roundhouse... it might be helpful to have a global gate that waits for every agent to become idle... and when that happens, give the operator the option to shut down and exit.

this will make it easier to find a quiet window to update the production roundhouse (which we're using).

## shape

- **Arming.** A control in the side panel's footer arms "shut down when idle" for the whole workspace. The service holds it, in memory only, so every browser sees it and a restart clears it. Arming and cancelling are announced through `workspace_changed`.
- **The gate holds new work back.** While armed, a prompt, a launch, an adoption and a Start are refused with a message naming the pending shutdown, so the workspace can only move toward rest. Everything that lets work finish stays available: answering a pending request, Interrupt, Release, and reading. Cancelling the shutdown is one press and lifts the gate at once.
- **What idle means.** No turn in flight, no pending request, and no background job, in any conversation. Background jobs count because a release waits for them and Claude takes one more turn when one finishes. A warm runtime that is merely idle does not hold the gate; the shutdown releases it cleanly.
- **What is still in the way.** While armed, a banner across the top of the main column, the side panel's footer and the rail say what the gate is waiting for, by count and by name: conversations with a turn in flight, requests waiting on the operator, background jobs. Each name opens its conversation. Requests are called out as the operator's to answer, since nothing else will clear them.
- **Idle reached.** The banner becomes "Every agent is idle" with Shut down and Cancel, and the page raises a browser notification and marks the tab title, so the operator need not watch for it. The gate keeps holding until he chooses.
- **Shut down.** The service performs the same orderly stop a SIGINT or SIGTERM does: every runtime released, each conversation recorded with no runtime, the listener closed, exit status 0. It is refused unless the gate is armed and the workspace is idle at that moment; a forced stop remains what it is today, Ctrl+C in the terminal.
- **After the exit.** The page that asked says roundhouse was shut down from here and when, keeps trying, and recovers by itself when the service returns. Other pages show the service as unreachable, which is true.

## rules it lives under

- The gate never answers, approves, denies or interrupts anything to reach idle (`permissions-never-weakened`). It only waits.
- The shutdown it offers is the clean path, so every conversation comes back with no runtime and nothing is unsupervised afterwards (`honest-runtime-state`).
- The armed flag is service state with one writer (`single-authority`); it is deliberately not stored, because the restart it announces is what ends it.
- The endpoint that exits the process sits behind the existing Host and Origin guard on loopback, and does nothing unless armed and idle.

## checks

Service tests: arming refuses prompt, launch, adopt and start, and allows answer, interrupt and release; idle is computed from turns, pending requests and background jobs and not from warm runtimes; shut down is refused while anything is in the way and when not armed; cancelling lifts the gate; the orderly stop records every conversation with no runtime. A server test for the endpoints and the refusal codes. The browser check with the fakes: the banner and footer counts through a turn, a request and a background job; a second page seeing the armed state; the notification and title change at idle; shut down leaving the page in its shut-down state and recovering when the scratch service is started again. `make e2e` is not needed.

## not in this card

Restarting the service from the UI; scheduling a shutdown for later; releasing or interrupting conversations on the operator's behalf; behaviour under a supervisor that restarts the process on exit, which belongs with the daemon card.

