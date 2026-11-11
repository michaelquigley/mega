import { blockerKinds, clockTime, gateCounts, gateSummary } from '../gate'
import type { ShutdownGate } from '../types'
import { Icon, IconButton } from './Icon'

// BlockerList names what the gate waits for, one conversation per line, each
// name a link that opens it. a request is marked as the operator's to answer.
export function BlockerList({ gate }: { gate: ShutdownGate }) {
  return (
    <ul className="gate-blockers">
      {gate.blockers.map((b) => (
        <li key={b.conversation_id}>
          <a href={`#/c/${encodeURIComponent(b.conversation_id)}`}>{b.name}</a>
          <span className={`gate-kinds${b.requests > 0 ? ' yours' : ''}`}>{blockerKinds(b).join(', ')}</span>
        </li>
      ))}
    </ul>
  )
}

interface BannerProps {
  gate: ShutdownGate | undefined
  // set once this page's shutdown was accepted: when it asked.
  shutDownAt: Date | null
  connected: boolean
  // the page shows only what it last read: the gate is last known, and nothing
  // is offered on it.
  stale: boolean
  busy: boolean
  error: string | null
  onShutDown: () => void
  onCancel: () => void
}

// ShutdownBanner runs across the top of the main column while the gate is
// armed: what it waits for, and at idle the offer to shut down. on the page
// that shut the service down it says so, and when, until the service is back.
export function ShutdownBanner({ gate, shutDownAt, connected, stale, busy, error, onShutDown, onCancel }: BannerProps) {
  if (shutDownAt) {
    return (
      <section className="gate-banner shut-down" role="status">
        <Icon name="power_settings_new" />
        <div className="gate-body">
          {connected ? (
            <>
              <strong>Shutting down</strong>
              <p>Asked from this page at {clockTime(shutDownAt)}. Every runtime is being released.</p>
            </>
          ) : (
            <>
              <strong>mega was shut down from this page at {clockTime(shutDownAt)}</strong>
              <p>The page keeps trying, and the workspace comes back by itself when the service starts again.</p>
            </>
          )}
        </div>
      </section>
    )
  }
  if (!gate?.armed) {
    return null
  }
  const known = stale ? 'Last known: ' : ''
  return (
    <section className={`gate-banner${gate.idle ? ' idle' : ''}${stale ? ' stale' : ''}`} role="status" aria-label="Shutdown">
      <Icon name="power_settings_new" />
      <div className="gate-body">
        {gate.idle ? (
          <>
            <strong>{known}Every agent is idle</strong>
            <p>
              {stale
                ? 'The page has lost the service, so this is only what it last read. Shut down is offered again once it reads the workspace.'
                : 'Nothing is in flight, nothing waits for an answer, and no background job runs. Shut down releases every runtime and stops the service.'}
            </p>
          </>
        ) : (
          <>
            <strong>
              {known}Waiting to shut down: {gateCounts(gate)}
            </strong>
            <p>No new work starts until the shutdown or a cancel. Nothing is answered, interrupted or released for you.</p>
            <BlockerList gate={gate} />
          </>
        )}
        {error && <p className="error">{error}</p>}
      </div>
      <div className="gate-actions">
        {gate.idle && !stale && <IconButton icon="power_settings_new" label="Shut down mega now" word="Shut down" disabled={busy} onClick={onShutDown} />}
        <IconButton icon="close" label="Cancel the shutdown" word="Cancel" disabled={busy || stale} onClick={onCancel} />
      </div>
    </section>
  )
}

interface FooterGateProps {
  gate: ShutdownGate | undefined
  stale: boolean
  busy: boolean
  onCancel: () => void
}

// FooterGate is the armed gate above the side panel's footer: what it waits
// for, by count and by name, and Cancel. the footer itself holds the control
// that arms it.
export function FooterGate({ gate, stale, busy, onCancel }: FooterGateProps) {
  if (!gate?.armed) {
    return null
  }
  return (
    <div className={`side-gate${gate.idle ? ' idle' : ''}${stale ? ' stale' : ''}`} role="status" aria-label="Shutdown">
      <div className="side-gate-head">
        <span>
          {stale ? 'Last known: ' : ''}
          {gate.idle ? 'Every agent is idle' : `Waiting to shut down: ${gateCounts(gate)}`}
        </span>
        <IconButton icon="close" label="Cancel the shutdown" disabled={busy || stale} onClick={onCancel} />
      </div>
      {!gate.idle && <BlockerList gate={gate} />}
    </div>
  )
}

// RailGate is the gate on the collapsed rail: how many conversations hold it,
// or a check at idle, with their names in the title. a press expands the panel,
// where each name opens its conversation.
export function RailGate({ gate, stale, onExpand }: { gate: ShutdownGate | undefined; stale: boolean; onExpand: () => void }) {
  if (!gate?.armed) {
    return null
  }
  const summary = `${stale ? 'Last known: ' : ''}${gateSummary(gate)}`
  return (
    <button type="button" className={`rail-gate${gate.idle ? ' idle' : ''}${stale ? ' stale' : ''}`} title={summary} aria-label={summary} onClick={onExpand}>
      <Icon name="power_settings_new" />
      <span>{gate.idle ? '✓' : gate.blockers.length}</span>
    </button>
  )
}
