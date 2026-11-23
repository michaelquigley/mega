import { useState, type ReactNode, type SyntheticEvent } from 'react'
import { getInvocation } from '../api'
import { harnessOf } from '../harnesses'
import { longElapsed } from '../clock'
import { fullTime, parse, shortTime } from '../zone'
import { shortModel, useModels } from '../models'
import { shortMode } from '../permissions'
import type { ConversationEvent, Invocation, Models, PendingRequest, TurnRecord } from '../types'
import { Spinner } from './AttentionMark'
import { BlockCopy, CopyButton } from './Copy'
import { Markdown } from './Markdown'

interface EventListProps {
  events: ConversationEvent[]
  pending: PendingRequest[]
  turnActive: boolean
  stale: boolean
  showNative: boolean
  // the streaming text of the turn in flight, shown as the reply it will land
  // as, in the same block and place.
  delta: string
  projectId: string
  conversationId: string
  harness: string
  onTranscript?: () => void
  // what the turn in flight asked for and ran with, once it has run.
  inFlight?: TurnRecord
}

const outcomeLabels: Record<string, string> = {
  answered: 'answered',
  aborted: 'aborted',
  lost: 'lost when the service restarted',
  // a pi dialog whose own time limit ran out: pi settled it with no answer.
  expired: 'expired: its time ran out before an answer, and pi went on without one',
  // a pi dialog pending when its runtime was ended, as a release ends pi
  // with a dialog waiting: the command it gated did not run.
  unanswered: 'never answered: the runtime was ended while it waited, and what it gated did not run',
  // a codex steer whose reply never came, by its deadline or before the
  // runtime went: the answer was sent, and may or may not have been taken.
  unconfirmed: 'Codex did not confirm whether it received your answer',
}

// Began is when a turn began: its prompt's time, or its first event's when
// it has none. a prompt from outside mega was imported, and its time is when
// mega imported it, not when it was sent.
interface Began {
  at: Date
  prompt: boolean
  imported: boolean
}

function historyBoundary(event: ConversationEvent): boolean {
  return event.kind === 'runtime' && (event.gap === true || event.status === 'unsupervised')
}

