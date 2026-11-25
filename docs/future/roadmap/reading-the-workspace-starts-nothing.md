---
title: reading the workspace starts nothing
state: building
created: 2026-11-19
tags: [story]
---

A project's environment is resolved when something needs it or when the operator asks, and at no other time. Reading the workspace starts nothing, and neither does starting mega or registering a project. The background resolution of environments is removed.

Sixth in the sequence of `simplify-the-core`, and next to be built, in the larger of its two sizes. The card is the whole plan; there is no work order. Michael tries it in use before it is committed.

## why

Resolving a project's environment runs its approved activation in a login shell. Mega does that in the background at four moments: at start-up for every project, when a project is registered, when an activation is approved or changed, and whenever anything reads the workspace and finds a project with no current resolution. The tray reads the workspace each time the service announces a change, and a finished resolution announces a change. So, in the words of the independent review of 2026-11-19, "updating the tray icon can cause a project's approved activation to run."

Only an approved activation ever runs, so no promise is broken. What is wrong is the shape: a read that has effects, work mega starts for itself so that it can show the result later, and a shutdown that has to wait for it. On Michael's machine ten of sixteen projects have an approved activation, so every start of mega runs ten login shells.

The card first offered two sizes: only stop the read from starting a resolution, or also stop resolving in the background at all. Michael took the larger on 2026-11-25: "in general, making the model simpler is the goal and this sounds like a decent simplification." What the background run feeds is the environment section of a project's page, and of that he said: "i don't even routinely look at that section."

## the failure that becomes impossible

A read of the workspace can no longer run anything, and mega can no longer run a project's activation that nothing asked for.

## what changes

- **Removed:** `resolveStaleLocked`, `resolveAllLocked` and `resolveRegisteredLocked`, with their calls at start-up, in `Workspace` and in `RegisterProject`.
- **One way to resolve is left,** the direct one, `resolveProject`, which returns its result to whatever asked. It is reached as now by a conversation's start, by a helper that runs a harness for a project (the launch and adopt dialogs' reads among them), by an approval, and by the operator's own request in the environment section.
- **State that existed only for a background resolution goes with it,** where nothing else reads it: the limit on how many run at once, as it applies to projects, and the sentence on the page that a resolution is running now.
- **The environment section of a project not yet resolved says so,** with the sentence it already has, until one of the things above resolves it.
- **The wait in `TestRemoveLeavesCheckoutAndOtherProjectsAlone` goes if it guards nothing,** since a registration then starts no resolution. Its lock stays.

## what it costs

After a restart, the environment section of a project the operator has not touched says it is not resolved yet, where today it shows the result of the run at start-up. The first thing that needs that environment waits while its activation runs, once.

The note on a conversation that it "started in an older environment" appears once something has resolved the project again, where today the run at start-up supplies the comparison.

## what does not change

A refusal the file alone decides, an environment file that is unapproved, changed or malformed, is known without running anything and still shows at once. The watch on environment files, which announces a change and runs nothing. What a start, a helper, an approval or the operator's request does. The other work mega starts in the background: reading harness versions and taking configuration snapshots.

## how it is shown

- reading the workspace runs no activation: not at start-up, not after a registration, not after an environment file changes, and not however many times it is read;
- starting mega with approved projects runs none;
- a conversation's start resolves its project once, and the section then shows the result;
- opening the launch dialog for a project not yet resolved resolves it;
- the operator's request in the environment section resolves it;
- a project whose file is unapproved shows its refusal with nothing run;
- nothing in the tree names `resolveStaleLocked`, `resolveAllLocked` or `resolveRegisteredLocked`.

## the trial

The implementer leaves the piece in the working tree, with every gate passed, and says so. Michael builds it, restarts on it, and tries it against his projects. He looks at one thing: whether the first launch dialog opened for a project after the restart feels slower, since that is now when its activation runs.

## how it is run

- **It starts by listing everything that reads the latest resolution of a project,** on the service and on the page, and what each shows when there is none. One that shows something untrue with none, where it could say "not resolved yet", is brought to Michael.
- **Documents:** "Project environments" in `docs/current/mega.md`, above all its "Resolution" part, the glossary's entry for a project environment, and the changelog. The spec's paragraph "Reading the workspace starts nothing" is the planning agent's to retire once Michael has kept the piece.
- **Tests and check steps of the removed behaviour go, or are rewritten to the rule.** The report names each. A test or a check that fails for any other reason is a change in behaviour, and stops and asks.
- **The gates are the epic's:** `make test`, all of `make checks`, `make e2e` with the harness versions and every skip named, and terminus on the working tree, then Michael. Run them from a disposable copy of the tree, so the Makefile's reinstall of the shim leaves Michael's running mega alone.
- **A terminus finding in code this card did not touch and did not make newly reachable** is matched to an existing card or captured in inbox, and listed at handoff.
- **A finding that is only wording** in a doc or a test of this card's own is fixed and reported at handoff.
- **A finding whose fix would add state, a hold or an ordering rule** is brought to Michael as a question of whether to fix it at all. Anything else this card caused comes to him one at a time.
- **Size.** `internal/service` was 6,171 non-test lines when piece 4 was built. The report gives the figure afterwards, and what was deleted outright.

## order

Depends on nothing. It does not touch the registry or the logs. Lands on `main`.
