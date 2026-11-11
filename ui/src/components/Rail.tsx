import { select } from '../selection'
import { connectionLabel, failing, type Knowledge, type StreamState } from '../stream'
import { treeGroups } from '../tree'
import type { Conversation, Project, ShutdownGate } from '../types'
import { RailGate } from './Gate'
import { ProjectMark } from './AttentionMark'
import { IconButton } from './Icon'

interface RailProps {
  projects: Project[]
  conversations: Conversation[]
  needsYou: string[]
  streamState: StreamState
  stale: boolean
  loaded: boolean
  knowledge: Knowledge
  onExpand: () => void
  gate: ShutdownGate | undefined
  shutDownAt: Date | null
}

// Rail is the collapsed side panel: the control to expand, how many
// conversations need the operator, one mark per project in tree order, and the
// connection. nothing that signals attention or activity is lost by
// collapsing: each project's mark is the same rollup the tree shows, working
// arc included.
export function Rail({ projects, conversations, needsYou, streamState, stale, loaded, knowledge, onExpand, gate, shutDownAt }: RailProps) {
  const { starred, rest } = treeGroups(projects)
  const waiting = needsYou.length
  // before the first read the count is unknown, never zero.
  let count = waiting === 0 ? 'Nothing needs you' : `${waiting} ${waiting === 1 ? 'conversation needs' : 'conversations need'} you`
  if (!loaded) {
    count =
      knowledge === 'unreachable'
        ? 'Not known: the service is unreachable'
        : knowledge === 'unreadable'
          ? 'Not known: the service could not read the workspace'
          : 'Not known yet: connecting'
  }
  const down = shutDownAt !== null && streamState !== 'connected'
  const connection = down ? 'Shut down from here, retrying' : connectionLabel(knowledge, streamState)
  return (
    <nav className="rail" aria-label="Side panel, collapsed">
      <IconButton icon="keyboard_double_arrow_right" label="Expand the side panel" onClick={onExpand} />
      <span
        className={`rail-count${!loaded ? ' unknown' : waiting > 0 ? ' waiting' : ''}${loaded && stale ? ' stale' : ''}`}
        title={loaded && stale ? `Last known: ${count.toLowerCase()}` : count}
        aria-label={count}
      >
        {loaded ? waiting : '?'}
      </span>
      {!shutDownAt && <RailGate gate={gate} stale={stale} onExpand={onExpand} />}
      <ul className="rail-projects">
        {[...starred, ...rest].map((project) => (
          <li key={project.id}>
            <button
              type="button"
              className="rail-mark"
              title={project.name}
              aria-label={project.name}
              onClick={() => {
                onExpand()
                select({ kind: 'project', id: project.id })
              }}
            >
              <ProjectMark conversations={conversations.filter((c) => c.project_id === project.id)} stale={stale} />
            </button>
          </li>
        ))}
      </ul>
      <span className={`rail-stream stream-${failing(knowledge) || down ? 'disconnected' : streamState}${loaded ? '' : ' unknown'}`} title={connection} aria-label={connection} role="img" />
    </nav>
  )
}
