import { useCallback, useEffect, useId, useLayoutEffect, useRef, useState, type FocusEvent, type FormEvent, type KeyboardEvent, type PointerEvent } from 'react'
import { interruptConversation, releaseConversation, removeConversation, resumeConversation, sendPrompt, setModel, setPermissionMode, startRuntime, updateConversation } from '../api'
import { harnesses } from '../harnesses'
import type { ConversationLog } from '../log'
import type { Conversation, Project } from '../types'
import { describeActivity } from '../activity'
import { usePathSearch } from '../mention'
import { ActivityLine } from './ActivityLine'
import { ModeControl, ModelControl } from './Controls'
import { PathList } from './PathList'
import { Dialog } from './Dialog'
import { Icon, IconButton } from './Icon'
import { CopyButton } from './Copy'
import { RequestCard } from './RequestCard'

// drafts stay in the browser, per conversation, and never reach the service.
// they survive collapsing the box, switching conversations and reconnects.
const drafts = new Map<string, string>()

// the reply box's remembered height: one size for every conversation, a
// preference kept in this browser only.
const heightKey = 'mega.replyHeight'
const defaultHeight = 120
const minHeight = 48

function readHeight(): number {
  try {
    const stored = Number(window.localStorage.getItem(heightKey))
    return Number.isFinite(stored) && stored >= minHeight ? stored : defaultHeight
  } catch {
    return defaultHeight
  }
}

function saveHeight(height: number) {
  try {
    window.localStorage.setItem(heightKey, String(Math.round(height)))
  } catch {
    // a browser that keeps nothing still works; the size is only a preference.
  }
}

const maxHeight = () => Math.round(window.innerHeight * 0.6)

// how close to the bottom counts as following it, as the conversation's own
// follow-at-bottom reckons it (latest.ts).
const followSlack = 32
const clampHeight = (height: number) => Math.min(Math.max(height, minHeight), maxHeight())

const terminalCommands: Record<string, string> = { claude: 'claude --resume', codex: 'codex resume' }
const harnessName = (value: string) => harnesses.find((h) => h.value === value)?.label ?? value

interface InputPanelProps {
  conversation: Conversation
  project: Project
  log: ConversationLog | null
  stale: boolean
  busy: boolean
  error: string | null
  attempt: (action: () => Promise<unknown>) => Promise<boolean>
  showNative: boolean
  onShowNative: (show: boolean) => void
  onRemoved: () => void
  newer: boolean
  onLatest: () => void
  // why new work waits (a shutdown armed, or done from this page), or null:
  // Send and Start wait, and say so.
  heldBy: string | null
}

type Open = 'release' | 'rename' | 'remove' | null

