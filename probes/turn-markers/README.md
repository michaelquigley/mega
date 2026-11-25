# Each harness's turn start, against the recorded logs

Whether each harness marks the start of every turn itself, so that a turn can open on that marker alone (card `a-turn-is-inferred-from-a-message`). `check.mjs` reads the probe logs already in the repository, under `probes/claude/logs`, `probes/codex/logs` and `probes/pi/logs`, and reads only frame types and turn ids, never message text. Run it from the repository's root:

```sh
node probes/turn-markers/check.mjs
```

The markers it follows are the ones the adapters read:

- **Claude:** a turn starts at `system/init` and ends at `result`.
- **Codex:** a turn starts when Codex names it, by `turn/started` or by its reply to `turn/start`, and ends at `turn/completed`.
- **pi:** a turn starts at `agent_start` and ends at `agent_settled`.

For each harness it counts the turns with a start and an end, and lists every log with an end that had no start, a start with no end before the log ends, a start while a turn is open, and output with no turn open. For Codex, whose items name their turn, output with no turn open is an item seen before its turn was named; an item of a turn that has already ended is counted apart, as output after its turn.

## 2026-11-24, at `40da1b0`

The logs are from Claude Code 2.1.318, codex-cli 0.166.0, and pi 1.1.0 and 1.5.2.

- **Claude: 89 turns, and every one has its `system/init` before its end.** No end has no start, and no start comes while a turn is open. The two starts with no end are the probes that killed the process or suspended it mid-turn, `reconnect-killed` and `suspend-pending`. The one output with no turn open is a background subagent's message, which carries its `parent_tool_use_id`, after the turn that started it ended.
- **Codex: 54 turns, and every one is named before its end.** The three ends logged a second time are in `hold-ws`, whose two clients each logged every frame. The four starts with no end are the probes that killed the owner or the app-server, or closed it during an approval, and `rt1-managed`, whose log stops mid-turn. No item came before its turn was named. One came after its turn had ended: the background command in `background-session`, which opens no turn.
- **pi: 62 turns, and every one has its `agent_start` before its end.** The eight starts with no end are the probes that timed out, were killed or were suspended mid-turn; `r4` holds two runs, each killed. The twelve starts while a turn is open are all in `rt1-managed`, on pi 1.1.0: pi sends `agent_start` and `agent_end` again for each retry inside one run, and `agent_settled` once at its end. A start while a turn is open must therefore change nothing.

None of these logs has a model switch on an idle Claude runtime, which Claude Code answers from 2.1.322 on with a replayed user message. The capability report's follow-up of 2026-11-19 records that no `system/init`, reply or `result` follows it.

Michael's own logs were read the same way on 2026-11-24, by kind and status only. His 17 Claude conversations held 502 `system/init`s and 501 turns that ended. The one still open was the turn in flight at the time of the reading. No end had no start, and no start came while a turn was open or was followed by the runtime's exit before its end.
