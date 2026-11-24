---
title: start-up reads every log
state: horizon
created: 2026-11-12
tags: [enhancement]
---

At start-up the service opens and scans every conversation's event log before it serves, so start-up time grows with everything ever recorded. Read only what start-up needs: each log's last line gives its last sequence number and catches a torn tail, and the full scan, which builds the index and the set of seen native identities, waits until the conversation is first opened, prompted or imported into.

## why

Measured 2026-11-12: 3.6 seconds from process start to serving, on a workspace of 136 MB of logs after three days of use. Tolerable now and linear in the total, so a month of work makes it a long wait on every restart. Split from the work that made a conversation open on its recent tail, to keep that work small.

## since

The workspace was 185 MB on 2026-11-13 and 223 MB on 2026-11-16, about 12 MB a day at the present rate of use.

## something new that leans on the full read, 2026-11-17

The last-instructed mark shows the agent last instructed in each project. Its time is not stored anywhere of its own: when a conversation's log is opened, the service takes it from the newest prompt it finds there, and start-up opens every log. That is free today and is exactly the read this card removes. When this card is worked, that time needs another source, such as the registry, or it will be missing after every restart until each conversation is next prompted.

The workspace was 258 MB on this date.

## not part of the simpler core, 2026-11-19

The first draft of the work order behind `simplify-the-core` took this card in as a stage: checkpoints in each log, and a start-up that reads back to the last one. Three review rounds showed what that costs: a new kind of line in the log that older builds must read safely, a name for each version of the fold, and the turn index and the list of native message ids built late because the full read no longer builds them. It is a speed feature and was cut from work whose point is to remove things. It stays here.

That work was to leave behind one function, `apply(state, event)`, used live and at start-up, which would have made a checkpoint that function's state written out. It was dropped from `simplify-the-core` on 2026-11-24, so a design here starts without it. The findings above are still the list of what it has to answer.