// EventList renders recorded history. requests appear here only as history;
// whether one is answerable comes from the service's pending set, never from
// the log.
export function EventList({ events, pending, turnActive, stale, showNative, delta, projectId, conversationId, harness, inFlight, onTranscript }: EventListProps) {
  const { models } = useModels(projectId, harness)
  const results = new Map<string, ConversationEvent>()
  const outcomes = new Map<string, string>()
  for (const event of events) {
    if (event.kind === 'tool_result' && event.tool_use_id) {
      results.set(event.tool_use_id, event)
    }
    if (event.kind === 'request_resolved' && event.req?.outcome) {
      outcomes.set(event.req.id, event.req.outcome)
    }
  }
  // a mode request Claude went on to confirm draws no row of its own: its
  // confirmation, the next mode event, says the same. a request nothing
  // confirmed (stored with no runtime, refused, put back, or unconfirmed)
  // keeps its row.
  const confirmed = new Set<number>()
  let nextMode: ConversationEvent | undefined
  for (let i = events.length - 1; i >= 0; i--) {
    const event = events[i]
    if (event.kind !== 'runtime' || (event.status !== 'mode_requested' && event.status !== 'mode_changed')) {
      continue
    }
    if (event.status === 'mode_requested' && nextMode?.status === 'mode_changed' && nextMode.facts?.permission_mode === event.facts?.permission_mode) {
      confirmed.add(event.seq)
    }
    nextMode = event
  }
  // when each turn began, by the seq of its end, and when the turn in flight
  // did. a runtime line between turns is not the turn's own.
  const began = new Map<number, Began>()
  let open: Began | undefined
  for (const event of events) {
    if (event.kind === 'turn_end') {
      if (open) {
        began.set(event.seq, open)
      }
      open = undefined
      continue
    }
    if (event.kind === 'runtime') {
      if (historyBoundary(event)) open = undefined
      continue
    }
    const at = parse(event.at)
    if (at && (!open || (!open.prompt && event.kind === 'user'))) {
      open = { at, prompt: event.kind === 'user', imported: event.origin === 'native' }
    }
  }
  const toolUses = new Set(events.filter((e) => e.kind === 'tool_use' && e.tool_use_id).map((e) => e.tool_use_id))
  const pendingIds = new Set(pending.map((request) => request.id))
  // a harness whose pending dialog holds every action of the turn without a
  // result (pi): none of them has run, so none is shown as running.
  const held = pending.length > 0 && !!harnessOf(harness)?.dialogsHoldActions
  // only a tool in the turn the service reports in flight can still be running;
  // an older one without a result simply has none recorded.
  let turnStart = -1
  events.forEach((event, index) => {
    if (event.kind === 'turn_end' || event.kind === 'runtime') {
      turnStart = index
    }
  })

  const render = (event: ConversationEvent, index: number): ReactNode => {
    switch (event.kind) {
      case 'user':
        return (
          <div key={event.seq} data-seq={event.seq} className="message user" title={promptTitle(event)}>
            <BlockCopy source={event.text ?? ''} />
            {/* the operator's own prompts carry no label: the block's tint says
                whose they are. one sent from outside mega says so. */}
            {event.origin === 'native' && <span className="who">outside mega</span>}
            <Markdown text={event.text ?? ''} />
          </div>
        )
      case 'assistant':
        return (
          <div key={event.seq} data-seq={event.seq} className="message assistant">
            <BlockCopy source={event.text ?? ''} />
            {event.origin === 'native' && <span className="who">From outside mega</span>}
            <Markdown text={event.text ?? ''} />
          </div>
        )
      case 'tool_use': {
        const result = event.tool_use_id ? results.get(event.tool_use_id) : undefined
        // a call reported after its own turn ended is never the turn in
        // flight's, whatever it follows.
        const live = turnActive && index > turnStart && !event.after_turn
        let state: ReactNode = null
        if (result?.status === 'error') {
          state = <span className="tool-error"> failed</span>
        } else if (!result && live && held) {
          state = <span className="muted"> waiting on a dialog</span>
        } else if (!result && live && !stale) {
          state = (
            <>
              {' '}
              <Spinner label="Running" />
            </>
          )
        } else if (!result) {
          state = <span className="muted">{live ? ' running when last seen' : ' no result recorded'}</span>
        }
        return <ToolRow key={event.seq} event={event} result={result} state={state} conversationId={conversationId} />
      }
      case 'tool_result':
        if (event.tool_use_id && toolUses.has(event.tool_use_id)) {
          return null
        }
        return (
          <details key={event.seq} data-seq={event.seq} className="tool">
            <summary>Tool result</summary>
            {event.text && <ToolOutput text={event.text} />}
          </details>
        )
      case 'request': {
        const id = event.req?.id ?? ''
        const outcome = outcomes.get(id)
        const payload = event.req?.payload
        const what = event.req?.kind === 'question' ? 'Question' : `Approval for ${payload?.tool ?? 'a tool'}`
        let status = 'not pending'
        if (pendingIds.has(id)) {
          status = 'waiting for you'
        } else if (outcome) {
          status = outcomeLabels[outcome] ?? outcome
        }
        return (
          <p key={event.seq} data-seq={event.seq} className="system">
            {what}: {status}
          </p>
        )
      }
      case 'runtime':
        if (event.status === 'mode_changed') {
          // in logs from before every launch passed a mode: a report that
          // only confirmed the mode the launch asked for, which changed
          // nothing, so the history draws no row.
          if (event.facts?.mode_confirmed) {
            return null
          }
          return (
            <p key={event.seq} data-seq={event.seq} className="system">
              Permission mode {event.facts?.permission_mode ?? 'changed'}
              {event.text && `: ${event.text}`}
            </p>
          )
        }
        if (event.status === 'mode_requested') {
          // the operator's mode request: never a report of what runs.
          if (confirmed.has(event.seq)) {
            return null
          }
          return (
            <p key={event.seq} data-seq={event.seq} className="system">
              Permission mode {event.facts?.permission_mode ?? 'changed'} requested
              {event.text && `: ${event.text}`}
            </p>
          )
        }
        if (event.status === 'choice_filled') {
          // the one-time fill of a conversation recorded before every one
          // held a concrete request: what was filled, and from where.
          const filled = [shortModel(models, event.facts?.model), event.facts?.effort, shortMode(event.facts?.permission_mode)].filter(Boolean)
          return (
            <p key={event.seq} data-seq={event.seq} className="system">
              Request {event.text ?? 'filled in'}: {filled.join(' · ')}
            </p>
          )
        }
        if (event.status === 'waiting') {
          // an interrupt pi cannot act on yet: its abort waits until the
          // pending dialog is answered or dismissed. mega answers neither.
          return (
            <p key={event.seq} data-seq={event.seq} className="system waiting">
              Interrupt waiting on a dialog{event.text && `: ${event.text}`}
            </p>
          )
        }
        if (event.status === 'model_changed') {
          // the harness's own report of what it now runs.
          const reported = [event.facts?.model, event.facts?.effort && `effort ${event.facts.effort}`].filter(Boolean)
          return (
            <p key={event.seq} data-seq={event.seq} className="system">
              Now running {reported.join(', ') || 'a different model'}
              {event.text && `: ${event.text}`}
            </p>
          )
        }
        {
          // a start whose build is older than the tested range is marked, on
          // the line that already names the version, in the notice colour,
          // with the range and the sentence as its title. nothing else is
          // said, and nothing was refused. a newer build is the normal state
          // for a harness that updates itself, and is not marked.
          const outside = event.status === 'started' && event.facts?.standing === 'older'
          let className = 'system'
          if (event.status === 'error' || event.status === 'unsupervised' || event.status === 'not_started') {
            className = 'system alert'
          } else if (outside) {
            className = 'system version-older'
          }
          return (
            <p key={event.seq} data-seq={event.seq} className={className} title={outside ? event.facts?.version_note : undefined}>
              Runtime {event.status === 'not_started' ? 'did not start' : event.status}
              {event.facts?.model && ` · ${event.facts.model}`}
              {event.facts?.effort && ` · effort ${event.facts.effort}`}
              {event.facts?.version && ` · v${event.facts.version}`}
              {outside && ` · older than the ${event.facts?.tested} mega was tested with`}
              {event.text && `: ${event.text}`}
              {event.gap && (
                <span className="history-gap">
                  History may have a gap.{' '}
                  {onTranscript
                    ? <button type="button" onClick={onTranscript}>View native transcript</button>
                    : 'Use Adopt a session to recover the native session.'}
                </span>
              )}
            </p>
          )
        }
      case 'turn_end':
        return (
          <div key={event.seq} className="turn-end">
            {event.status === 'success' ? (
              <hr data-seq={event.seq} className="turn" />
            ) : (
              <p data-seq={event.seq} className="system alert">
                Turn ended: {event.status}
              </p>
            )}
            <TurnLine record={event.turn} models={models} began={began.get(event.seq)} ended={parse(event.at)} />
          </div>
        )
      case 'native':
        if (!showNative) {
          return null
        }
        return (
          <details key={event.seq} data-seq={event.seq} className="native">
            <summary>{event.text ?? 'native'}</summary>
            <pre>{JSON.stringify(event.raw, null, 2)}</pre>
          </details>
        )
      default:
        return null
    }
  }

  // actions (tool rows, request rows, and native rows when shown) between two
  // pieces of speech form one indented group; speech, runtime lines and turn
  // separators stand on their own.
  // a group is keyed by its first event, so loading earlier history above it
  // keeps every group, and the rows open in it, as they were.
  // each turn's blocks, to its end, form one turn block, so its line shows
  // while the pointer is over any of it or focus is inside it. a turn block
  // is keyed by its first event, as a group is.
  const turns: ReactNode[] = []
  let blocks: ReactNode[] = []
  let turnFrom: number | null = null
  let group: ReactNode[] = []
  let groupFrom = 0
  const flush = () => {
    if (group.length > 0) {
      blocks.push(
        <div key={`group-${groupFrom}`} className="action-group">
          {group}
        </div>,
      )
      group = []
    }
  }
  const closeTurn = (key: string) => {
    flush()
    if (blocks.length > 0) {
      turns.push(
        <div key={key} className="turn-block">
          {blocks}
        </div>,
      )
    }
    blocks = []
    turnFrom = null
  }
  events.forEach((event, index) => {
    const node = render(event, index)
    if (node === null) {
      return
    }
    if (historyBoundary(event) && turnFrom !== null) {
      closeTurn(`turn-${turnFrom}`)
    }
    if (turnFrom === null) {
      turnFrom = event.seq
    }
    if (actionKinds.has(event.kind)) {
      if (group.length === 0) {
        groupFrom = event.seq
      }
      group.push(node)
      return
    }
    flush()
    blocks.push(node)
    if (event.kind === 'turn_end') {
      closeTurn(`turn-${turnFrom}`)
    }
  })
  flush()
  if (delta) {
    blocks.push(
      <div key="streaming" className="message assistant streaming">
        <Markdown text={delta} />
      </div>,
    )
  }
  // the turn in flight ends in the space its separator will take, where its
  // line shows what it runs with once the harness has said.
  if (turnActive) {
    blocks.push(
      <div key="turn-end-pending" className="turn-end pending">
        <hr className="turn" />
        <TurnLine record={inFlight} models={models} began={open} />
      </div>,
    )
  }
  closeTurn(`turn-${turnFrom ?? 'open'}`)
  return <div className="events">{turns}</div>
}

