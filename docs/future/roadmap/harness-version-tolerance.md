---
title: harness version tolerance
state: researching
created: 2026-11-13
tags: [story, spike]
---

Mega works with the harness versions people actually have, within a stated range, and says plainly when it is outside one. Today it names one Claude Code build and one Codex build, and nobody has recorded what happens with any other.

## why

The README asks for Claude Code 2.1.318 at `~/.local/share/claude/versions/2.1.318` and codex-cli 0.166.0. Michael holds those still on purpose. A coworker's Claude Code updates itself, and their Codex is whatever they installed last. The stage 2 gate crossed one Claude version without trouble, which is the only evidence there is. Without this, the commonest first experience of mega is a failure with no explanation.

## spike

Recorded in the capability report with versions:

- A Claude Code newer than the shim's pinned Agent SDK: what works, what breaks, and whether the SDK or the shim notices.
- Whether the shim can use the Claude Code on `PATH` instead of a versioned directory, and what that costs in certainty.
- A Codex newer and older than 0.166.0 against the app-server calls the adapter makes.
- How each harness reports its version, and how early mega can know it.

## shape to explore

- A tested range per harness, stated in the README and in `mega version`, not one exact build.
- At start-up and at launch, mega compares what it finds with the range. Inside it, nothing is said. Outside it, the launch goes ahead with a plain notice of what was tested, unless the spike finds a version that cannot work, which is refused with the reason.
- Never an upgrade or a downgrade by mega. It reports; the operator decides.

## notes from the clean-machine rehearsal, 2026-11-13

- **No fresh harness was installed.** In the agent's pass, running the downloaded Claude Code installer in the scratch home was refused by the session's permission classifier. Codex through npm into a scratch prefix was not tried, for the same reason. So the rehearsal saw no version other than the pinned ones, only none at all. Michael's WSL2 pass, or an agent pass he approves installs for, is where a fresh version is first seen.
- **A Claude other than 2.1.318 is, to mega, a Claude that is not there, and it says so badly.** The default `claude.executable` is the versioned path. A Claude Code installed today lands under another version's directory, and mega's check passes an absolute path unchecked. The first launch fails with "internal error" on the page and the SDK's "Claude Code native binary not found at …/versions/2.1.318" in the log (`a-missing-claude-is-named`). Until a range exists, the refusal could at least name the version mega expected and the path it tried.
- **An npm-installed `claude` is refused by name**, in code though not observed: a `claude` that is a script is refused with "set claude.executable to a native build, such as ~/.local/share/claude/versions/<version>". Coworkers who installed Claude Code through npm get that.
- **Codex is taken from `PATH` at any version**, with no check before the launch. What an older or newer one does is the open question above.
- **Nothing names the expected or found versions before a launch.** `mega version` prints go, target and the log path. `/api/version` adds `claude_sdk: 0.3.318`. Neither names Claude Code 2.1.318 or codex-cli 0.166.0, nor what was found.
