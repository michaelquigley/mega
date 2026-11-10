import { useRef, useState } from 'react'
import { useLatest } from '../latest'
import { loadedThrough, type ConversationLog } from '../log'
import type { Conversation, Project } from '../types'
import { EventList } from './EventList'
import { InputPanel } from './InputPanel'

interface ConversationViewProps {
  conversation: Conversation
  project: Project
  log: ConversationLog | null
  stale: boolean
  onRemoved: () => void
}

const stateLabels: Record<Conversation['state'], string> = {
  none: 'No runtime',
  starting: 'Starting',
  running: 'Running',
  stopping: 'Stopping',
  resuming: 'Reading native history',
  unsupervised: 'Supervision lost',
}

// ConversationView is one conversation: a read-only header of identity and
// what the harness reports, the history, and the input panel pinned below
// it, where every control and every pending request lives.
export function ConversationView({ conversation, project, log, stale, onRemoved }: ConversationViewProps) {
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const [showNative, setShowNative] = useState(false)
  const articleRef = useRef<HTMLElement>(null)

  // history counts as loaded on the same gate the reading position uses: the
  // unbroken run from the first event reaches the newest one held.
  const delta = log?.delta ?? ''
  const loaded = log !== null && loadedThrough(log.events) === (log.events.at(-1)?.seq ?? 0)
  const activity = `${log?.events.at(-1)?.seq ?? 0}:${delta.length}:${conversation.pending.map((request) => request.id).join(',')}`
  const { newer, toLatest } = useLatest(articleRef, loaded, activity)

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

  return (
    <article className="conversation" ref={articleRef}>
      <header className="conversation-head">
        <div className="title-row">
          <h1>{conversation.name}</h1>
          {conversation.label && <span className="tag">{conversation.label}</span>}
          <span className={`state state-${state}${stale ? ' stale' : ''}`}>
            {stale ? `${stateLabels[state]} (last known)` : stateLabels[state]}
          </span>
        </div>
        <p className="facts">
          {[
            conversation.harness,
            conversation.model,
            effortFact(conversation),
            conversation.version && `v${conversation.version}`,
            conversation.mode,
            conversation.permission_mode &&
              `permissions: ${conversation.permission_mode}${conversation.permission_mode_inherited ? ' (inherited)' : ''}`,
            // the stored choice is intent for the next launch; shown when there
            // is no live mode, or apart from it when they differ.
            conversation.permission_mode_choice &&
              conversation.permission_mode_choice !== conversation.permission_mode &&
              `next launch: ${conversation.permission_mode_choice}`,
            nextPrompt(conversation),
          ]
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
      </header>

      {state === 'unsupervised' && (
        <p className="notice" role="note">
          The service restarted without seeing this conversation&apos;s runtime exit. It may have kept
          working on its own, so its execution is uncertain. Make sure nothing else holds the session,
          then resume, from the More menu below, to read its native history before continuing.
        </p>
      )}
      {project.untrusted && conversation.harness === 'claude' && (
        <p className="notice" role="note">
          Claude has not been trusted in this checkout, so it ignores the project&apos;s permission rules
          until you run <code>claude</code> there once and accept the trust dialog.
        </p>
      )}

      <EventList events={log?.events ?? []} pending={pending} turnActive={active} stale={stale} showNative={showNative} delta={delta} />
      {log === null && <p className="muted">Loading history…</p>}

      <InputPanel
        conversation={conversation}
        project={project}
        log={log}
        stale={stale}
        busy={busy}
        error={error}
        attempt={attempt}
        showNative={showNative}
        onShowNative={setShowNative}
        onRemoved={onRemoved}
        newer={newer}
        onLatest={toLatest}
      />
    </article>
  )
}

// effortFact is the effort in the header: as codex reports it, or for claude,
// which never reports it, the level asked for, labelled as such.
function effortFact(conversation: Conversation): string | undefined {
  if (conversation.effort) {
    return `effort: ${conversation.effort}`
  }
  if (conversation.harness === 'claude' && conversation.model && conversation.effort_choice) {
    return `effort: ${conversation.effort_choice} (requested)`
  }
  return undefined
}

// nextPrompt is a model or effort the next prompt carries that the header
// does not show as running yet: a change asked for while running (claude's
// effort excepted, shown as requested already), or with no runtime, the
// stored choice the next launch passes.
function nextPrompt(conversation: Conversation): string | undefined {
  const running = !!conversation.model
  const model = running ? (conversation.model_next_inherit ? 'inherit' : conversation.model_next) : conversation.model_choice
  const effort = running
    ? conversation.harness === 'claude'
      ? undefined
      : conversation.effort_next_inherit
        ? 'inherit'
        : conversation.effort_next
    : conversation.effort_choice
  const parts = [model, effort && `effort ${effort}`].filter(Boolean)
  return parts.length > 0 ? `next prompt: ${parts.join(', ')}` : undefined
}
