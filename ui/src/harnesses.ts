// the harnesses a conversation can run on, as the service names them.
// attachesMentions: whether "@path" in a prompt attaches the file, as the
// harness's terminal does. claude's does; codex takes the text literally
// (docs/research/harness-capabilities.md, 2026-11-10), so the path search
// inserts the bare path for it.
export const harnesses = [
  { value: 'claude', label: 'Claude Code', attachesMentions: true },
  { value: 'codex', label: 'Codex', attachesMentions: false },
]
