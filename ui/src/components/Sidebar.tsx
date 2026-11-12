import { useEffect, useState, useSyncExternalStore } from 'react'
import { starProject } from '../api'
import { folds, isFolded } from '../prefs'
import { select, type Selection } from '../selection'
import { connectionLabel, failing, serviceAddress, type Knowledge, type StreamState } from '../stream'
import { useWallNow } from '../clock'
import { waitingLabel, type Waiting } from '../waiting'
import { treeGroups } from '../tree'
import type { About, Conversation, Project, ShutdownGate } from '../types'
import { AttentionMark, BackgroundLabel, Elapsed, ProjectMark } from './AttentionMark'
import { FooterGate } from './Gate'
import { Icon, IconButton } from './Icon'
import { missingReason } from '../checkout'
import { panelShortcut } from './SidePanel'
import { aboutTitle, newerLabel } from '../build'

interface SidebarProps {
  projects: Project[]
  conversations: Conversation[]
  needsYou: string[]
  selection: Selection | null
  streamState: StreamState
  stale: boolean
  // whether the workspace has been read at least once, so a project missing
  // from it is really gone.
  loaded: boolean
  // what the page knows; before the first read, nothing, and it says so.
  knowledge: Knowledge
  // what the page is waiting for, named in the connection label's title.
  waiting: Waiting | null
  onRegister: () => void
  onCollapse: () => void
  gate: ShutdownGate | undefined
  shutDownAt: Date | null
  gateBusy: boolean
  onArm: () => void
  onCancelShutdown: () => void
  about: About | null
  newer: boolean
  onAbout: () => void
}

