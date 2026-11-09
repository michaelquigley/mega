# CHANGELOG

## Unreleased

FEATURE: `roundhouse` serves a localhost workspace on `127.0.0.1:8470` where projects are registered from existing checkouts, listed, renamed, and removed without touching the checkout. The list survives a restart, a change reaches the disk before any browser is shown it, a second instance on the same port or the same data directory refuses to start, and a project whose checkout Claude has not been trusted in shows a notice rather than having its trust flag set on the operator's behalf.

FEATURE: Claude Code conversations run inside a project: launch one with an opening prompt, reply, answer its approvals and questions from any open browser (the first answer wins and a late one is refused), interrupt it, release it to continue in a terminal with the exact `claude --resume` command shown, and resume it to pick up what happened there. Native permissions are inherited, never relaxed. A conversation always resumes its own session by id, and history from the terminal is added without repeating what is already shown. After a service restart, a conversation whose runtime was not seen to stop is marked as lost supervision until it is deliberately resumed, and any request that was pending is shown as lost rather than answerable.
