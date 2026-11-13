// the claude permission modes an operator may choose, each with the short
// word a closed control and a turn's line use, and the description its open
// list gives. claude's default is the mode that asks, so it shows as ask.
// the bypass modes are never offered; the service refuses them too.
export const permissionModes = [
  { value: 'default', short: 'ask', label: 'Ask: ask for each approval' },
  { value: 'auto', short: 'auto', label: 'Auto: a classifier approves routine calls' },
  { value: 'acceptEdits', short: 'accept edits', label: 'Accept edits: file edits without asking' },
  { value: 'plan', short: 'plan', label: 'Plan: read and plan, change nothing' },
]

// shortMode is a mode as a closed control or a turn's line names it; a mode
// mega does not offer, as claude may report, shows as it is.
export function shortMode(mode: string | undefined): string {
  return permissionModes.find((option) => option.value === mode)?.short ?? mode ?? ''
}
