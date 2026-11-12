import { useEffect, useState } from 'react'
import {
  approveEnvironment,
  clearEnvironmentOverride,
  getApprovedVersion,
  resolveEnvironment,
  setEnvironmentOverride,
  type ApprovedVersion,
  type Resolved,
} from '../api'
import type { Project, ProjectEnvironment } from '../types'
import { IconButton } from './Icon'

const when = (at?: string) => (at ? new Date(at).toLocaleString() : 'unknown')

// EnvironmentSection is the project's shell environment on its page: the
// repository's .mega/environment.yaml and whether it is approved, both
// versions side by side when it changed after approval, the local override,
// the variables the activation changes (their values only on request, from a
// live resolution held in this page alone), when it last resolved, and what a
// harness cannot find in it.
export function EnvironmentSection({ project }: { project: Project }) {
  const env = project.environment
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const [values, setValues] = useState<Resolved | null>(null)
  const [draft, setDraft] = useState(project.environment?.override?.activate ?? '')

  if (!env) {
    return null
  }

  const attempt = async (action: () => Promise<unknown>) => {
    setError(null)
    setBusy(true)
    try {
      await action()
      return true
    } catch (failure) {
      setError(failure instanceof Error ? failure.message : String(failure))
      return false
    } finally {
      setBusy(false)
    }
  }
  const showValues = () =>
    void attempt(async () => {
      setValues(await resolveEnvironment(project.id, true))
    })

  return (
    <section className="environment" aria-label="Environment">
      <div className="section-head">
        <h2>Environment</h2>
        <span className="section-actions">
          <IconButton icon="refresh" label="Resolve the environment now" disabled={busy || !!env.refusal} onClick={() => void attempt(() => resolveEnvironment(project.id, false))} />
          {values ? (
            <IconButton icon="visibility_off" label="Hide the values" onClick={() => setValues(null)} />
          ) : (
            <IconButton icon="visibility" label="Show the values of the variables the activation changes" disabled={busy || !!env.refusal} onClick={showValues} />
          )}
        </span>
      </div>
      <Summary env={env} />
      {env.refusal && (
        <p className="notice env-refusal" role="alert">
          {env.refusal.message}
        </p>
      )}
      {env.status === 'changed' ? (
        <Compare projectId={project.id} env={env} busy={busy} onApprove={(fp) => void attempt(() => approveEnvironment(project.id, fp))} />
      ) : (
        env.file.present && (
          <FileBox env={env} onApprove={env.status === 'unapproved' ? (fp) => void attempt(() => approveEnvironment(project.id, fp)) : undefined} busy={busy} />
        )
      )}
      <Resolution env={env} values={values} />
      {env.missing.length > 0 && (
        <ul className="env-missing">
          {env.missing.map((m) => (
            <li key={m.harness} className="notice">
              {m.reason}
            </li>
          ))}
        </ul>
      )}
      <details className="env-override" open={!!env.override}>
        <summary>Local override</summary>
        <p className="muted">
          Your own choice for this project on this machine, kept in the workspace, not the repository. It needs no approval.
        </p>
        {env.override ? (
          <p>
            {env.override.ignore ? 'The repository’s file is ignored here.' : 'This activation runs in place of the repository’s file:'}
            {!env.override.ignore && <pre className="env-file">{env.override.activate}</pre>}
          </p>
        ) : null}
        <span className="actions-row">
          {env.override ? (
            <IconButton icon="undo" label="Use the repository's file again" word="Use the file" disabled={busy} onClick={() => void attempt(() => clearEnvironmentOverride(project.id))} />
          ) : null}
          {!env.override?.ignore && (
            <IconButton icon="block" label="Ignore the repository's file here" word="Ignore the file" disabled={busy} onClick={() => void attempt(() => setEnvironmentOverride(project.id, { ignore: true }))} />
          )}
        </span>
        <label className="env-activate">
          Another activation, run in the checkout
          <textarea rows={2} value={draft} placeholder="devprj" onChange={(event) => setDraft(event.target.value)} />
        </label>
        <IconButton
          icon="save"
          label="Run this activation in place of the repository's file"
          word="Save"
          disabled={busy || draft.trim() === '' || draft.trim() === (env.override?.activate ?? '')}
          onClick={() => void attempt(() => setEnvironmentOverride(project.id, { ignore: false, activate: draft }))}
        />
      </details>
      {error && <p className="error">{error}</p>}
    </section>
  )
}