// promptTitle is a prompt block's title: when it was sent, or, from outside
// mega, when mega imported it, in the service's zone.
function promptTitle(event: ConversationEvent): string | undefined {
  const at = parse(event.at)
  if (!at) {
    return undefined
  }
  return `${event.origin === 'native' ? 'Imported' : 'Sent'} ${fullTime(at)}`
}

// TurnLine is one compact line saying what a turn ran with: the model, the
// effort and (claude) the mode, as the harness reported them; claude never
// reports effort, so its effort is the one sent. after it, when the prompt
// was sent and how long the turn took, or for the turn in flight when it
// started; a turn with no record of what it ran with has its time alone. a
// time is to the minute, with its date when not today, in the service's
// zone; the title has both to the second. it shows only while the turn is
// under the pointer or holds focus, unless the turn ran with other than it
// asked for: then it stays, in the notice colour, and names both.
function TurnLine({ record, models, began, ended }: { record?: TurnRecord; models: Models | null; began?: Began; ended?: Date }) {
  const differs = record?.differs ?? []
  const field = (name: string, ran: string | undefined, asked: string | undefined, show: (value: string | undefined) => string) => {
    if (!ran) {
      return ''
    }
    return differs.includes(name) ? `${show(ran)}, asked ${show(asked)}` : show(ran)
  }
  const parts = record
    ? [
        field('model', record.ran.model, record.asked.model, (value) => shortModel(models, value)),
        field('effort', record.ran.effort, record.asked.effort, (value) => value ?? ''),
        field('mode', record.ran.mode, record.asked.mode, shortMode),
      ].filter(Boolean)
    : []
  // an imported turn's times are when mega imported it, so it has no length.
  // the length is said as the activity line says it, and a turn under a
  // second as under 1s.
  const ms = began && ended && !began.imported ? Math.max(0, ended.getTime() - began.at.getTime()) : null
  const length = ms === null ? '' : ms < 1000 ? 'under 1s' : longElapsed(ms)
  const how = began ? (began.imported ? 'imported' : began.prompt ? 'sent' : 'started') : ''
  const time = began ? `${how} ${shortTime(began.at)}${length && ` · took ${length}`}` : ''
  if (parts.length === 0 && !time) {
    return null
  }
  const exact = (settings: TurnRecord['asked']) => [settings.model, settings.effort, settings.mode].filter(Boolean).join(' · ')
  const title = [
    record && parts.length > 0 && `Asked for ${exact(record.asked)}; ran ${exact(record.ran)}`,
    began && `${how[0].toUpperCase()}${how.slice(1)} ${fullTime(began.at)}${length && ended ? `; ended ${fullTime(ended)}` : ''}`,
  ]
    .filter(Boolean)
    .join('. ')
  const text = parts.length > 0 ? `Ran ${parts.join(' · ')}${time && ` · ${time}`}` : `${time[0].toUpperCase()}${time.slice(1)}`
  return (
    <span className={`turn-line${differs.length > 0 ? ' differs' : ''}`} title={title}>
      {text}
    </span>
  )
}

