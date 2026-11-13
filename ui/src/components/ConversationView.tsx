import { useLayoutEffect, useRef, useState } from 'react'
import { useWallNow } from '../clock'
import { useLatest } from '../latest'
import type { ConversationLog } from '../log'
import type { Conversation, Project } from '../types'
import { waitingLabel, type Waiting } from '../waiting'
import { EventList } from './EventList'
import { IconButton } from './Icon'
import { InputPanel } from './InputPanel'
import { McpSummary } from './Mcp'

interface ConversationViewProps {
  conversation: Conversation
  project: Project
  log: ConversationLog | null
  // the opening read and a read of earlier history, while the page waits on them.
  loading: Waiting | null
  loadingEarlier: Waiting | null
  // load the ten turns before what is shown, or everything before it.
  onEarlier: (all: boolean) => void
  // show or hide native events; showing them refetches what is shown.
  onShowNative: (show: boolean) => void
  stale: boolean
  onRemoved: () => void
  // why new work waits, or null: prompts and starts wait, and say so.
  heldBy: string | null
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
export function ConversationView(props: ConversationViewProps) {
  const { conversation, project, log, loading, loadingEarlier, onEarlier, onShowNative, stale, onRemoved, heldBy } = props
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const showNative = log?.native ?? false
  const articleRef = useRef<HTMLElement>(null)

  // history counts as loaded on the same gate the reading position uses: the
  // opening read has landed, and what it covers reaches the newest event held.
  const delta = log?.delta ?? ''
  const loaded = log !== null && log.loaded && log.through >= (log.events.at(-1)?.seq ?? 0)
  const activity = `${log?.events.at(-1)?.seq ?? 0}:${delta.length}:${conversation.pending.map((request) => request.id).join(',')}`
  const dockRef = useRef<HTMLDivElement>(null)
  const { newer, toLatest, isFollowing } = useLatest(articleRef, dockRef, loaded, activity)

  // loading earlier history keeps the view where it was: the first row in view
  // is noted before the read, and once the rows above it are in, the view
  // moves by however far that row moved.
  const anchor = useRef<{ seq: string; top: number } | null>(null)
  const earlierPage = (all: boolean) => {
    const scroller = articleRef.current?.closest<HTMLElement>('.main')
    if (scroller && articleRef.current) {
      const top = scroller.getBoundingClientRect().top
      const rows = articleRef.current.querySelectorAll<HTMLElement>('.events [data-seq]')
      const first = [...rows].find((row) => row.getBoundingClientRect().bottom > top)
      anchor.current = first ? { seq: first.dataset.seq ?? '', top: first.getBoundingClientRect().top } : null
    }
    onEarlier(all)
  }
  const from = log?.from ?? 0
  useLayoutEffect(() => {
    const held = anchor.current
    const scroller = articleRef.current?.closest<HTMLElement>('.main')
    if (!held || !scroller || !articleRef.current) {
      return
    }
    const row = articleRef.current.querySelector<HTMLElement>(`.events [data-seq="${held.seq}"]`)
    if (row) {
      scroller.scrollTop += row.getBoundingClientRect().top - held.top
    }
    anchor.current = null
  }, [from])

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
            conversation.permission_mode && `permissions: ${conversation.permission_mode}${modeQualifier(conversation)}`,
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
        <p className="facts mcp-line">
          <McpSummary conversation={conversation} />
        </p>
        {conversation.environment && (
          <p className={`facts${conversation.environment_stale ? ' env-stale' : ''}`}>
            environment {conversation.environment.fingerprint}
            {conversation.environment.changed.length > 0 && ` · ${conversation.environment.changed.join(', ')}`}
            {conversation.environment_stale &&
              ' · started in an older environment: the project’s has changed since, and the next start picks it up'}
          </p>
        )}
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
      {project.untrusted && !project.checkout_missing && conversation.harness === 'claude' && (
        <p className="notice" role="note">
          Claude has not been trusted in this checkout, so it ignores the project&apos;s permission rules
          until you run <code>claude</code> there once and accept the trust dialog.
        </p>
      )}

      {log?.loaded && log.earlier && log.earlier.turns > 0 && (
        <Earlier turns={log.earlier.turns} events={log.earlier.events} loading={loadingEarlier} onEarlier={earlierPage} />
      )}
      <EventList events={log?.events ?? []} pending={pending} turnActive={active} stale={stale} showNative={showNative} delta={delta} />
      {(log === null || !log.loaded) && <Loading waiting={loading} />}

      <InputPanel
        conversation={conversation}
        project={project}
        log={log}
        stale={stale}
        busy={busy}
        error={error}
        attempt={attempt}
        showNative={showNative}
        onShowNative={onShowNative}
        onRemoved={onRemoved}
        newer={newer}
        onLatest={toLatest}
        dock={dockRef}
        isFollowing={isFollowing}
        heldBy={heldBy}
      />
    </article>
  )
}

// modeQualifier says where the running mode came from: the operator's
// settings, and whether claude has reported it yet or it is only what the
// launch asked for.
function modeQualifier(conversation: Conversation): string {
  const notes = [conversation.permission_mode_inherited && 'inherited', conversation.permission_mode_requested && 'requested'].filter(Boolean)
  return notes.length > 0 ? ` (${notes.join(', ')})` : ''
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

// Earlier says how much history lies above what is shown, with the two ways
// to reach it: the ten turns before, or all of it. nothing loads on scroll.
function Earlier({ turns, events, loading, onEarlier }: { turns: number; events: number; loading: Waiting | null; onEarlier: (all: boolean) => void }) {
  const busy = loading !== null && loading.error === null
  return (
    <div className="earlier">
      <span className="muted">
        {turns === 1 ? '1 earlier turn' : `${turns} earlier turns`} ({events === 1 ? '1 event' : `${events} events`})
      </span>
      <IconButton icon="arrow_upward" label="Show the ten turns before these" word="Show earlier" disabled={busy} onClick={() => onEarlier(false)} />
      <IconButton icon="vertical_align_top" label="Show the whole history" word="Show all" disabled={busy} onClick={() => onEarlier(true)} />
      {loading && <Loading waiting={loading} />}
    </div>
  )
}

// Loading says what the history is waiting for: which request, for how long,
// which attempt, and the last error.
function Loading({ waiting }: { waiting: Waiting | null }) {
  if (!waiting) {
    return <p className="muted">Loading history…</p>
  }
  return <TimedLoading waiting={waiting} />
}

function TimedLoading({ waiting }: { waiting: Waiting }) {
  const now = useWallNow()
  return (
    <p className="muted loading" role="status">
      Loading history: {waitingLabel(waiting, now)}
    </p>
  )
}