// Sidebar is the expanded side panel: the needs-you list, then the projects,
// starred first, each group by name, each project folding to its row and its
// rollup mark. a starred project is unfolded and any other folded, until the
// operator folds or unfolds it himself, which this browser keeps; stars are
// workspace state, written through the service.
export function Sidebar(props: SidebarProps) {
  const { projects, conversations, needsYou, selection, streamState, stale, loaded, knowledge, onRegister, onCollapse } = props
  const { gate, shutDownAt, gateBusy, onArm, onCancelShutdown } = props
  const { waiting: waitingFor, about, newer, onAbout } = props
  // before the first read the page knows nothing, and says so, never "none".
  const unknownText =
    knowledge === 'unreachable'
      ? `Not known: the service at ${serviceAddress()} is unreachable.`
      : knowledge === 'unreadable'
        ? `Not known: the service at ${serviceAddress()} could not read the workspace.`
        : 'Not known yet: connecting.'
  const unknown = !loaded ? <p className="muted unknown-line">{unknownText}</p> : null
  const foldState = useSyncExternalStore(folds.subscribe, folds.get)
  const [starError, setStarError] = useState<string | null>(null)
  const isSelected = (kind: Selection['kind'], id: string) => selection?.kind === kind && selection.id === id
  const byId = new Map(conversations.map((conversation) => [conversation.id, conversation]))
  const projectNames = new Map(projects.map((project) => [project.id, project.name]))
  const waiting = needsYou.map((id) => byId.get(id)).filter((c): c is Conversation => c !== undefined)
  const { starred, rest } = treeGroups(projects)
  // where the connection status is: a page running an older interface than
  // the service serves says so while it is connected; it never reloads
  // itself.
  const down = shutDownAt !== null && streamState !== 'connected'
  const showNewer = newer && !down && streamState === 'connected' && !failing(knowledge)

  // a fold choice for a project that no longer exists is forgotten, once the
  // workspace has been read and its absence means something.
  useEffect(() => {
    if (loaded) {
      folds.forget(new Set(projects.map((project) => project.id)))
    }
  }, [loaded, projects])

  // navigation never lands on something hidden: selecting a conversation, by
  // the needs-you list, the next-waiting shortcut or a link, unfolds its
  // project. that is viewing, not a choice, so nothing is recorded.
  const selectedProject = selection?.kind === 'conversation' ? byId.get(selection.id)?.project_id : undefined
  useEffect(() => {
    if (selectedProject) {
      folds.view(selectedProject)
    }
  }, [selectedProject])

  const toggleStar = async (project: Project) => {
    setStarError(null)
    try {
      await starProject(project.id, !project.starred)
    } catch (failure) {
      setStarError(failure instanceof Error ? failure.message : String(failure))
    }
  }

  const projectItem = (project: Project) => {
    const inProject = conversations.filter((conversation) => conversation.project_id === project.id)
    const folded = isFolded(foldState, project)
    return (
      <li key={project.id}>
        <div className={isSelected('project', project.id) ? 'project-row selected' : 'project-row'}>
          <IconButton
            className="fold"
            icon={folded ? 'chevron_right' : 'expand_more'}
            label={folded ? `Show the conversations in ${project.name}` : `Hide the conversations in ${project.name}`}
            aria-expanded={!folded}
            onClick={() => folds.toggle(project)}
          />
          <button
            type="button"
            className="tree-item project-item"
            aria-current={isSelected('project', project.id) ? 'true' : undefined}
            onClick={() => select({ kind: 'project', id: project.id })}
          >
            <ProjectMark conversations={inProject} stale={stale} />
            <span className="tree-label">{project.name}</span>
            {project.checkout_missing && (
              <span className="checkout-missing-mark" title={missingReason(project) ?? ''} aria-label={missingReason(project) ?? ''} role="img">
                <Icon name="error_outline" />
              </span>
            )}
          </button>
          <IconButton
            className={project.starred ? 'star starred' : 'star'}
            icon={project.starred ? 'star' : 'star_border'}
            label={project.starred ? `Unstar ${project.name}` : `Star ${project.name}`}
            aria-pressed={project.starred}
            onClick={() => void toggleStar(project)}
          />
        </div>
        {!folded && (
          <ul className="tree-children">
            {inProject.map((conversation) => (
              <li key={conversation.id}>
                <button
                  type="button"
                  className={isSelected('conversation', conversation.id) ? 'tree-item selected' : 'tree-item'}
                  aria-current={isSelected('conversation', conversation.id) ? 'true' : undefined}
                  onClick={() => select({ kind: 'conversation', id: conversation.id })}
                >
                  <AttentionMark conversation={conversation} stale={stale} />
                  <span className="tree-label">{conversation.name}</span>
                  <Elapsed conversation={conversation} />
                  <BackgroundLabel conversation={conversation} />
                  {conversation.label && <span className="tag">{conversation.label}</span>}
                </button>
              </li>
            ))}
          </ul>
        )}
      </li>
    )
  }

  return (
    <nav className="sidebar">
      <section className="needs-you">
        <div className="section-head">
          <h2>Needs you</h2>
          <span className="muted shortcut" title="Next waiting conversation: Ctrl+/ anywhere">
            Ctrl+/
          </span>
        </div>
        {unknown ?? (waiting.length === 0 ? (
          <p className="muted">Nothing waiting.</p>
        ) : (
          <ul>
            {waiting.map((conversation) => (
              <li key={conversation.id}>
                <button
                  type="button"
                  className={isSelected('conversation', conversation.id) ? 'tree-item waiting selected' : 'tree-item waiting'}
                  aria-current={isSelected('conversation', conversation.id) ? 'true' : undefined}
                  onClick={() => select({ kind: 'conversation', id: conversation.id })}
                >
                  <AttentionMark conversation={conversation} stale={stale} />
                  <span className="tree-label">{conversation.name}</span>
                  <Elapsed conversation={conversation} />
                  <BackgroundLabel conversation={conversation} />
                  <span className="muted waiting-project">{projectNames.get(conversation.project_id)}</span>
                </button>
              </li>
            ))}
          </ul>
        ))}
      </section>
      <section className="projects">
        <div className="section-head">
          <h2>Projects</h2>
          <IconButton
            icon="create_new_folder"
            label={loaded ? 'Register a project' : 'Register a project (waiting for the service)'}
            disabled={!loaded}
            onClick={onRegister}
          />
        </div>
        {starError && <p className="error">{starError}</p>}
        {unknown}
        {loaded && projects.length === 0 && <p className="muted">No projects yet.</p>}
        {starred.length > 0 && (
          <ul className="project-group starred-group" aria-label="Starred projects">
            {starred.map(projectItem)}
          </ul>
        )}
        {rest.length > 0 && (
          <ul className="project-group" aria-label={starred.length > 0 ? 'Other projects' : 'Projects'}>
            {rest.map(projectItem)}
          </ul>
        )}
      </section>
      {!shutDownAt && <FooterGate gate={gate} stale={stale} busy={gateBusy} onCancel={onCancelShutdown} />}
      <footer className="side-foot">
        <ConnectionStatus
          className={`stream stream-${failing(knowledge) || down ? 'disconnected' : showNewer ? 'newer' : streamState}`}
          label={down ? 'Shut down from here, retrying' : showNewer ? newerLabel : connectionLabel(knowledge, streamState)}
          waiting={waitingFor}
        />
        {about && (
          <button type="button" className="version-label" title={aboutTitle(about)} aria-label={aboutTitle(about)} onClick={onAbout}>
            {about.version}
          </button>
        )}
        <span className="side-foot-actions">
          {!gate?.armed && !shutDownAt && (
            <IconButton icon="power_settings_new" label="Shut down when every agent is idle" disabled={gateBusy || !loaded || stale} onClick={onArm} />
          )}
          <IconButton icon="keyboard_double_arrow_left" label={`Collapse the side panel (${panelShortcut})`} onClick={onCollapse} />
        </span>
      </footer>
    </nav>
  )
}

// ConnectionStatus is the footer's connection label. while the page waits on
// something, its title names it, timed; only then does it read the clock.
function ConnectionStatus({ className, label, waiting }: { className: string; label: string; waiting: Waiting | null }) {
  if (!waiting) {
    return <span className={className}>{label}</span>
  }
  return <TimedStatus className={className} label={label} waiting={waiting} />
}

function TimedStatus({ className, label, waiting }: { className: string; label: string; waiting: Waiting }) {
  const now = useWallNow()
  return (
    <span className={className} title={waitingLabel(waiting, now)}>
      {label}
    </span>
  )
}
