// the claude permission modes an operator may choose. the bypass modes are
// never offered; the service refuses them too.
export const permissionModes = [
  { value: 'default', label: 'Default: ask for each approval' },
  { value: 'auto', label: 'Auto: a classifier approves routine calls' },
  { value: 'acceptEdits', label: 'Accept edits: file edits without asking' },
  { value: 'plan', label: 'Plan: read and plan, change nothing' },
]
