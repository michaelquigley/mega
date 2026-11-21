---
title: rooms
state: inbox
created: 2026-11-21
tags: [feature]
---

A room is a named set of harness profiles, one login per harness, with its own projects. Each project is in at most one room. mega starts every harness process for a project with its room's profile, and the tree shows one room's projects at a time. Proposed by a user on 2026-11-21, for Michael to decide.

## why

A user runs two Claude accounts on one Mac, one for work and one personal, each in its own config directory with its own login. Today the user does it with a local override on each project that needs the second account, such as `export CLAUDE_CONFIG_DIR="$HOME/.claude-work"`. That works, because the override's variable reaches the harness process. What it does not give:

- **One setting per account.** Each project repeats the same snippet. A project without it, or with a typo, runs under the default account, and nothing on the page says so.
- **mega does not know which profile a project uses.** The variable is only text in a snippet. So:
  - the trust notice reads `~/.claude.json`, not the project's config directory;
  - the configuration snapshots take only the operator's own config directory;
  - the model catalog is cached per harness, though two accounts can offer different models.
- **One tree for both accounts.** Work and personal projects are mixed.

The failure it makes impossible: a conversation can no longer start under an account its project was not meant to use because one project's snippet was missing or wrong.

## what is already known

- **Two Claude logins coexist on macOS.** A user confirmed on 2026-11-21 that two config directories hold two separate logins, used at the same time. The build installed that day is 2.1.330.
- **Codex and pi are not yet checked.** Codex keeps its login under `CODEX_HOME` and pi under `PI_CODING_AGENT_DIR`, but nobody has observed two logins coexisting for either.
- **Much of the code already follows the environment.** The Claude settings lookup, the shim's transcript reader, the snapshot roots of all three adapters and pi's session directory read these variables from the environment they are given. Every `harness.Place` is built in one function in `internal/service/environment.go`, so launch, resume, discovery, adoption, the model list and path search all pass through one point.

## decisions taken

Taken by the user on 2026-11-21, in the session that wrote this card:

1. **Rooms are managed at runtime.** They are workspace state in `registry.yaml`, written through the service, with no restart.
2. **A project is in at most one room.** It is a `room` field on the project. The registry already refuses a checkout registered twice, so a project in two rooms cannot be configured at all, rather than detected and warned about.
3. **The room wins over the activation.** mega sets the room's variables after the activation runs. An activation that set one of them is overridden, and the project page says so. An agent can write the repository's file, so that file must not pick the account.
4. **A move is refused only when it changes a harness profile in use.** Moving a project to another room is refused when the move changes the config directory of a harness the project has conversations with. That harness's sessions live in the old directory, and a resume would not find them. Every other move goes through, so a project that uses `.claude-work` through an override today can join a room that names `.claude-work`.
5. **One room at a time in the tree.** A dropdown below "Needs you" picks the room. The projects of every other room are hidden.
6. **All three harnesses.** Claude, Codex and pi.
7. **"Needs you" covers every room.** Each entry names its room in small muted text under the conversation's name. Clicking an entry switches to its room and selects the conversation.
8. **One room is marked as the default.** There is no implicit default room.
9. **The project page shows the room and the account.** It names the project's room and the account each harness uses now.

## shape

**The room.** `{id, name, default, claude: {config_dir}, codex: {home}, pi: {agent_dir}}`. Each harness entry is optional. A harness the room does not name uses its default, from the operator's environment, as today. A project's `room` holds the room's id, so a rename keeps its projects.

**The default room.**
- Once a room exists, exactly one room is the default. The first room made is the default.
- The default is where a project goes when nothing names a room, such as a project registered through the API without `room`.
- Membership is always explicit. When the first room is made, every project joins it in the same registry write. Marking another room as the default moves no project.
- The default room cannot be deleted. Mark another room as the default first. So once rooms exist, at least one remains.
- Making the first room moves every project, so it follows the move rule. One way in always passes: make a room with no directories first. Every project then keeps the profiles it uses today, overrides included. Then make the other rooms and move projects into them.

**The API.**
- The workspace read carries the rooms.
- `POST /api/rooms`, `PATCH /api/rooms/{id}` and `DELETE /api/rooms/{id}` manage them. Each change is announced with `workspace_changed`.
- `POST /api/projects` and `PATCH /api/projects/{id}` take `room`.

**The rules.**
- A room's name is required and unique.
- Each directory starts from `~` or `/`, as `path_search.directories` does. No two rooms name the same directory for the same harness.
- A directory need not exist when the room is made, since the operator logs in afterwards. While it is missing, a launch, prompt or Start for that harness is refused. The message says how to log in: for Claude, `CLAUDE_CONFIG_DIR=… claude` in a terminal, then `/login`.
- A room with projects cannot be deleted.
- Editing a room's directory follows the move rule (decision 4) for every project in it.
- A project naming a room that no longer exists is refused as `checkout_missing` is: launches, prompts and Starts wait, and reads, rename and removal keep working.