// InputPanel is the conversation's control cluster, pinned to the bottom of
// the column: the pending requests docked above everything else, the
// activity line, the reply box, and the control bar. the pending cards here
// are the one place a request can be answered, driven by the service's
// current pending set; the history keeps only a row for each. the reply box
// is one line at rest and opens while focus is anywhere in the cluster.
export function InputPanel(props: InputPanelProps) {
  const { conversation, project, log, stale, busy, error, attempt, showNative, onShowNative, onRemoved, newer, onLatest, heldBy } = props
  const [draft, setDraft] = useState(() => drafts.get(conversation.id) ?? '')
  const [expanded, setExpanded] = useState(false)
  const [height, setHeight] = useState(readHeight)
  const [menu, setMenu] = useState(false)
  const [open, setOpen] = useState<Open>(null)
  const clusterRef = useRef<HTMLDivElement>(null)
  const boxRef = useRef<HTMLTextAreaElement>(null)
  const moreRef = useRef<HTMLDivElement>(null)
  const formRef = useRef<HTMLFormElement>(null)
  // fitted: the box has been sized once, so later changes move the view;
  // toLatestAfterSend: a sent prompt's view change waits for the box to close.
  const fitted = useRef(false)
  const toLatestAfterSend = useRef(false)
  // onLatest is a new function each render; the sizing reads the newest.
  const latest = useRef(onLatest)
  useEffect(() => {
    latest.current = onLatest
  })
  const changeDraft = (text: string) => {
    setDraft(text)
    drafts.set(conversation.id, text)
  }
  // the @ list: a path picked from the checkout goes into the draft, with the
  // @ kept only where the harness attaches the file it names.
  const keepAt = harnesses.find((h) => h.value === conversation.harness)?.attachesMentions ?? false
  const search = usePathSearch(project.id, keepAt, boxRef, changeDraft)
  const listId = useId()

  // the menu takes focus when it opens, and closes on Escape or on a press
  // anywhere outside it.
  useEffect(() => {
    if (!menu) {
      return
    }
    moreRef.current?.querySelector<HTMLButtonElement>('.menu button:not(:disabled)')?.focus()
    const outside = (event: globalThis.PointerEvent) => {
      if (!moreRef.current?.contains(event.target as Node)) {
        setMenu(false)
      }
    }
    document.addEventListener('pointerdown', outside)
    return () => document.removeEventListener('pointerdown', outside)
  }, [menu])

  const { state, turn_active: active, pending } = conversation
  const background = conversation.background ?? []
  const hasRuntime = state === 'running' || state === 'stopping'
  const blocked = state === 'unsupervised' || state === 'starting' || state === 'stopping' || state === 'resuming' || active
  const activity = describeActivity(conversation, log, stale)
  // the placeholder says what is true: the activity line's own words while a
  // prompt cannot be sent.
  const placeholder = blocked && activity ? activity.text : heldBy ??  'Reply (Ctrl+Enter to send)'

  // expanded, the box opens to the remembered height and grows with its
  // content to 60% of the viewport, then scrolls. collapsed, it is one line
  // showing the start of any draft.
  //
  // the conversation moves with the panel's top edge: when the box opens,
  // closes, grows or is dragged, the history scrolls by exactly how far that
  // edge moved, so the line just above the panel stays just above it. a view
  // following the bottom stays at the bottom. after a send, once the box has
  // closed, the view goes to the newest activity and follows it.
  const fit = useCallback(() => {
    const box = boxRef.current
    if (!box) {
      return
    }
    const cluster = clusterRef.current
    const scroller = cluster?.closest<HTMLElement>('.main') ?? null
    const topBefore = cluster?.getBoundingClientRect().top ?? 0
    const following = scroller !== null && scroller.scrollHeight - scroller.scrollTop - scroller.clientHeight <= followSlack
    if (!expanded) {
      box.style.height = ''
      box.scrollTop = 0
    } else {
      box.style.height = '0px'
      const content = box.scrollHeight + (box.offsetHeight - box.clientHeight)
      box.style.height = `${Math.min(Math.max(height, content), maxHeight())}px`
    }
    if (scroller && cluster && fitted.current) {
      if (following) {
        scroller.scrollTop = scroller.scrollHeight
      } else {
        scroller.scrollTop += topBefore - cluster.getBoundingClientRect().top
      }
    }
    fitted.current = true
    if (toLatestAfterSend.current) {
      toLatestAfterSend.current = false
      latest.current()
    }
  }, [expanded, height])
  useLayoutEffect(fit, [fit, draft])
  useEffect(() => {
    window.addEventListener('resize', fit)
    return () => window.removeEventListener('resize', fit)
  }, [fit])

  // a press outside the cluster leaves it too, even where focus had already
  // fallen to the page body without a blur the cluster could see.
  useEffect(() => {
    if (!expanded) {
      return
    }
    const outside = (event: globalThis.PointerEvent) => {
      const target = event.target as Element | null
      if (!clusterRef.current?.contains(target) && !target?.closest('dialog')) {
        setExpanded(false)
      }
    }
    document.addEventListener('pointerdown', outside)
    return () => document.removeEventListener('pointerdown', outside)
  }, [expanded])

  // the box opens when focus enters the reply box or the control bar, and
  // closes only when focus leaves the whole cluster: the menu and the dialogs
  // opened from it are inside. focus on a docked card does neither, since
  // either would move the card under the pointer mid-press.
  const onFocus = (event: FocusEvent) => {
    if (formRef.current?.contains(event.target as Node)) {
      setExpanded(true)
    }
  }
  // focus that goes nowhere (an answered card removed under it, the window
  // losing focus) is not leaving: a press outside closes the box, and so does
  // a send.
  const onBlur = (event: FocusEvent) => {
    const to = event.relatedTarget as Node | null
    if (to !== null && !clusterRef.current?.contains(to)) {
      setExpanded(false)
      setMenu(false)
    }
  }

  const onDragStart = (event: PointerEvent<HTMLDivElement>) => {
    // keep focus where it is, so dragging never collapses the box.
    event.preventDefault()
    const startY = event.clientY
    const startHeight = boxRef.current?.offsetHeight ?? height
    const target = event.currentTarget
    target.setPointerCapture(event.pointerId)
    let latest = startHeight
    const move = (e: globalThis.PointerEvent) => {
      latest = clampHeight(startHeight + startY - e.clientY)
      setHeight(latest)
    }
    const up = () => {
      target.removeEventListener('pointermove', move)
      target.removeEventListener('pointerup', up)
      saveHeight(latest)
    }
    target.addEventListener('pointermove', move)
    target.addEventListener('pointerup', up)
  }
  const onDragKey = (event: KeyboardEvent<HTMLDivElement>) => {
    const step = event.key === 'ArrowUp' ? 16 : event.key === 'ArrowDown' ? -16 : 0
    if (step) {
      event.preventDefault()
      const next = clampHeight((boxRef.current?.offsetHeight ?? height) + step)
      setHeight(next)
      saveHeight(next)
    }
  }

  const onSend = async (event?: FormEvent) => {
    event?.preventDefault()
    const text = draft.trim()
    if (!text || blocked) {
      return
    }
    // a send the service refuses changes nothing: the box stays open with the
    // draft, and the error shows.
    if (await attempt(() => sendPrompt(conversation.id, text))) {
      setDraft('')
      drafts.delete(conversation.id)
      // sent: the box closes, focus leaves it so it stays closed, and the view
      // goes to the newest activity, following the reply as it arrives.
      toLatestAfterSend.current = true
      boxRef.current?.blur()
      setExpanded(false)
    }
  }

  const switchMode = (mode: string) =>
    void attempt(async () => {
      const { outcome, message } = await setPermissionMode(conversation.id, mode)
      if (outcome === 'refused') {
        throw new Error(`Claude refused the switch: ${message ?? 'no reason given'}`)
      }
      if (outcome === 'unconfirmed') {
        throw new Error(message ?? 'Claude did not confirm the switch in time.')
      }
    })
  const applyModel = (model: string, effort: string) =>
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

  const settling = state === 'starting' || state === 'stopping' || state === 'resuming'
  const canResume = (state === 'none' || state === 'unsupervised') && !!conversation.native_id
  // start: a stopped conversation with a session to resume, sending nothing.
  // never while unsupervised, which Resume settles first, and never on its
  // own: only this gesture starts a runtime without a prompt.
  const canStart = state === 'none' && !!conversation.native_id
  const terminal =
    conversation.native_id && terminalCommands[conversation.harness]
      ? `cd ${project.checkout} && ${terminalCommands[conversation.harness]} ${conversation.native_id}`
      : null
  const docked = [...pending].sort((a, b) => (Date.parse(a.at ?? '') || 0) - (Date.parse(b.at ?? '') || 0))
  // a dialog opened from the menu hands focus back to the More button when it
  // closes, so focus stays in the cluster.
  const choose = (next: Open) => {
    moreRef.current?.querySelector<HTMLButtonElement>('button')?.focus()
    setMenu(false)
    setOpen(next)
  }

  return (
    <div className={`panel-cluster${expanded ? ' expanded' : ''}`} ref={clusterRef} onFocus={onFocus} onBlur={onBlur}>
      {newer && (
        <div className="newer">
          <IconButton icon="arrow_downward" label="Newer activity below" word="Newer activity below" onClick={onLatest} />
        </div>
      )}
      {docked.length > 0 && (
        <div className="docked">
          {docked.map((request) => (
            <RequestCard key={request.id} conversationId={conversation.id} request={request} />
          ))}
        </div>
      )}
      {error && <p className="error">{error}</p>}
      <ActivityLine conversation={conversation} log={log} stale={stale} />
      <form className="reply" ref={formRef} onSubmit={(event) => void onSend(event)}>
        {expanded && (
          <div
            className="drag-handle"
            role="separator"
            aria-orientation="horizontal"
            aria-label="Resize the reply box"
            title="Drag to resize the reply box"
            tabIndex={0}
            onPointerDown={onDragStart}
            onKeyDown={onDragKey}
          />
        )}
        <textarea
          ref={boxRef}
          rows={1}
          aria-label="Reply"
          placeholder={placeholder}
          value={draft}
          aria-autocomplete="list"
          aria-controls={search.mention && search.paths.length > 0 ? listId : undefined}
          aria-activedescendant={search.mention && search.paths.length > 0 ? `${listId}-${search.selected}` : undefined}
          onChange={(event) => {
            changeDraft(event.target.value)
            search.track()
          }}
          onSelect={search.track}
          onScroll={search.track}
          onBlur={search.close}
          onKeyDown={(event) => {
            // Ctrl+Enter always sends, list or no list.
            if (event.key === 'Enter' && (event.ctrlKey || event.metaKey)) {
              event.preventDefault()
              void onSend()
              return
            }
            if (search.onKeyDown(event)) {
              event.preventDefault()
              event.stopPropagation()
            }
          }}
        />
        <PathList search={search} id={listId} />
        <div className="control-bar">
          <div className="bar-start">
            <IconButton
              type="submit"
              icon="send"
              label={heldBy ??  'Send (Ctrl+Enter)'}
              disabled={busy || blocked || draft.trim() === '' || heldBy !== null}
            />
            {canStart && (
              <IconButton
                icon="power_settings_new"
                label={heldBy ??  'Start the runtime on this session, sending nothing'}
                disabled={busy || heldBy !== null}
                onClick={() => void attempt(() => startRuntime(conversation.id))}
              />
            )}
            {hasRuntime && (active || pending.length > 0) && (
              <IconButton icon="stop" label="Interrupt" disabled={busy} onClick={() => void attempt(() => interruptConversation(conversation.id))} />
            )}
          </div>
          <div className="bar-end">
            {conversation.harness === 'claude' && state !== 'unsupervised' && (
              <ModeControl
                live={state === 'running' && !!conversation.permission_mode}
                mode={(state === 'running' && conversation.permission_mode) || conversation.permission_mode_choice}
                inheritedMode={project.inherited_permission_mode}
                disabled={busy || stale || settling || (state === 'running' && !conversation.permission_mode)}
                onChange={switchMode}
              />
            )}
            {state !== 'unsupervised' && (
              <ModelControl
                key={`${conversation.model_choice ?? ''}\n${conversation.effort_choice ?? ''}`}
                conversation={conversation}
                projectId={project.id}
                disabled={busy || stale || settling || (state === 'running' && !conversation.model)}
                onApply={applyModel}
              />
            )}
            <div
              className="more"
              ref={moreRef}
              onKeyDown={(event) => {
                if (event.key === 'Escape' && menu) {
                  event.preventDefault()
                  setMenu(false)
                  moreRef.current?.querySelector<HTMLButtonElement>('button')?.focus()
                }
              }}
            >
              <IconButton icon="more_vert" label="More" aria-haspopup="menu" aria-expanded={menu} onClick={() => setMenu(!menu)} />
              {menu && (
                <div className="menu" role="menu">
                  <MenuItem icon="logout" word="Release" disabled={busy || state !== 'running'} onClick={() => choose('release')} />
                  {canResume && (
                    <MenuItem
                      icon="play_arrow"
                      word="Resume"
                      disabled={busy}
                      onClick={() => {
                        setMenu(false)
                        void attempt(() => resumeConversation(conversation.id))
                      }}
                    />
                  )}
                  <MenuItem icon="edit" word="Name and label" onClick={() => choose('rename')} />
                  <MenuItem icon="delete" word="Remove from workspace" onClick={() => choose('remove')} />
                  <button
                    type="button"
                    role="menuitemcheckbox"
                    aria-checked={showNative}
                    title="Show the harness's own events in the history"
                    aria-label="Native events"
                    className="menu-item"
                    disabled={!log?.loaded}
                    onClick={() => {
                      setMenu(false)
                      onShowNative(!showNative)
                    }}
                  >
                    <Icon name="visibility" />
                    <span className="word">Native events</span>
                    <span className="check">{showNative ? <Icon name="check" /> : null}</span>
                  </button>
                </div>
              )}
            </div>
          </div>
        </div>
      </form>

      {open === 'release' && (
        <Dialog title="Release this conversation" onClose={() => setOpen(null)}>
          <p>
            Release stops the runtime and frees the session, so you can continue in a terminal.
            {(active || pending.length > 0) && ' The turn in flight is interrupted first.'}
          </p>
          {background.length > 0 && (
            <div className="release-background">
              <p>
                {background.length === 1 ? '1 background job is' : `${background.length} background jobs are`} running:
              </p>
              <ul>
                {background.map((job) => (
                  <li key={job.id}>
                    <code>{job.description}</code>
                  </li>
                ))}
              </ul>
              <p>
                {conversation.harness === 'claude'
                  ? 'Claude waits for them before it stops, and when they finish it takes one more turn on its own, so the release lasts until then.'
                  : 'Codex is not asked to wait for them. Any still running when it exits are reported in the history.'}
              </p>
            </div>
          )}
          {terminal ? (
            <div className="command">
              <code>{terminal}</code>
              <CopyButton text={terminal} />
            </div>
          ) : (
            <p className="muted">This conversation has no session yet to continue elsewhere.</p>
          )}
          <p>Close the terminal before you resume or prompt here again; an idle terminal still holds the session.</p>
          <div className="actions">
            <IconButton
              icon="logout"
              label="Release"
              word="Release"
              disabled={busy || state !== 'running'}
              onClick={() => {
                setOpen(null)
                void attempt(() => releaseConversation(conversation.id))
              }}
            />
          </div>
        </Dialog>
      )}
      {open === 'rename' && (
        <Dialog title="Name and label" onClose={() => setOpen(null)}>
          <RenameForm
            conversation={conversation}
            onSave={async (name, label) => {
              if (await attempt(() => updateConversation(conversation.id, name, label))) {
                setOpen(null)
              }
            }}
          />
        </Dialog>
      )}
      {open === 'remove' && (
        <Dialog title="Remove from workspace" onClose={() => setOpen(null)}>
          <p>
            mega forgets this conversation: its own copy of the history and its reading position are deleted. The
            checkout and the native session stay untouched, and the session can be adopted again from the project.
          </p>
          {state === 'unsupervised' && <p className="muted">Resume it first; its runtime may still be working.</p>}
          {state !== 'none' && state !== 'unsupervised' && <p className="muted">Release its runtime first.</p>}
          <div className="actions">
            <IconButton
              icon="delete"
              label="Remove from workspace"
              word="Remove"
              disabled={busy || state !== 'none'}
              onClick={() =>
                void attempt(() => removeConversation(conversation.id)).then((removed) => {
                  if (removed) {
                    drafts.delete(conversation.id)
                    setOpen(null)
                    onRemoved()
                  }
                })
              }
            />
          </div>
        </Dialog>
      )}
    </div>
  )
}

