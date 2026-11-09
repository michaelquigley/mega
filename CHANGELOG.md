# CHANGELOG

## Unreleased

FEATURE: `roundhouse` serves a localhost workspace on `127.0.0.1:8470` where projects are registered from existing checkouts, listed, renamed, and removed without touching the checkout. The list survives a restart, a change reaches the disk before any browser is shown it, a second instance on the same port or the same data directory refuses to start, and a project whose checkout Claude has not been trusted in shows a notice rather than having its trust flag set on the operator's behalf.
