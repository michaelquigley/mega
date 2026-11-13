import type { ReactNode } from 'react'
import { shortModel, useModels } from '../models'
import { shortMode } from '../permissions'
import type { ConversationEvent, Models, PendingRequest, TurnRecord } from '../types'
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
  harness: string
  // what the turn in flight asked for and ran with, once it has run.
  inFlight?: TurnRecord
}

const outcomeLabels: Record<string, string> = {
  answered: 'answered',
  aborted: 'aborted',
  lost: 'lost when the service restarted',
}

// EventList renders recorded history. requests appear here only as history;
// whether one is answerable comes from the service's pending set, never from
// the log.
export function EventList({ events, pending, turnActive, stale, showNative, delta, projectId, harness, inFlight }: EventListProps) {
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
  const toolUses = new Set(events.filter((e) => e.kind === 'tool_use' && e.tool_use_id).map((e) => e.tool_use_id))
  const pendingIds = new Set(pending.map((request) => request.id))
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
          <div key={event.seq} data-seq={event.seq} className="message user">
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
        return (
          <details key={event.seq} data-seq={event.seq} className="tool">
            <summary>
              {event.text}
              {result?.status === 'error' && <span className="tool-error"> failed</span>}
              {!result &&
                (turnActive && index > turnStart && !stale ? (
                  <>
                    {' '}
                    <Spinner label="Running" />
                  </>
                ) : (
                  <span className="muted">
                    {turnActive && index > turnStart ? ' running when last seen' : ' no result recorded'}
                  </span>
                ))}
            </summary>
            {result?.text && <ToolOutput text={result.text} />}
          </details>
        )
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
        return (
          <p
            key={event.seq} data-seq={event.seq}
            className={event.status === 'error' || event.status === 'unsupervised' || event.status === 'not_started' ? 'system alert' : 'system'}
          >
            Runtime {event.status === 'not_started' ? 'did not start' : event.status}
            {event.facts?.model && ` · ${event.facts.model}`}
            {event.facts?.effort && ` · effort ${event.facts.effort}`}
            {event.facts?.version && ` · v${event.facts.version}`}
            {event.text && `: ${event.text}`}
          </p>
        )
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
            {event.turn && <TurnLine record={event.turn} models={models} />}
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
        {inFlight && <TurnLine record={inFlight} models={models} />}
      </div>,
    )
  }
  closeTurn(`turn-${turnFrom ?? 'open'}`)
  return <div className="events">{turns}</div>
}

// TurnLine is one compact line saying what a turn ran with: the model, the
// effort and (claude) the mode, as the harness reported them; claude never
// reports effort, so its effort is the one sent. it shows only while the turn
// is under the pointer or holds focus, unless the turn ran with other than it
// asked for: then it stays, in the notice colour, and names both.
function TurnLine({ record, models }: { record: TurnRecord; models: Models | null }) {
  const differs = record.differs ?? []
  const field = (name: string, ran: string | undefined, asked: string | undefined, show: (value: string | undefined) => string) => {
    if (!ran) {
      return ''
    }
    return differs.includes(name) ? `${show(ran)}, asked ${show(asked)}` : show(ran)
  }
  const parts = [
    field('model', record.ran.model, record.asked.model, (value) => shortModel(models, value)),
    field('effort', record.ran.effort, record.asked.effort, (value) => value ?? ''),
    field('mode', record.ran.mode, record.asked.mode, shortMode),
  ].filter(Boolean)
  if (parts.length === 0) {
    return null
  }
  const exact = (settings: TurnRecord['asked']) => [settings.model, settings.effort, settings.mode].filter(Boolean).join(' · ')
  return (
    <span className={`turn-line${differs.length > 0 ? ' differs' : ''}`} title={`Asked for ${exact(record.asked)}; ran ${exact(record.ran)}`}>
      Ran {parts.join(' · ')}
    </span>
  )
}

const actionKinds = new Set<ConversationEvent['kind']>(['tool_use', 'tool_result', 'native', 'request', 'request_resolved'])

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
