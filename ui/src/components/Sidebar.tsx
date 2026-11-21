import { useEffect, useState, useSyncExternalStore, type KeyboardEvent, type MouseEvent } from 'react'
import { starProject } from '../api'
import { folds, isFolded, type FoldChoice } from '../prefs'
import { select, type Selection } from '../selection'
import { connectionLabel, connectionOf, failing, serviceAddress, type Knowledge, type StreamState } from '../stream'
import { useWallNow } from '../clock'
import { waitingLabel, type Waiting } from '../waiting'
import { treeGroups } from '../tree'
import type { About, Conversation, Project, ShutdownGate } from '../types'
import { AttentionMark, BackgroundLabel, Elapsed, ProjectMark } from './AttentionMark'
import { LastInstructed } from './LastInstructed'
import { lastInstructed } from '../instructed'
import { FooterGate } from './Gate'
import { Icon, IconButton } from './Icon'
import { environmentReason, missingReason } from '../checkout'
import { collapseAll, expandAll, nextWaiting, panelFocus, panelToggle } from '../keys'
import { entryRow, focusPanel } from '../panel'
import { aboutTitle, newerLabel } from '../build'
import { shutDownFooter } from '../gate'

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
  // a project row's adopt a session and new conversation, and what holds
  // new work page-wide, a shutdown; the row adds its own checkout and
  // environment, as the project page does.
  onLaunch: (projectId: string) => void
  onAdopt: (projectId: string) => void
  heldBy: string | null
  onCollapse: () => void
  gate: ShutdownGate | undefined
  shutDownAt: Date | null
  shutDownHeard: boolean
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
  const { gate, shutDownAt, shutDownHeard, gateBusy, onArm, onCancelShutdown } = props
  const { waiting: waitingFor, about, newer, onAbout, onLaunch, onAdopt, heldBy } = props
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
  const showNewer = newer && !down && connectionOf(knowledge, streamState) === 'connected' && !failing(knowledge)

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

  // expand all and collapse all record a choice for every project, as a
  // press on each one's own control would.
  const foldAll = (choice: FoldChoice) => folds.setAll(projects.map((project) => project.id), choice)

  // the keys in the projects tree. Up, Down, Home and End move the focus
  // along the rows, across both groups and on into the needs-you list above
  // (rowStep). Right unfolds a folded project, or
  // goes to its first conversation; Left folds an unfolded one, or goes from
  // a conversation to its project. a fold or an unfold is a choice, as the
  // chevron's is. the keys only move the focus: Enter or Space opens a row,
  // since opening a conversation marks it read and puts the cursor in its
  // reply box.
  const onTreeKey = (event: KeyboardEvent<HTMLElement>) => {
    const here = rowOf(event)
    if (!here) {
      return
    }
    event.preventDefault()
    let next: HTMLElement | null | undefined
    if (event.key === 'ArrowRight' || event.key === 'ArrowLeft') {
      const project = projects.find((p) => p.id === here.dataset.project)
      if (project) {
        const folded = isFolded(foldState, project)
        if ((event.key === 'ArrowRight' && folded) || (event.key === 'ArrowLeft' && !folded)) {
          folds.toggle(project)
        } else if (event.key === 'ArrowRight') {
          next = here.closest('li')?.querySelector<HTMLElement>('.tree-children .tree-item')
        }
      } else if (event.key === 'ArrowLeft') {
        next = here.closest('.tree-children')?.closest('li')?.querySelector<HTMLElement>('.project-item')
      }
    } else {
      next = rowStep(event.currentTarget, here, event.key)
    }
    next?.focus()
  }

  // the needs-you list takes Up, Down, Home and End, as the tree does, the
  // two being one run of rows.
  const onWaitingKey = (event: KeyboardEvent<HTMLElement>) => {
    const here = rowOf(event)
    if (!here || event.key === 'ArrowRight' || event.key === 'ArrowLeft') {
      return
    }
    event.preventDefault()
    rowStep(event.currentTarget, here, event.key)?.focus()
  }

  // where the operator left off in each project; not attention.
  const leftOff = lastInstructed(conversations)
  const projectItem = (project: Project) => {
    const inProject = conversations.filter((conversation) => conversation.project_id === project.id)
    const folded = isFolded(foldState, project)
    // new work in the project is held as on its page: by a shutdown, a
    // missing checkout or a refused environment, the reason as the label.
    const held = heldBy ?? missingReason(project) ?? environmentReason(project)
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
            data-project={project.id}
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
          <IconButton className="row-action" icon="download" label={held ?? `Adopt a session in ${project.name}`} disabled={held !== null} onClick={() => onAdopt(project.id)} />
          <IconButton className="row-action" icon="add_comment" label={held ?? `New conversation in ${project.name}`} disabled={held !== null} onClick={() => onLaunch(project.id)} />
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
                  {leftOff.has(conversation.id) && <LastInstructed conversation={conversation} />}
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
    <nav className="sidebar" onClick={onPanelClick} onKeyDown={onPanelKey} aria-keyshortcuts={panelFocus.aria}>
      <section className="needs-you" tabIndex={-1}>
        <div className="section-head">
          <h2>Needs you</h2>
          <span className="muted shortcut" title={`Next waiting conversation: ${nextWaiting.label}, anywhere but in a dialog`}>
            {nextWaiting.label}
          </span>
        </div>
        {unknown ?? (waiting.length === 0 ? (
          <p className="muted">Nothing waiting.</p>
        ) : (
          <ul onKeyDown={onWaitingKey}>
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
      <section className="projects" tabIndex={-1} onKeyDown={onTreeKey}>
        <div className="section-head">
          <h2>Projects</h2>
          <span className="section-actions">
            {loaded && projects.length > 0 && (
              <>
                <IconButton icon="unfold_more" label={`Expand all projects (${expandAll.label})`} aria-keyshortcuts={expandAll.aria} onClick={() => foldAll('unfolded')} />
                <IconButton icon="unfold_less" label={`Collapse all projects (${collapseAll.label})`} aria-keyshortcuts={collapseAll.aria} onClick={() => foldAll('folded')} />
              </>
            )}
            <IconButton
              icon="create_new_folder"
              label={loaded ? 'Register a project' : 'Register a project (waiting for the service)'}
              // a word as well as the icon while there is nothing yet, so the
              // first page can name the control it points to.
              word={loaded && projects.length === 0 ? 'Register' : undefined}
              disabled={!loaded}
              onClick={onRegister}
            />
          </span>
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
          className={`stream stream-${failing(knowledge) || down ? 'disconnected' : showNewer ? 'newer' : connectionOf(knowledge, streamState)}`}
          label={down ? shutDownFooter(shutDownHeard) : showNewer ? newerLabel : connectionLabel(knowledge, streamState)}
          waiting={waitingFor}
        />
        {about?.name && (
          <span className="mega-name" title={`This mega is ${about.name}`}>
            {about.name}
          </span>
        )}
        {about && (
          <button type="button" className="version-label" title={aboutTitle(about)} aria-label={aboutTitle(about)} onClick={onAbout}>
            {about.version}
          </button>
        )}
        <span className="side-foot-actions">
          {!gate?.armed && !shutDownAt && (
            <IconButton icon="power_settings_new" label="Shut down when every agent is idle" disabled={gateBusy || !loaded || stale} onClick={onArm} />
          )}
          <IconButton icon="keyboard_double_arrow_left" label={`Collapse the side panel (${panelToggle.label})`} onClick={onCollapse} />
        </span>
      </footer>
    </nav>
  )
}

// onPanelClick gives the focus to the list a pointer clicks in, a row or a
// row's control included, once the click has done its own work: the list
// holds it with no row highlighted, and the first row key highlights one
// (onPanelKey). Safari and Firefox on macOS leave a clicked button
// unfocused, so without this the keys had nothing to start from. opening a
// conversation still puts the cursor in its reply box afterwards, as
// InputPanel does for any way of opening it. a click made by the keyboard,
// Enter or Space on a focused control (detail 0), leaves the focus where it
// is, and so does a click outside the two lists, or one that ends a text
// selection.
function onPanelClick(event: MouseEvent<HTMLElement>) {
  const target = event.target as Element
  const list = target.closest('section.needs-you') ? 'needs-you' : target.closest('section.projects') ? 'projects' : null
  if (!list || event.detail === 0 || !window.getSelection()?.isCollapsed) {
    return
  }
  focusPanel(list)
}

// onPanelKey: with the focus on one of the panel's lists and no row
// highlighted, the first row key highlights one. Home and End go to the
// panel's first and last row, and any other to the list's entry row
// (entryRow); every key after that moves from the row, as onTreeKey and
// onWaitingKey do.
function onPanelKey(event: KeyboardEvent<HTMLElement>) {
  const target = event.target as Element
  const list = target.matches('section.needs-you') ? 'needs-you' : target.matches('section.projects') ? 'projects' : null
  if (!list || !rowKeys.has(event.key) || event.altKey || event.ctrlKey || event.metaKey || event.shiftKey) {
    return
  }
  event.preventDefault()
  const rows = [...event.currentTarget.querySelectorAll<HTMLElement>('.tree-item')]
  const row = event.key === 'Home' ? rows[0] : event.key === 'End' ? rows[rows.length - 1] : entryRow(list)
  row?.focus()
}

// rowKeys are the keys a list of rows takes, with no modifier.
const rowKeys = new Set(['ArrowUp', 'ArrowDown', 'ArrowLeft', 'ArrowRight', 'Home', 'End'])

// rowOf is the row a key is pressed on, when the list takes that key: the
// row's main button, which the fold and the star on a project's row stand
// for. for any other key, or away from a row, it is null and the key is left
// alone.
function rowOf(event: KeyboardEvent<HTMLElement>): HTMLElement | null {
  if (!rowKeys.has(event.key) || event.altKey || event.ctrlKey || event.metaKey || event.shiftKey) {
    return null
  }
  const target = event.target as HTMLElement
  return target.closest('.project-row')?.querySelector<HTMLElement>('.project-item') ?? target.closest<HTMLElement>('.tree-item')
}

// rowStep is where Up and Down move the focus, one row along and never past
// either end, and Home and End, to the first and the last row. the rows are
// the whole panel's in the order they are shown: the needs-you list, then
// the tree, leaving out what is folded. so Up from the first project goes to
// the last waiting conversation, and Down from it back.
function rowStep(list: HTMLElement, here: HTMLElement, key: string): HTMLElement | undefined {
  const panel = list.closest<HTMLElement>('nav.sidebar') ?? list
  const rows = [...panel.querySelectorAll<HTMLElement>('.tree-item')]
  const at = rows.indexOf(here)
  if (at < 0) {
    return undefined
  }
  switch (key) {
    case 'ArrowDown':
      return rows[Math.min(at + 1, rows.length - 1)]
    case 'ArrowUp':
      return rows[Math.max(at - 1, 0)]
    case 'Home':
      return rows[0]
    case 'End':
      return rows[rows.length - 1]
  }
  return undefined
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
