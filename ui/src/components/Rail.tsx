import { select } from '../selection'
import { streamLabels, type StreamState } from '../stream'
import { treeGroups } from '../tree'
import type { Conversation, Project } from '../types'
import { ProjectMark } from './AttentionMark'
import { IconButton } from './Icon'

interface RailProps {
  projects: Project[]
  conversations: Conversation[]
  needsYou: string[]
  streamState: StreamState
  stale: boolean
  onExpand: () => void
}

// Rail is the collapsed side panel: the control to expand, how many
// conversations need the operator, one mark per project in tree order, and the
// connection. nothing that signals attention or activity is lost by
// collapsing: each project's mark is the same rollup the tree shows, working
// arc included.
export function Rail({ projects, conversations, needsYou, streamState, stale, onExpand }: RailProps) {
  const { starred, rest } = treeGroups(projects)
  const waiting = needsYou.length
  const count = waiting === 0 ? 'Nothing needs you' : `${waiting} ${waiting === 1 ? 'conversation needs' : 'conversations need'} you`
  return (
    <nav className="rail" aria-label="Side panel, collapsed">
      <IconButton icon="keyboard_double_arrow_right" label="Expand the side panel" onClick={onExpand} />
      <span className={`rail-count${waiting > 0 ? ' waiting' : ''}${stale ? ' stale' : ''}`} title={stale ? `Last known: ${count.toLowerCase()}` : count} aria-label={count}>
        {waiting}
      </span>
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
      <span className={`rail-stream stream-${streamState}`} title={streamLabels[streamState]} aria-label={streamLabels[streamState]} role="img" />
    </nav>
  )
}