// Summary says where the environment comes from, in a sentence.
function Summary({ env }: { env: ProjectEnvironment }) {
  switch (env.source) {
    case 'none':
      return (
        <p>
          No <code>.mega/environment.yaml</code> in the checkout: agents here run in your shell&apos;s own environment, as a new
          terminal opened in the checkout has it.
        </p>
      )
    case 'ignored':
      return <p>The repository&apos;s file is ignored here: agents run in your shell&apos;s own environment.</p>
    case 'override':
      return <p>A local activation, your override, runs in place of the repository&apos;s file.</p>
    default:
      if (env.status === 'ready') {
        return (
          <p>
            From <code>{env.file.path}</code>, approved {when(env.approved?.at)}.
          </p>
        )
      }
      return (
        <p>
          From <code>{env.file.path}</code>, which mega will not run until you approve it.
        </p>
      )
  }
}

// FileBox is the file as it is now: its content, its fingerprint, when it
// last changed, and Approve while it waits on approval.
function FileBox({ env, onApprove, busy }: { env: ProjectEnvironment; onApprove?: (fingerprint: string) => void; busy: boolean }) {
  return (
    <div className="env-version">
      <p className="facts">
        {fileApproval(env)} · changed {when(env.file.modified_at)} · {env.file.fingerprint}
      </p>
      <pre className="env-file">{env.file.content}</pre>
      {onApprove && env.file.fingerprint && (
        <IconButton icon="task_alt" label="Approve this version, so mega runs it" word="Approve" disabled={busy} onClick={() => onApprove(env.file.fingerprint!)} />
      )}
    </div>
  )
}

// fileApproval says whether the file as it is now is the approved one, from
// its fingerprint alone: an override that makes the environment ready says
// nothing about the file.
function fileApproval(env: ProjectEnvironment): string {
  if (env.file.malformed) {
    return 'cannot be used'
  }
  if (!env.approved) {
    return 'not approved'
  }
  return env.approved.fingerprint === env.file.fingerprint ? 'approved' : 'changed since it was approved'
}

// Compare shows the approved version beside the current one, so the change
// is seen before it is approved. the approved version is found in the
// checkout's git history by its fingerprint; it is never stored.
function Compare({ projectId, env, busy, onApprove }: { projectId: string; env: ProjectEnvironment; busy: boolean; onApprove: (fingerprint: string) => void }) {
  const [approved, setApproved] = useState<ApprovedVersion | null>(null)
  const [error, setError] = useState<string | null>(null)
  const key = `${env.approved?.fingerprint}:${env.file.fingerprint}`
  useEffect(() => {
    let cancelled = false
    getApprovedVersion(projectId)
      .then((version) => !cancelled && setApproved(version))
      .catch((failure: unknown) => !cancelled && setError(failure instanceof Error ? failure.message : String(failure)))
    return () => {
      cancelled = true
    }
  }, [projectId, key])
  return (
    <div className="env-compare">
      <div className="env-version">
        <h3>Approved {when(env.approved?.at)}</h3>
        <p className="facts">{env.approved?.fingerprint}{approved?.found ? ` · from ${approved.found}` : ''}</p>
        {approved?.available ? (
          <pre className="env-file">{approved.content}</pre>
        ) : (
          <p className="muted">
            {error ??
              (approved
                ? 'This version is not in the checkout’s git history, so it cannot be shown; mega keeps only its fingerprint.'
                : 'Looking for it in git history…')}
          </p>
        )}
      </div>
      <div className="env-version">
        <h3>Now, changed {when(env.file.modified_at)}</h3>
        <p className="facts">{env.file.fingerprint}</p>
        <pre className="env-file">{env.file.content}</pre>
        {env.file.fingerprint && (
          <IconButton icon="task_alt" label="Approve the changed version, so mega runs it" word="Approve" disabled={busy} onClick={() => onApprove(env.file.fingerprint!)} />
        )}
      </div>
    </div>
  )
}

// Resolution is the latest resolution: when, its fingerprint and the names
// the activation changed, or why it failed; and the values, only while the
// operator has asked to see them.
function Resolution({ env, values }: { env: ProjectEnvironment; values: Resolved | null }) {
  const r = env.resolved
  if (!r) {
    return <p className="muted">Not resolved yet: it resolves when something starts here, or on request.</p>
  }
  if (r.error) {
    return (
      <p className="notice" role="note">
        Last resolution, {when(r.at)}: {r.error}
      </p>
    )
  }
  return (
    <div className="env-resolution">
      <p className="facts">
        Last resolved {when(r.at)} · {r.fingerprint} · {r.changed.length === 0 ? 'the activation changes nothing' : `changes ${r.changed.join(', ')}`}
      </p>
      {values && (
        <table className="env-values">
          <tbody>
            {values.changed.map((name) => (
              <tr key={name}>
                <th scope="row">{name}</th>
                <td>
                  <code>{values.values?.[name] ?? '(unset)'}</code>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  )
}
