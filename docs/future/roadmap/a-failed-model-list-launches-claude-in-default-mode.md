---
title: a failed model list launches claude in default mode
state: researching
created: 2026-11-25
tags: [defect]
---

The launch dialog shows the permission mode as the operator's pick, else the settings copy the model list returns, else `default` (`LaunchDialog.tsx`, `own.mode ?? copy?.mode ?? 'default'`). It submits that value as it shows it. When the model list fails with anything but `executable_missing`, for example a failed activation or a harness failure, there is no copy and Launch stays enabled. A launch then sends `default` as if the operator had chosen it. The service takes that as the operator's choice and does not read the native mode. A checkout whose Claude settings name `plan` starts outside plan mode, though the operator never touched the selector. That is a `permissions-never-weakened` matter. Submit an empty mode when neither the operator nor the copy supplied one, so the service reads the native mode at creation, as the copy would have.

Raised by terminus `e568a9208317` on `reading-the-workspace-starts-nothing` as blocking. The fallback dates from `6839018` (2026-11-13). Before that card, the model list already resolved the project whenever its last resolution was more than 15 seconds old, so the card did not make this reachable.