**The environment.** The function that builds `harness.Place` sets the room's variables in `Env`, never in `Find`, so mega still finds its own programs as before. The activation's changed names, and the rule that they hold, stay the activation's alone. The `started` event's environment facts add the room, and the environment's fingerprint is taken after the room's variables are set.

**What must follow the room.** Each place that reads a harness's configuration takes the room's directory:
- The Claude trust notice, `untrustedChecker` in `cmd/mega/startup.go`. It reads `~/.claude.json` once for every project. It should read `<config_dir>/.claude.json` for a project in a room.
- pi's trust notice already reads `trust.json` from the agent directory the environment names, so it follows the room once the environment does. mega reads no Codex trust state.
- The model catalog, keyed by harness and the config directory in effect, since two accounts can offer different models. The directory is the key, not the room, because a project's own override can still set it in a room that names none.
- The account in use, keyed the same way (see the project page below).
- The configuration snapshots, with one set of roots for each room's directories.
- `mega version` and the about page, which list the rooms and their directories.

Discovery and adoption need no change of their own. They run through the room's environment, so the adopt dialog lists the room's sessions only.

**The side panel.**
- "Needs you" stays at the top and lists the waiting conversations of every room. Each entry has a second line under the conversation's name, in small muted text, that names its room. Clicking an entry switches to its room and selects the conversation. `Ctrl+/` follows the same list, so it crosses rooms too, and the tab's count stays global.
- The room picker, a dropdown, sits below "Needs you" and above the projects. It marks the default room.
- Picking a room hides every other room's projects in the tree and the rail. Their marks for unread and working output are then hidden too. So each room in the picker carries the strongest mark of its conversations, as a project's mark does for its own.
- The register dialog puts a new project in the room shown.
- The conversation header names the room.
- Which room is shown is this browser's choice, not workspace state. It follows the selection in the address, so a link to a conversation in another room switches to that room. With nothing selected, it is the room last shown, kept in `localStorage` as the panel's width is, else the default room.
- With no rooms made, there is no picker and no room line, and the page is as it is now.

**The project page.**
- It names the project's room, with a selector that follows the move rule and gives its refusal as the reason.
- For each harness, it shows the account that harness is logged in to for this project, and when that was read. For example: "Claude: you@example.com, Max, read 10:42".
- Where the account comes from. mega never reads a credential file for it:
  - **Claude:** the SDK's `accountInfo()` (SDK 0.3.318) gives the email, organization, subscription type and API provider. The shim asks for it on the query it already opens to list models, and when a runtime starts.
  - **Codex:** to establish. On 0.166.0 the capability report saw `account/updated` at a thread's start, and the app server may answer `account/read`. Neither is in the schema subset checked in.
  - **pi:** to establish. pi keeps a login per provider in `auth.json`, which mega never reads. If pi reports no account, the page says so and names the provider of the current model.
- The account is kept in memory only. It is never written to the registry, the event log or the server's log, as an environment's values are not.
- Reading the page starts nothing, as `reading-the-workspace-starts-nothing` requires. mega learns the account when a harness runs for the project anyway, and a Check control on the page asks for it. Until then, the page says it has not been read.
- The operator can log in to another account from a terminal at any time, so the page always says when the account was read.

## edge cases

- **Overrides that set the same variable.** The room wins, and the page names the override as overridden. Such overrides can then be cleared.
- **A room deleted while a browser shows it.** The browser falls back to the room marked as the default.
- **A login missing from a room's directory.** mega cannot tell without reading credentials, and it must not read them. The harness's own failure is reported as `harness_failed`, as now.
- **A room whose Claude directory is `~/.claude`.** Allowed. It names Claude's own default directory explicitly.

## open questions for Michael

1. **Scope and timing.** Rooms add a concept to the core while `simplify-the-core` is under way. Does this wait for the sequence to finish?
2. **The registry version.** `rooms` and a project's `room` are new fields. An older build would drop them on its next write, as it would drop stars. Is that acceptable, or does the version go up so an older build refuses?
3. **No way back.** The default room cannot be deleted, so a workspace that has made rooms cannot return to having none. Is that acceptable?
4. **The name.** "Room" is the user's word for it.

## before building

- Observe two Codex logins (`CODEX_HOME`) and two pi logins (`PI_CODING_AGENT_DIR`) running at the same time, and record the versions, as the user did for Claude.
- Establish how Codex 0.166.0 and pi 1.5.2 report the account in use, from their schemas and documents, with a probe where those do not settle it. Record it in the capability report.
- Show without a login that each harness process gets its room's directory, in a browser suite in `checks/`. The fake Codex and the fake pi already keep their stores under `CODEX_HOME` and `PI_CODING_AGENT_DIR`. The fake Claude shim does not read `CLAUDE_CONFIG_DIR` yet, and would have to report it.
