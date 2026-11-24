---
title: a service test writes the registry behind the service
state: researching
created: 2026-11-23
tags: [defect]
---

`TestRemoveLeavesCheckoutAndOtherProjectsAlone` in `internal/service/service_test.go` fails under the race detector, and has since at least `8503877`. Make the test change the workspace through the service, or wait for the background resolution it started, so it passes under `-race`.

## why

The test calls `Registry.Update` itself while a goroutine the service started is still reading the registry. `RegisterProject` starts a background resolution of the project's environment (`resolveStaleLocked`, `internal/service/environment.go`), which reads the registry through `projectLocked`. The product has one writer of the registry, so this is the test's fault and not a race in the service.

`make test` does not run the race detector, so the gate does not see it. An implementer reported it twice on 2026-11-23 while building `a-first-launch-holds-nothing-back`, and the design agent reproduced it the same day.

`reading-the-workspace-starts-nothing` is about these background resolutions, and its larger size removes some of them. The one a registration starts stays in both sizes, so this does not go away with that piece.

## from 2026-11-24

The implementer of `a-failed-write-stops-mega` met it again and measured it: the race detector reports it two runs in three on `4a962cd`. Two fixes in the test would do: take the service's lock around the write, or wait on the service's background work before it. A second card it captured for the same race, `a-service-test-races-with-background-resolution`, duplicates this one.
