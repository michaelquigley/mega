---
title: a service test writes the registry behind the service
state: evaluating
created: 2026-11-23
tags: [defect]
---

`TestRemoveLeavesCheckoutAndOtherProjectsAlone` in `internal/service/service_test.go` fails under the race detector, and has since at least `8503877`. Make the test change the workspace through the service, or wait for the background resolution it started, so it passes under `-race`.

## why

The test calls `Registry.Update` itself while a goroutine the service started is still reading the registry. `RegisterProject` starts a background resolution of the project's environment (`resolveStaleLocked`, `internal/service/environment.go`), which reads the registry through `projectLocked`. The product has one writer of the registry, so this is the test's fault and not a race in the service.

`make test` does not run the race detector, so the gate does not see it. An implementer reported it twice on 2026-11-23 while building `a-first-launch-holds-nothing-back`, and the design agent reproduced it the same day.

`reading-the-workspace-starts-nothing` is about these background resolutions, and its larger size removes some of them. The one a registration starts stays in both sizes, so this does not go away with that piece.

## from 2026-11-24

The implementer of `a-failed-write-stops-mega` met it again and measured it: the race detector reports it two runs in three on `4a962cd`. Two fixes in the test would do: take the service's lock around the write, or wait on the service's background work before it. A second card it captured for the same race duplicated this one and was removed.

## as built

Built on 2026-11-24. The test waits for the background resolutions the registrations started, then writes the registry under the service's lock. Each does its own job. The lock covers every reader of the registry, including the watcher that reads it every two seconds, which the wait alone did not. The wait lets the resolutions finish before the removal and the close, so they do not log. Without it they both logged as the service closed, and `df/dl`'s first log call is not guarded. That fault is carded in df's own roadmap as `dl-races-on-its-first-log-call`.

The test still seeds two conversations straight into the registry. Terminus raised that as `removal-test-bypasses-service`, and Michael left it: about a dozen tests seed the registry the same way, and the canon's `single-authority` now says a test's fixture is not a second writer.

Since `reading-the-workspace-starts-nothing` was built on 2026-11-25, a registration starts no resolution, so the wait had nothing left to guard and was removed. The lock stays.
