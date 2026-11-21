// the page's keyboard shortcuts in one place: how each is matched and how
// the page names it. on a Mac, mega's own keys are ⌃⌘ chords, and every key
// is named with Apple's symbols; a Mac takes the Ctrl keys too, so a habit
// from another machine still works. anywhere else, the Ctrl keys alone.
// none can take a keystroke that would type, since each needs Ctrl. there
// are no Alt chords: the operator's browser takes them before the page sees
// them, and on a Mac they type characters (journal, 2026-11-09).

// isMac is whether the page runs on a Mac. every browser still reports
// navigator.platform; the checks set it to drive either set of keys.
export const isMac = /Mac|iPhone|iPad/.test(navigator.platform)

// Shortcut is one key: whether a press is it, how the page names it in a
// title or a hint, and its aria-keyshortcuts value.
export interface Shortcut {
  matches: (event: KeyboardEvent) => boolean
  label: string
  aria: string
}

// macLetter is a ⌃⌘ chord on a letter, matched by the character so it holds on
// any Latin layout, and by the physical key where the layout types no Latin
// letter.
function macLetter(event: KeyboardEvent, letter: string): boolean {
  if (!isMac || !event.ctrlKey || !event.metaKey || event.altKey || event.shiftKey) {
    return false
  }
  const key = event.key.toLowerCase()
  return /^[a-z]$/.test(key) ? key === letter : event.code === `Key${letter.toUpperCase()}`
}

// macArrow is a ⌃⌘ chord on an arrow.
function macArrow(event: KeyboardEvent, arrow: 'ArrowUp' | 'ArrowDown'): boolean {
  return isMac && event.ctrlKey && event.metaKey && !event.altKey && !event.shiftKey && event.key === arrow
}

// editing is whether an element is a field that takes text: one the
// operator may be typing in, whose own keys a shortcut must not take.
export function editing(element: EventTarget | null): boolean {
  if (element instanceof HTMLTextAreaElement || (element instanceof HTMLElement && element.isContentEditable)) {
    return true
  }
  return element instanceof HTMLInputElement && !['button', 'checkbox', 'radio', 'submit', 'reset', 'range', 'color', 'file', 'image'].includes(element.type)
}

// ctrlShiftArrow is Ctrl+Shift on an arrow, outside a text field, where it
// selects text by the paragraph, and outside a select, whose arrows are its
// own.
function ctrlShiftArrow(event: KeyboardEvent, arrow: 'ArrowUp' | 'ArrowDown'): boolean {
  const own = editing(event.target) || event.target instanceof HTMLSelectElement
  return event.ctrlKey && event.shiftKey && !event.altKey && !event.metaKey && event.key === arrow && !own
}

// the Ctrl keys match the physical key (Slash, Quote, Space), so they hold
// across layouts.
export const nextWaiting: Shortcut = {
  matches: (event) => (event.ctrlKey && event.code === 'Slash') || macLetter(event, 'n'),
  label: isMac ? '⌃⌘N' : 'Ctrl+/',
  aria: isMac ? 'Control+Meta+N' : 'Control+/',
}

export const panelToggle: Shortcut = {
  matches: (event) => (event.ctrlKey && !event.altKey && !event.metaKey && event.code === 'Quote') || macLetter(event, 's'),
  label: isMac ? '⌃⌘S' : "Ctrl+'",
  aria: isMac ? 'Control+Meta+S' : "Control+'",
}

// panelFocus puts the focus on the side panel's projects, so the row keys
// work from the keyboard alone; Safari's Tab skips buttons by default. off a
// Mac it is Ctrl+Shift+L, since Ctrl+Shift+P opens a private window in
// Firefox.
export const panelFocus: Shortcut = {
  matches: (event) => (event.ctrlKey && event.shiftKey && !event.altKey && !event.metaKey && event.code === 'KeyL') || macLetter(event, 'p'),
  label: isMac ? '⌃⌘P' : 'Ctrl+Shift+L',
  aria: isMac ? 'Control+Meta+P' : 'Control+Shift+L',
}

// Ctrl+Space is kept on a Mac, though macOS takes it for switching input
// sources by default and it may never reach the page.
export const replyFocus: Shortcut = {
  matches: (event) => (event.ctrlKey && !event.altKey && !event.metaKey && !event.shiftKey && event.code === 'Space') || macLetter(event, 'r'),
  label: isMac ? '⌃⌘R' : 'Ctrl+Space',
  aria: isMac ? 'Control+Meta+R' : 'Control+Space',
}

export const expandAll: Shortcut = {
  matches: (event) => ctrlShiftArrow(event, 'ArrowUp') || macArrow(event, 'ArrowUp'),
  label: isMac ? '⌃⌘↑' : 'Ctrl+Shift+Up',
  aria: isMac ? 'Control+Meta+ArrowUp' : 'Control+Shift+ArrowUp',
}

export const collapseAll: Shortcut = {
  matches: (event) => ctrlShiftArrow(event, 'ArrowDown') || macArrow(event, 'ArrowDown'),
  label: isMac ? '⌃⌘↓' : 'Ctrl+Shift+Down',
  aria: isMac ? 'Control+Meta+ArrowDown' : 'Control+Shift+ArrowDown',
}

// the send and allow keys take Ctrl or ⌘ on every machine, as they always
// have (InputPanel matches them); only their names follow the machine.
export const sendKey = { label: isMac ? '⌘↩' : 'Ctrl+Enter', aria: isMac ? 'Meta+Enter' : 'Control+Enter' }
export const allowSessionKey = { label: isMac ? '⇧⌘↩' : 'Ctrl+Shift+Enter', aria: isMac ? 'Shift+Meta+Enter' : 'Control+Shift+Enter' }
