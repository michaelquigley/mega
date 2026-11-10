import { useRef, useState, type FormEvent } from 'react'
import {
  interruptConversation,
  releaseConversation,
  removeConversation,
  resumeConversation,
  sendPrompt,
  setModel,
  setPermissionMode,
  updateConversation,
} from '../api'
import { harnesses } from '../harnesses'
import { useLatest } from '../latest'
import { useModels } from '../models'
import { permissionModes } from '../permissions'
import { loadedThrough, type ConversationLog } from '../log'
import type { Conversation, Project } from '../types'
import { ActivityLine } from './ActivityLine'
import { EventList } from './EventList'
import { ModelPicker } from './ModelPicker'
import { RequestCard } from './RequestCard'

interface ConversationViewProps {
  conversation: Conversation
  project: Project
  log: ConversationLog | null
  stale: boolean
  onRemoved: () => void
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

export function ConversationView({ conversation, project, log, stale, onRemoved }: ConversationViewProps) {
  const [draft, setDraft] = useState(() => drafts.get(conversation.id) ?? '')
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

  const terminalCommands: Record<string, string> = { claude: 'claude --resume', codex: 'codex resume' }
  const terminal =
    conversation.native_id && terminalCommands[conversation.harness]
      ? `cd ${project.checkout} && ${terminalCommands[conversation.harness]} ${conversation.native_id}`
      : null

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
        {conversation.harness === 'claude' && state !== 'unsupervised' && (
          <ModeControl
            live={state === 'running' && !!conversation.permission_mode}
            mode={(state === 'running' && conversation.permission_mode) || conversation.permission_mode_choice}
            inheritedMode={project.inherited_permission_mode}
            disabled={busy || stale || state === 'starting' || state === 'stopping' || state === 'resuming' || (state === 'running' && !conversation.permission_mode)}
            onChange={(mode) =>
              void attempt(async () => {
                const { outcome, message } = await setPermissionMode(conversation.id, mode)
                if (outcome === 'refused') {
                  throw new Error(`Claude refused the switch: ${message ?? 'no reason given'}`)
                }
                if (outcome === 'unconfirmed') {
                  throw new Error(message ?? 'Claude did not confirm the switch in time.')
                }
              })
            }
          />
        )}
        {state !== 'unsupervised' && (
          <ModelControl
            key={`${conversation.model_choice ?? ''}\n${conversation.effort_choice ?? ''}`}
            conversation={conversation}
            projectId={project.id}
            disabled={busy || stale || state === 'starting' || state === 'stopping' || state === 'resuming' || (state === 'running' && !conversation.model)}
            onApply={(model, effort) =>
              attempt(async () => {
                const { outcome, message } = await setModel(conversation.id, model, effort)
                const name = harnessName(conversation.harness)
                if (outcome === 'refused') {
                  throw new Error(`${name} refused the change: ${message ?? 'no reason given'}`)
                }
                if (outcome === 'unconfirmed') {
                  throw new Error(message ?? `${name} did not confirm the change in time.`)
                }
              })
            }
          />
        )}
        <details className="edit">
          <summary>Name and label</summary>
          <RenameForm conversation={conversation} onSave={(name, label) => attempt(() => updateConversation(conversation.id, name, label))} />
        </details>
        <details className="edit">
          <summary>Remove from workspace</summary>
          <div className="remove">
            <p>
              Roundhouse forgets this conversation: its own copy of the history and its reading position are
              deleted. The checkout and the native session stay untouched, and the session can be adopted again
              from the project.
            </p>
            {state === 'unsupervised' && <p className="muted">Resume it first; its runtime may still be working.</p>}
            {state !== 'none' && state !== 'unsupervised' && <p className="muted">Release its runtime first.</p>}
            <button
              type="button"
              disabled={busy || state !== 'none'}
              onClick={() =>
                void attempt(() => removeConversation(conversation.id)).then((removed) => {
                  if (removed) {
                    drafts.delete(conversation.id)
                    onRemoved()
                  }
                })
              }
            >
              Remove
            </button>
          </div>
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

      <EventList events={log?.events ?? []} pending={pending} turnActive={active} stale={stale} showNative={showNative} delta={delta} />
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
      <ActivityLine conversation={conversation} log={log} stale={stale} />
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
          {/* while a turn runs, the activity line above already says so. */}
          {blocked && !active && <span className="muted">{blocked}</span>}
          <button type="submit" disabled={busy || !!blocked || draft.trim() === ''}>
            Send
          </button>
        </div>
      </form>
      {newer && (
        <div className="newer">
          <button type="button" onClick={toLatest}>
            Newer activity below
          </button>
        </div>
      )}
    </article>
  )
}

