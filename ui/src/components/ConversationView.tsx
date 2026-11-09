import { useEffect, useRef, useState, type FormEvent } from 'react'
import {
  interruptConversation,
  releaseConversation,
  resumeConversation,
  sendPrompt,
  updateConversation,
} from '../api'
import type { ConversationLog } from '../log'
import type { Conversation, Project } from '../types'
import { EventList } from './EventList'
import { RequestCard } from './RequestCard'

interface ConversationViewProps {
  conversation: Conversation
  project: Project
  log: ConversationLog | null
  stale: boolean
}

// drafts stay in the browser, per conversation, and never reach the service.
const drafts = new Map<string, string>()

const stateLabels: Record<Conversation['state'], string> = {
  none: 'No runtime',
  starting: 'Starting',
  running: 'Running',
  stopping: 'Stopping',
  resuming: 'Reading native history',
  unsupervised: 'Supervision lost',
}

export function ConversationView({ conversation, project, log, stale }: ConversationViewProps) {
  const [draft, setDraft] = useState(() => drafts.get(conversation.id) ?? '')
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const [showNative, setShowNative] = useState(false)
  const endRef = useRef<HTMLDivElement>(null)
  const scrollerRef = useRef<HTMLDivElement>(null)

  const eventCount = log?.events.length ?? 0
  const delta = log?.delta ?? ''
  useEffect(() => {
    const scroller = scrollerRef.current?.closest('.main')
    if (!scroller) {
      return
    }
    const nearBottom = scroller.scrollHeight - scroller.scrollTop - scroller.clientHeight < 200
    if (nearBottom) {
      endRef.current?.scrollIntoView({ block: 'end' })
    }
  }, [eventCount, delta])

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

  const { state, turn_active: active, pending } = conversation
  const hasRuntime = state === 'running' || state === 'stopping'
  let blocked: string | null = null
  if (state === 'unsupervised') {
    blocked = 'Resume this conversation before sending a prompt.'
  } else if (state === 'starting' || state === 'stopping' || state === 'resuming') {
    blocked = `${stateLabels[state]}…`
  } else if (active) {
    blocked = 'Working. Wait for the turn to end, or interrupt it.'
  }

  const onSend = async (event?: FormEvent) => {
    event?.preventDefault()
    const text = draft.trim()
    if (!text || blocked) {
      return
    }
    if (await attempt(() => sendPrompt(conversation.id, text))) {
      setDraft('')
      drafts.delete(conversation.id)
    }
  }

  const terminal =
    conversation.harness === 'claude' && conversation.native_id
      ? `cd ${project.checkout} && claude --resume ${conversation.native_id}`
      : null

  return (
    <article className="conversation" ref={scrollerRef}>
      <header className="conversation-head">
        <div className="title-row">
          <h1>{conversation.name}</h1>
          {conversation.label && <span className="tag">{conversation.label}</span>}
          <span className={`state state-${state}${stale ? ' stale' : ''}`}>
            {stale ? `${stateLabels[state]} (last known; reconnecting)` : stateLabels[state]}
          </span>
        </div>
        <p className="facts">
          {[conversation.harness, conversation.model, conversation.version && `v${conversation.version}`, conversation.mode, conversation.permission_mode && `permissions: ${conversation.permission_mode}`]
            .filter(Boolean)
            .join(' · ')}
        </p>
        <p className="facts">
          <code>{project.checkout}</code>
          {conversation.native_id && (
            <>
              {' · '}
              <code>{conversation.native_id}</code>
            </>
          )}
        </p>
        <details className="edit">
          <summary>Name and label</summary>
          <RenameForm conversation={conversation} onSave={(name, label) => attempt(() => updateConversation(conversation.id, name, label))} />
        </details>
      </header>

      {state === 'unsupervised' && (
        <p className="notice" role="note">
          The service restarted without seeing this conversation&apos;s runtime exit. It may have kept
          working on its own, so its execution is uncertain. Make sure nothing else holds the session,
          then resume to read its native history before continuing.
        </p>
      )}
      {project.untrusted && conversation.harness === 'claude' && (
        <p className="notice" role="note">
          Claude has not been trusted in this checkout, so it ignores the project&apos;s permission rules
          until you run <code>claude</code> there once and accept the trust dialog.
        </p>
      )}

      <EventList events={log?.events ?? []} pending={pending} turnActive={active} stale={stale} showNative={showNative} />
      {delta && <p className="message assistant streaming">{delta}</p>}
      {log === null && <p className="muted">Loading history…</p>}

      {pending.map((request) => (
        <RequestCard key={request.id} conversationId={conversation.id} request={request} />
      ))}

      <section className="controls">
        <button type="button" disabled={busy || !(active || pending.length > 0) || !hasRuntime} onClick={() => void attempt(() => interruptConversation(conversation.id))}>
          Interrupt
        </button>
        <button type="button" disabled={busy || state !== 'running'} onClick={() => void attempt(() => releaseConversation(conversation.id))}>
          Release
        </button>
        <button
          type="button"
          disabled={busy || !(state === 'none' || state === 'unsupervised') || !conversation.native_id}
          onClick={() => void attempt(() => resumeConversation(conversation.id))}
        >
          Resume
        </button>
        <label className="toggle">
          <input type="checkbox" checked={showNative} onChange={(event) => setShowNative(event.target.checked)} />
          Native events
        </label>
      </section>
      <p className="handoff muted">
        To continue in a terminal, release first: the runtime stops and the session is free.
        {terminal && (
          <>
            {' '}Then run <code>{terminal}</code>.
          </>
        )}{' '}
        Close the terminal before you resume or prompt here again; an idle terminal still holds the session.
      </p>

      {error && <p className="error">{error}</p>}
      <form className="reply" onSubmit={(event) => void onSend(event)}>
        <textarea
          rows={4}
          placeholder={blocked ?? 'Reply (Ctrl+Enter to send)'}
          value={draft}
          onChange={(event) => {
            setDraft(event.target.value)
            drafts.set(conversation.id, event.target.value)
          }}
          onKeyDown={(event) => {
            if (event.key === 'Enter' && (event.ctrlKey || event.metaKey)) {
              event.preventDefault()
              void onSend()
            }
          }}
        />
        <div className="actions">
          {blocked && <span className="muted">{blocked}</span>}
          <button type="submit" disabled={busy || !!blocked || draft.trim() === ''}>
            Send
          </button>
        </div>
      </form>
      <div ref={endRef} />
    </article>
  )
}

function RenameForm({ conversation, onSave }: { conversation: Conversation; onSave: (name: string, label: string) => Promise<boolean> }) {
  const [name, setName] = useState(conversation.name)
  const [label, setLabel] = useState(conversation.label)
  const unchanged = name.trim() === conversation.name && label.trim() === conversation.label
  return (
    <form
      className="inline-form"
      onSubmit={(event) => {
        event.preventDefault()
        void onSave(name, label)
      }}
    >
      <input aria-label="Name" value={name} onChange={(event) => setName(event.target.value)} />
      <input aria-label="Label" placeholder="Label, such as planning" value={label} onChange={(event) => setLabel(event.target.value)} />
      <button type="submit" disabled={name.trim() === '' || unchanged}>
        Save
      </button>
    </form>
  )
}