const actionKinds = new Set<ConversationEvent['kind']>(['tool_use', 'tool_result', 'native', 'request', 'request_resolved'])

// ToolRow is one action. collapsed, it is its one line, with a failed
// result marked in the error colour. expanded, it shows what it invoked, in
// full, read from the service as it first opens, since the recorded event
// already holds it and nothing stores it again; then what came back.
function ToolRow({ event, result, state, conversationId }: { event: ConversationEvent; result?: ConversationEvent; state: ReactNode; conversationId: string }) {
  const [invocation, setInvocation] = useState<Invocation | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [asked, setAsked] = useState(false)
  const onToggle = (toggled: SyntheticEvent<HTMLDetailsElement>) => {
    if (!toggled.currentTarget.open || (asked && !error)) {
      return
    }
    setAsked(true)
    setError(null)
    getInvocation(conversationId, event.seq).then(setInvocation, (failure: unknown) =>
      setError(failure instanceof Error ? failure.message : String(failure)),
    )
  }
  return (
    <details data-seq={event.seq} className={`tool${result?.status === 'error' ? ' failed' : ''}`} onToggle={onToggle}>
      <summary>
        {event.text}
        {state}
      </summary>
      {error ? (
        <p className="error">Could not read what it invoked: {error}</p>
      ) : invocation ? (
        <InvocationView invocation={invocation} />
      ) : (
        asked && <p className="muted tool-part">Reading what it invoked…</p>
      )}
      {result?.text && (
        <>
          <p className="tool-part">Result</p>
          <ToolOutput text={result.text} />
        </>
      )}
    </details>
  )
}

