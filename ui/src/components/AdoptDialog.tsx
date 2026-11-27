import { useEffect, useRef, useState, type FormEvent } from 'react'
import { ApiError, adoptSession, discoverSessions, getLook, type DiscoveredSession, type Look } from '../api'
import { ago, dateTime } from '../zone'
import { harnesses } from '../harnesses'
import type { Conversation } from '../types'
import { HarnessIcon } from './HarnessIcon'
import { IconButton } from './Icon'

interface AdoptDialogProps {
  projectId: string
  onClose: () => void
  onAdopted: (conversation: Conversation) => void
}

// what discovery found for one harness: its sessions and how many a
// conversation already holds, or why it could not look.
type Found = { sessions: DiscoveredSession[]; held: number } | { error: string; missing: boolean }

const when = (session: DiscoveredSession) => Date.parse(session.updated_at ?? session.created_at ?? '') || 0

// AdoptDialog lists the native sessions stored for the checkout that no
// conversation holds, one harness at a time, and binds the one the operator
// picks. the harness control shows each harness's mark and how many sessions
// it has, or that it cannot run here. picking a row reads how that session
// began and left off (getLook), beside the list; nothing is read for the
// rows not picked, and nothing is chosen for the operator. no runtime starts
// until the first prompt.
export function AdoptDialog({ projectId, onClose, onAdopted }: AdoptDialogProps) {
  const dialogRef = useRef<HTMLDialogElement>(null)
  const [harness, setHarness] = useState('claude')
  const [found, setFound] = useState<Record<string, Found>>({})
  const [chosen, setChosen] = useState<string | null>(null)
  const [filter, setFilter] = useState('')
  const [look, setLook] = useState<{ session: string; look: Look | null; error: string | null } | null>(null)
  const [name, setName] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)

  useEffect(() => {
    dialogRef.current?.showModal()
  }, [])

  // every harness is looked in as the dialog opens, for its count.
  useEffect(() => {
    let cancelled = false
    for (const h of harnesses) {
      discoverSessions(projectId, h.value).then(
        (answer) => !cancelled && setFound((current) => ({ ...current, [h.value]: { sessions: answer.sessions, held: answer.held ?? 0 } })),
        (failure: unknown) =>
          !cancelled &&
          setFound((current) => ({
            ...current,
            [h.value]: { error: failure instanceof Error ? failure.message : String(failure), missing: failure instanceof ApiError && failure.code === 'executable_missing' },
          })),
      )
    }
    return () => {
      cancelled = true
    }
  }, [projectId])

  // the look at the session picked, read once it is picked.
  useEffect(() => {
    if (!chosen) {
      return
    }
    let current = true
    getLook(projectId, harness, chosen).then(
      (read) => current && setLook({ session: chosen, look: read, error: null }),
      (failure: unknown) => current && setLook({ session: chosen, look: null, error: failure instanceof Error ? failure.message : String(failure) }),
    )
    return () => {
      current = false
    }
  }, [projectId, harness, chosen])

  const here = found[harness]
  const listed = here && 'sessions' in here ? [...here.sessions].sort((a, b) => when(b) - when(a)) : []
  const needle = filter.trim().toLowerCase()
  // an id copied from a conversation's header finds its session too.
  const rows = needle ? listed.filter((s) => s.preview.toLowerCase().includes(needle) || s.native_id.toLowerCase().includes(needle)) : listed
  const entry = harnesses.find((h) => h.value === harness)

  const choose = (value: string) => {
    setHarness(value)
    setChosen(null)
    setLook(null)
    setError(null)
  }

  const onSubmit = async (event: FormEvent) => {
    event.preventDefault()
    if (!chosen) {
      return
    }
    setSubmitting(true)
    setError(null)
    try {
      onAdopted(await adoptSession(projectId, harness, chosen, name))
    } catch (failure) {
      setError(failure instanceof Error ? failure.message : String(failure))
      setSubmitting(false)
    }
  }

  const shownLook = look && look.session === chosen ? look : null
  return (
    <dialog ref={dialogRef} onClose={onClose} className="adopt-dialog">
      <form onSubmit={(event) => void onSubmit(event)}>
        <div className="dialog-head">
          <h2>Adopt a session</h2>
          <IconButton icon="close" label="Cancel" onClick={onClose} />
        </div>
        <div className="adopt-controls">
          <div className="harness-switch" role="radiogroup" aria-label="Harness">
            {harnesses.map((h) => {
              const f = found[h.value]
              const cannot = f && 'error' in f
              const count = f && 'sessions' in f ? String(f.sessions.length) : cannot ? 'cannot run' : '…'
              return (
                <button
                  key={h.value}
                  type="button"
                  role="radio"
                  aria-checked={harness === h.value}
                  className={`harness-choice${harness === h.value ? ' chosen' : ''}${cannot ? ' cannot' : ''}`}
                  title={cannot && f ? `${h.label}: ${f.error}` : `${h.label}: ${count} session${count === '1' ? '' : 's'} to adopt`}
                  onClick={() => choose(h.value)}
                >
                  <HarnessIcon harness={h.value} />
                  <span className="word">{h.label}</span>
                  <span className="count">{count}</span>
                </button>
              )
            })}
          </div>
          <input
            type="search"
            className="adopt-filter"
            placeholder="Filter by first prompt or id"
            aria-label="Filter the sessions by their first prompt or id"
            value={filter}
            onChange={(event) => setFilter(event.target.value)}
          />
        </div>
        <p className="muted adopt-counts" data-counts="">
          {here === undefined
            ? `Looking for ${entry?.label ?? harness} sessions in this checkout…`
            : 'error' in here
              ? `${entry?.label ?? harness} cannot be looked in here: ${here.error}`
              : `${listed.length} session${listed.length === 1 ? '' : 's'} no conversation holds${needle ? `, ${rows.length} matching` : ''}${here.held > 0 ? `; ${here.held} left out because a conversation already holds ${here.held === 1 ? 'it' : 'them'}` : ''}.`}
        </p>
        <div className="adopt-body">
          <fieldset className="session-list">
            <legend className="visually-hidden">Sessions</legend>
            {rows.map((session) => {
              const at = when(session)
              return (
                <label key={session.native_id} className={`session-row${chosen === session.native_id ? ' chosen' : ''}`}>
                  <input type="radio" name="session" checked={chosen === session.native_id} onChange={() => setChosen(session.native_id)} />
                  <span className="session-preview">{session.preview || <em>no first prompt</em>}</span>
                  <span className="session-ago" title={at ? dateTime(new Date(at)) : undefined}>
                    {at ? ago(at) : ''}
                  </span>
                  <code className="session-id">{session.native_id}</code>
                </label>
              )
            })}
            {here && 'sessions' in here && rows.length === 0 && (
              <p className="muted">{needle ? 'No session’s first prompt or id matches.' : 'No sessions here that a conversation does not already hold.'}</p>
            )}
          </fieldset>
          <section className="session-look" aria-label="The session picked" aria-live="polite">
            {!chosen ? (
              <p className="muted">Pick a session to see how it began and how it left off.</p>
            ) : !shownLook ? (
              <p className="muted">Reading the session…</p>
            ) : shownLook.error ? (
              <p className="error">{shownLook.error}</p>
            ) : (
              shownLook.look && <LookView look={shownLook.look} />
            )}
          </section>
        </div>
        <div className="adopt-foot">
          <label>
            Name
            <input placeholder="Defaults to the first prompt" value={name} onChange={(event) => setName(event.target.value)} />
          </label>
          <p className="muted">
            {entry?.noLock
              ? 'pi has no lock, so mega cannot tell whether a terminal has this session open. Close any terminal using it before you prompt it here.'
              : 'Close any terminal that has this session open before you prompt it here; an idle terminal still holds the session.'}
          </p>
          {error && <p className="error">{error}</p>}
          <div className="actions">
            <IconButton type="submit" icon="download" label="Adopt" word="Adopt" disabled={submitting || !chosen} />
          </div>
        </div>
      </form>
    </dialog>
  )
}

// LookView is how a session began and left off: its first prompt in full (to
// a bound), its last prompt where it has more than one, and the end of its
// last reply.
function LookView({ look }: { look: Look }) {
  const cut = new Set(look.cut ?? [])
  return (
    <>
      <h3>Began with</h3>
      <pre className="look-text">
        {look.first_prompt || '(no prompt)'}
        {cut.has('first') && '…'}
      </pre>
      {look.prompts > 1 && look.last_prompt && (
        <>
          <h3>Left off with, prompt {look.prompts}</h3>
          <pre className="look-text">
            {look.last_prompt}
            {cut.has('last') && '…'}
          </pre>
        </>
      )}
      {look.last_reply && (
        <>
          <h3>The end of the last reply</h3>
          <pre className="look-text">
            {cut.has('reply') && '…'}
            {look.last_reply}
          </pre>
        </>
      )}
    </>
  )
}
