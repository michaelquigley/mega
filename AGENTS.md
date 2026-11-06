# Roundhouse

Roundhouse is an investigation toward a network-owned, project-oriented workspace for working directly with heterogeneous coding agents across machines. It is not an autonomous team coordinator or a replacement coding harness.

## Current assignment

Read `docs/future/agent-workspace-investigation.md`. Execute its bounded integration investigation, not a product implementation. The brief is self-contained; the originating design conversation is not required.

Record reproducible findings in `docs/research/harness-capabilities.md`, small harness-specific probes under `probes/`, and durable execution notes in `docs/journal/YYYY-MM-DD.md`. Distinguish observations from documentation claims and suggestions. Do not invent results for tests requiring Michael's interaction.

## Working rules

- Michael commits; agents do not. Leave changes in the working tree and hand off.
- Use disposable repositories and newly created test sessions. Do not alter, adopt, interrupt, or delete existing real-work sessions.
- Do not upgrade harnesses, change global configuration, expose network listeners, or change authentication without Michael's approval.
- Preserve native permission behavior. Do not bypass permissions to make a test pass.
- Keep credentials and private transcripts out of reports and committed fixtures.
- Run `unfurl -i <file>` on authored or edited markdown before handoff.
- If using Go, use `github.com/michaelquigley/df/dl` for logging and `github.com/michaelquigley/df/dd` for YAML/JSON marshaling. Do not introduce a language or shared framework merely for architectural symmetry.

The grimoire's `AGENTS.md` and `agents/agent-roles` provide broader practice orientation when available. This assignment is exploratory research, not implementation of a converged product spec.
