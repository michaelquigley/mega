import type { Project } from './types'

// missingReason is why nothing can start in a project whose checkout cannot
// be worked in, as the service words its refusal, or null when it can.
export function missingReason(project: Project | undefined): string | null {
  if (!project?.checkout_missing) {
    return null
  }
  return `The checkout is missing: '${project.checkout}' (${project.checkout_missing})`
}
