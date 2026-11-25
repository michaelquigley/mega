---
title: the pi dialog row step reads the row too early
state: evaluating
created: 2026-11-25
tags: [defect]
---

The `conversation` suite's step "a pi action whose dialog is pending says it waits on the dialog, not that it runs" (`checks/conversation.mjs`) waits only for the `bash sleep 1` row to exist, then reads its summary and counts its spinners. The page can have the tool call before it has the pending dialog. For that moment the row shows its running spinner, and the step fails, while its own message, read a moment later, shows "bash sleep 1 waiting on a dialog". It failed once in eight runs on 2026-11-25, during the gates of `reading-the-workspace-starts-nothing`, which touches none of this. The step should wait for the row to say it is waiting on a dialog before it asserts there is no spinner.

## as built

Built on 2026-11-25. The step waits until the row says it is waiting on a dialog before it counts spinners. The race was not forced or reproduced. The fix rests on the waiting text and the spinner being exclusive branches of one render.
