---
title: the panel shows the request, the turn shows what ran
state: inbox
created: 2026-11-13
tags: [enhancement]
---

Model, effort and permission mode appear in two places with two meanings and no qualifiers. The input panel shows what Michael is requesting. The conversation shows what each turn ran with. The words "inherit", "requested", "next launch" and "next prompt" leave the interface.

## why

Michael, 2026-11-13: "all of the qualifiers... are what make it confusing. i really care about what i'm requesting, and what was processed... what i'm requesting should be in the operator panel. what was executed should be in the conversation." The control bar's three selectors were answering three questions in one truncated label: what is in effect, who decided it, and what happens next. "Inherit" alone had five phrasings.

The header batch of 2026-11-12 also left a hole this closes. The header used to show what the harness reported; it no longer does, and the selectors show the stored choice, so the harness's own report is on screen nowhere. A comment in `ui/src/components/Controls.tsx` still says the header shows it.

## decided

- **The panel is the request.** Three controls, each always holding a concrete value. The closed control shows a short value, `Fable 5.2`, `high`, `auto`, and Claude's mode named `default` shows as `ask`, since "default" reads as unset; the longer descriptions belong in the open list. Nothing on the bar says where a value came from or when it applies.
- **There is no unset state.** A new conversation's controls start from the harness's current settings, copied once, and mega passes them. A later change to the Claude settings or the Codex config does not reach conversations that already exist; Michael accepted that cost for the clarity.
- **The conversation is what ran.** Each turn carries what it ran with, as compactly as possible, hidden unless the pointer is over that turn. It appears in space the turn already has, as the copy button does, so nothing moves when it shows.
- **The notes go.** "next launch" and "next prompt" leave the control bar: the panel is by definition what the next send carries.

## also decided

Proposed alongside and accepted by Michael, 2026-11-13:

- **A difference shows without hovering.** When a turn ran with something other than what was requested, its line stays visible in the notice colour and names both. Everything agreeing stays hidden. Otherwise the one case worth noticing is the one nobody sees.
- **Keyboard focus inside a turn reveals its line too**, so it is not reachable by pointer alone.
- **A mode the settings name that mega never offers.** Claude's settings can name a bypass mode. Mega never passes one, so the copy takes `default`, which asks for each approval. That is stricter than passing nothing, which today lets the settings' mode run.
- **Existing conversations are filled in once**, from what the harness last reported where the log has it, else from the settings as a new conversation would be. The service does it, as a recorded change.

## to establish before building

- **What the log already says about a turn.** It records runtime events for `started`, `model_changed` and `mode_changed`. Whether those are enough to say what each turn ran with, or the turn's own events must carry it. Older turns show nothing where the log cannot say.
- **Claude never reports effort.** The turn can only show the effort that was sent. Say so once, in the open control or the documentation, not on every turn.
- **A mode switched during a turn.** The turn shows the mode it started in; where and whether the switch shows.
- **Copying Claude's settings.** Mega reads the settings files without per-model overrides or environment variables, and a settings value may be an alias the model list does not hold. What the copy does when the value is missing, or names nothing in the list: the list's default is the obvious fallback.
- **What this removes.** The rule that Claude cannot return to inherit mid-session, Codex re-reading its config for an unset choice on every turn, the launch dialog's inherit options, `inherited_permission_mode` on the project.
- **The canon.** `permissions-never-weakened` described "inherit" as the harness's own settings. Amended to the copy-once rule on 2026-11-13, ahead of the build, for Michael's review.

## checks

The panel suite: the controls hold concrete values for a new conversation of each harness, and no control or note says inherit, requested or next. The conversation suite: a turn's line is hidden at rest, shown on hover and on focus, moves nothing when it shows, and stays visible in the notice colour when the fake reports a model other than the one requested.