function MenuItem({ icon, word, disabled, onClick }: { icon: 'logout' | 'play_arrow' | 'edit' | 'delete'; word: string; disabled?: boolean; onClick: () => void }) {
  return (
    <button type="button" role="menuitem" className="menu-item" title={word} aria-label={word} disabled={disabled} onClick={onClick}>
      <Icon name={icon} />
      <span className="word">{word}</span>
    </button>
  )
}

function RenameForm({ conversation, onSave }: { conversation: Conversation; onSave: (name: string, label: string) => Promise<void> }) {
  const [name, setName] = useState(conversation.name)
  const [label, setLabel] = useState(conversation.label)
  const unchanged = name.trim() === conversation.name && label.trim() === conversation.label
  return (
    <form
      className="dialog-form"
      onSubmit={(event) => {
        event.preventDefault()
        void onSave(name, label)
      }}
    >
      <label>
        Name
        <input value={name} onChange={(event) => setName(event.target.value)} />
      </label>
      <label>
        Label
        <input placeholder="Label, such as planning" value={label} onChange={(event) => setLabel(event.target.value)} />
      </label>
      <div className="actions">
        <IconButton type="submit" icon="check" label="Save the name and label" word="Save" disabled={name.trim() === '' || unchanged} />
      </div>
    </form>
  )
}
