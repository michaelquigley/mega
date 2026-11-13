// the harnesses a conversation can run on, as the service names them.
// attachesMentions: whether "@path" in a prompt attaches the file, as the
// harness's terminal does. claude's does; codex takes the text literally
// (docs/research/harness-capabilities.md, 2026-11-10), so the path search
// inserts the bare path for it.
// icon: the vendor's own mark, a file in ui/src/harness-icons (whose
// SOURCES.md says where each came from), shown on the control bar to say which
// harness a conversation runs on. a harness with none shows its label in
// words; removing a mark is removing its file and its icon line here.
// iconMono: the file is a single dark colour, so the bar draws its shape in
// the text colour, legible in either theme, rather than as it is.
// iconScale: how much larger than the bar's icon to draw a mono file whose
// mark sits inside clear space, so the mark itself fills the icon; the file
// is not changed.
export interface Harness {
  value: string
  label: string
  attachesMentions: boolean
  icon?: string
  iconMono?: boolean
  iconScale?: number
}

export const harnesses: Harness[] = [
  { value: 'claude', label: 'Claude Code', attachesMentions: true, icon: 'claude-code.svg' },
  { value: 'codex', label: 'Codex', attachesMentions: false, icon: 'codex.svg', iconMono: true, iconScale: 2 },
]

export const harnessLabel = (value: string) => harnesses.find((h) => h.value === value)?.label ?? value