// invocationText is what the invocation's copy control takes: the command,
// or the tool and each field of its input.
function invocationText(invocation: Invocation): string {
  if (invocation.command) {
    return invocation.command
  }
  return [invocation.tool, ...(invocation.fields ?? []).map((field) => `${field.name}: ${field.value}`)].join('\n')
}

// InvocationView is what an action invoked: a shell command whole, then any
// other input it was given; any other tool as its input, field by field.
// each box is bounded and scrolls, as a result's does, and the copy control
// takes all of it. an action whose event holds nothing its adapter can read
// shows the one line it was recorded with.
function InvocationView({ invocation }: { invocation: Invocation }) {
  if (!invocation.recorded) {
    return (
      <>
        <p className="tool-part">Invoked</p>
        <p className="muted">Only its one line was recorded:</p>
        <pre className="invocation-summary">{invocation.summary}</pre>
      </>
    )
  }
  const fields = invocation.fields ?? []
  return (
    <>
      <p className="tool-part">{invocation.command ? 'Command' : `Invoked ${invocation.tool}`}</p>
      <div className="tool-output invocation">
        <CopyButton text={invocationText(invocation)} label={invocation.command ? 'Copy the command' : 'Copy the input'} />
        {invocation.command && <pre className="command">{invocation.command}</pre>}
        {fields.length > 0 && (
          <dl className="invocation-fields">
            {fields.map((field) => (
              <div key={field.name}>
                <dt>{field.name}</dt>
                <dd>
                  <pre>{field.value}</pre>
                </dd>
              </div>
            ))}
          </dl>
        )}
      </div>
    </>
  )
}

// ToolOutput is an expanded tool result: the output, scrolling inside its
// box, with one control that copies all of it, whatever the box shows.
function ToolOutput({ text }: { text: string }) {
  return (
    <div className="tool-output">
      <CopyButton text={text} label="Copy the output" />
      <pre>{text}</pre>
    </div>
  )
}