// ModeControl changes a claude conversation's permission mode. while the
// runtime runs (live) it switches the session; otherwise it stores the choice
// for the next launch, and an empty choice shows as not chosen. it shows
// the mode the service last confirmed, so a change shows only once recorded. a
// mode inherited from the operator's settings that roundhouse does not offer
// is shown as it is, and cannot be chosen again once left.
function ModeControl({
  live,
  mode,
  inheritedMode,
  disabled,
  onChange,
}: {
  live: boolean
  mode: string
  inheritedMode: string
  disabled: boolean
  onChange: (mode: string) => void
}) {
  const offered = permissionModes.some((option) => option.value === mode)
  let unoffered = mode
  if (mode === '') {
    unoffered = inheritedMode ? `Not chosen: inherits ${inheritedMode} from your settings` : 'Not chosen: default'
  }
  return (
    <label className="mode-control">
      Permission mode
      <select value={mode} disabled={disabled} onChange={(event) => onChange(event.target.value)}>
        {!offered && (
          <option value={mode} disabled>
            {unoffered}
          </option>
        )}
        {permissionModes.map((option) => (
          <option key={option.value} value={option.value}>
            {option.label}
          </option>
        ))}
      </select>
      <span className="muted">{live ? 'Applies from the next tool call.' : 'Applies at the next launch.'}</span>
    </label>
  )
}

const harnessName = (value: string) => harnesses.find((h) => h.value === value)?.label ?? value

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
  const model = running ? conversation.model_next : conversation.model_choice
  const effort = running
    ? conversation.harness === 'claude'
      ? undefined
      : conversation.effort_next
    : conversation.effort_choice
  const parts = [model, effort && `effort ${effort}`].filter(Boolean)
  return parts.length > 0 ? `next prompt: ${parts.join(', ')}` : undefined
}

// ModelControl changes a conversation's model and effort. while the runtime
// runs (live) it asks the harness: claude applies a change from the next
// response, codex with the next prompt. otherwise it stores the choice for the
// next launch. while live, inherit cannot be picked again once left, since
// neither harness returns to the operator's settings mid-session. it shows the
// stored choice; the header shows what the harness reports.
function ModelControl({
  conversation,
  projectId,
  disabled,
  onApply,
}: {
  conversation: Conversation
  projectId: string
  disabled: boolean
  onApply: (model: string, effort: string) => Promise<boolean>
}) {
  const { models } = useModels(projectId, conversation.harness)
  const stored = { model: conversation.model_choice ?? '', effort: conversation.effort_choice ?? '' }
  const [draft, setDraft] = useState(stored)
  const live = conversation.state === 'running' && !!conversation.model
  const changed = draft.model !== stored.model || draft.effort !== stored.effort
  let when = 'Applies at the next launch.'
  if (live) {
    when = conversation.harness === 'codex' ? 'Applies from the next prompt.' : 'Applies from the next response.'
  }
  return (
    <div className="mode-control model-control">
      <ModelPicker
        harness={conversation.harness}
        models={models}
        model={draft.model}
        effort={draft.effort}
        inheritModel={!live || stored.model === ''}
        inheritEffort={!live || stored.effort === ''}
        disabled={disabled}
        onChange={(model, effort) => setDraft({ model, effort })}
      />
      <button type="button" disabled={disabled || !changed} onClick={() => void onApply(draft.model, draft.effort)}>
        Apply
      </button>
      {changed && (
        <button type="button" disabled={disabled} onClick={() => setDraft(stored)}>
          Undo
        </button>
      )}
      <span className="muted">{when}</span>
    </div>
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
