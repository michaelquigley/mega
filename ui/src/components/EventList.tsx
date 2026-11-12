import type { ReactNode } from 'react'
import type { ConversationEvent, PendingRequest } from '../types'
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
}

const outcomeLabels: Record<string, string> = {
  answered: 'answered',
  aborted: 'aborted',
  lost: 'lost when the service restarted',
}

// EventList renders recorded history. requests appear here only as history;
// whether one is answerable comes from the service's pending set, never from
// the log.
export function EventList({ events, pending, turnActive, stale, showNative, delta }: EventListProps) {
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
            <span className="who">{event.origin === 'native' ? 'You, outside mega' : 'You'}</span>
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
          // claude's first report after a start with no prompt, naming the
          // mode that was requested: the header dropping "(requested)" is
          // the change, so the history draws no row.
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
        if (event.status === 'success') {
          return <hr key={event.seq} data-seq={event.seq} className="turn" />
        }
        return (
          <p key={event.seq} data-seq={event.seq} className="system alert">
            Turn ended: {event.status}
          </p>
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
  const blocks: ReactNode[] = []
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
  events.forEach((event, index) => {
    const node = render(event, index)
    if (node === null) {
      return
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
  })
  flush()
  if (delta) {
    blocks.push(
      <div key="streaming" className="message assistant streaming">
        <Markdown text={delta} />
      </div>,
    )
  }
  return <div className="events">{blocks}</div>
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
