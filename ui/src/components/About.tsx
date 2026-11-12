import { newerLabel } from '../build'
import type { About } from '../types'
import { Dialog } from './Dialog'

// AboutPanel is what 'mega version' prints, from the service's build
// information, with where it logs and the Claude Agent SDK the shim runs, and
// whether this page runs the interface the service now serves.
export function AboutPanel({ about, newer, onClose }: { about: About; newer: boolean; onClose: () => void }) {
  return (
    <Dialog title="This mega" onClose={onClose}>
      <dl className="about">
        <dt>Version</dt>
        <dd>
          {about.version}
          {about.developer && ' (developer build)'}
        </dd>
        {about.commit && (
          <>
            <dt>Commit</dt>
            <dd>
              <code>{about.commit}</code>
            </dd>
          </>
        )}
        {about.built && (
          <>
            <dt>Built</dt>
            <dd>{about.built}</dd>
          </>
        )}
        {about.branch && (
          <>
            <dt>Branch</dt>
            <dd>{about.branch}</dd>
          </>
        )}
        <dt>Go</dt>
        <dd>{about.go}</dd>
        <dt>Target</dt>
        <dd>{about.target}</dd>
        <dt>Log file</dt>
        <dd>
          <code>{about.log_file}</code>
          <span className="muted">
            {about.logs_to_file ? 'This mega is writing to it.' : 'Written when mega serves with no terminal; this one logs to its terminal.'}
          </span>
        </dd>
        <dt>Claude Agent SDK</dt>
        <dd>{about.claude_sdk}</dd>
        <dt>This page</dt>
        <dd>{newer ? newerLabel : 'Running the interface this mega serves.'}</dd>
      </dl>
    </Dialog>
  )
}
