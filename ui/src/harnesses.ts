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
// iconScale: how much larger than the bar's icon to draw a file whose mark
// sits inside clear space, so the mark itself fills the icon; the file is
// not changed. pi's three shapes span about 59% of their box.
// permissionModes: the harness has permission modes the operator chooses
// (claude); one without has no mode control at all.
// listsMcp: the harness reports its MCP servers to mega, which the More menu
// lists; one that does not has no MCP item.
// loneEffort: the effort control is left out when the model takes one level
// or none, rather than shown with nothing to choose (pi).
// dialogsHoldActions: while one of its dialogs is pending, no action of the
// turn without a result has run, so none is shown as running. pi raises an
// extension's dialog before the tool it gates executes, and runs nothing
// else of that batch until it is settled (pi 1.5.2, agent-loop.js).
// noLock: the harness takes no lock on a session, so mega cannot tell
// whether a terminal has it open (pi).
// notice: what the launch dialog and the conversation say about how the
// harness asks, where it differs from the others.
export interface Harness {
  value: string
  label: string
  attachesMentions: boolean
  icon?: string
  iconMono?: boolean
  iconScale?: number
  permissionModes?: boolean
  listsMcp?: boolean
  loneEffort?: boolean
  dialogsHoldActions?: boolean
  noLock?: boolean
  notice?: string
}

export const harnesses: Harness[] = [
  { value: 'claude', label: 'Claude Code', attachesMentions: true, icon: 'claude-code.svg', permissionModes: true, listsMcp: true },
  { value: 'codex', label: 'Codex', attachesMentions: false, icon: 'codex.svg', iconMono: true, iconScale: 2, listsMcp: true },
  {
    value: 'pi',
    label: 'pi',
    attachesMentions: false,
    icon: 'pi.svg',
    iconScale: 1.6,
    loneEffort: true,
    dialogsHoldActions: true,
    noLock: true,
    notice:
      'pi asks for nothing unless an extension asks: it runs its tools without asking, as it does in its own terminal. It runs its own MCP servers, which mega cannot list.',
  },
]

export const harnessOf = (value: string): Harness | undefined => harnesses.find((h) => h.value === value)

export const harnessLabel = (value: string) => harnesses.find((h) => h.value === value)?.label ?? value

// asSentence is a note the service words for its log (a harness's version
// against its tested range), begun as a sentence is on the page.
export const asSentence = (note: string | undefined) => (note ? note.charAt(0).toUpperCase() + note.slice(1) : '')
