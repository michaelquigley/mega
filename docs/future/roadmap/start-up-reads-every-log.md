---
title: start-up reads every log
state: horizon
created: 2026-11-12
tags: [enhancement]
---

At start-up the service opens and scans every conversation's event log before it serves, so start-up time grows with everything ever recorded. Read only what start-up needs: each log's last line gives its last sequence number and catches a torn tail, and the full scan, which builds the index and the set of seen native identities, waits until the conversation is first opened, prompted or imported into.

## why

Measured 2026-11-12: 3.6 seconds from process start to serving, on a workspace of 136 MB of logs after three days of use. Tolerable now and linear in the total, so a month of work makes it a long wait on every restart. Split from `opening-a-conversation-costs-its-whole-history` to keep that card small.
