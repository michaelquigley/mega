import { useCallback, useEffect, useId, useLayoutEffect, useRef, useState, type FocusEvent, type FormEvent, type KeyboardEvent, type PointerEvent, type RefObject } from 'react'
import { interruptConversation, releaseConversation, removeConversation, resumeConversation, sendPrompt, setModel, setPermissionMode, startRuntime, updateConversation } from '../api'
import { harnessLabel, harnessOf, harnesses } from '../harnesses'
import { place } from '../latest'
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
import { HarnessIcon } from './HarnessIcon'
import { McpPanel } from './Mcp'
import { mcpSummaryText, mcpTrouble, mcpTroubleText } from '../mcp'
import { environmentReason } from '../checkout'
import { dialogOpen } from '../modal'

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

const clampHeight = (height: number) => Math.min(Math.max(height, minHeight), maxHeight())

// settleMs is how long a docked request waits before a key can answer it.
const settleMs = 500

// replyShortcut puts the cursor in the reply box, as the box's title says.
export const replyShortcut = 'Ctrl+Space'

// isReplyFocus is ctrl+space, by physical key so it holds across layouts, from
// anywhere on the page. some desktops keep it for switching input methods and
// never pass it on; the operator's browser is to be confirmed by hand.
function isReplyFocus(event: globalThis.KeyboardEvent): boolean {
  return event.ctrlKey && !event.altKey && !event.metaKey && !event.shiftKey && event.code === 'Space'
}

// typingIn is whether focus is in a field that takes text, which opening a
// conversation never takes the focus from.
function typingIn(element: Element | null): boolean {
  if (element instanceof HTMLTextAreaElement || (element instanceof HTMLElement && element.isContentEditable)) {
    return true
  }
  return element instanceof HTMLInputElement && !['button', 'checkbox', 'radio', 'submit', 'reset', 'range', 'color', 'file', 'image'].includes(element.type)
}

// historyOf is the column the history scrolls in: what Escape hands the keys
// to, and, like the page itself, somewhere focus goes nowhere in particular.
const historyOf = (cluster: HTMLElement | null) => cluster?.closest<HTMLElement>('.main') ?? null

// why a docked request waits while the page is stale: it shows only what it
// last read, and cannot stand behind that pending set.
const heldWhileStale = 'Held until the page has caught up with mega: what it shows is only last known, and an answer waits for the workspace to be read again.'

// why the answers that would let work go on wait while a stop is under way:
// the service refuses them then, and Deny and Dismiss stay live.
const heldWhileStopping = 'The turn is stopping, so answers that would let work go on are held. Deny or Dismiss can still be pressed.'

// the words the header used for a runtime started in an older environment.
const staleEnvironment = 'started in an older environment: the project’s has changed since, and the next start picks it up'


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
  onTranscript?: () => void
  newer: boolean
  onLatest: () => void
  // dock is the panel's own element, which the conversation watches for size;
  // isFollowing says whether the view follows the bottom (latest.ts).
  dock: RefObject<HTMLDivElement | null>
  isFollowing: () => boolean
  // why new work waits (a shutdown armed, or done from this page), or null:
  // Send and Start wait, and say so.
  heldBy: string | null
}

type Open = 'release' | 'rename' | 'remove' | 'mcp' | null

// InputPanel is the conversation's control cluster, pinned to the bottom of
// the column: the pending requests docked above everything else, the
// activity line, the reply box, and the control bar. the pending cards here
// are the one place a request can be answered, driven by the service's
// current pending set; the history keeps only a row for each. the reply box
// is one line at rest, takes the focus when the conversation is opened, and
// opens once it holds a draft and focus is anywhere in the cluster.
export function InputPanel(props: InputPanelProps) {
  const { conversation, project, log, stale, busy, error, attempt, showNative, onShowNative, onRemoved, onTranscript, newer, onLatest, heldBy, dock, isFollowing } = props
  const [draft, setDraft] = useState(() => drafts.get(conversation.id) ?? '')
  const [expanded, setExpanded] = useState(false)
  const [height, setHeight] = useState(readHeight)
  const [menu, setMenu] = useState(false)
  const [open, setOpen] = useState<Open>(null)
  const clusterRef = dock
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

  // opening a conversation, however it was opened, puts the cursor in its
  // reply box, at the end of any draft: never while a dialog is open, and
  // never taking the focus from a field the operator is typing in. focus
  // alone leaves the box at one line, so the history does not move; a draft
  // kept from before opens it.
  useEffect(() => {
    const box = boxRef.current
    if (!box || dialogOpen() || typingIn(document.activeElement)) {
      return
    }
    box.focus({ preventScroll: true })
    box.setSelectionRange(box.value.length, box.value.length)
  }, [])

  // ctrl+space puts the cursor in the reply box from anywhere on the page,
  // and does nothing while a dialog is open or with the cursor already there.
  // the key is taken either way, so a space never presses the button that has
  // the focus, such as a dialog's close.
  useEffect(() => {
    const onKey = (event: globalThis.KeyboardEvent) => {
      if (!isReplyFocus(event)) {
        return
      }
      event.preventDefault()
      const box = boxRef.current
      if (box && box !== document.activeElement && !dialogOpen()) {
        setMenu(false)
        box.focus({ preventScroll: true })
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [])
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
  const docked = [...pending].sort((a, b) => (Date.parse(a.at ?? '') || 0) - (Date.parse(b.at ?? '') || 0))
  // the approval the keys answer: the oldest docked, never a question, so
  // never a pi dialog either.
  const keyed = docked.find((request) => request.kind === 'permission' && !request.payload?.dialog)
  const hasRuntime = state === 'running' || state === 'stopping'
  // a stop under way: an interrupt not yet answered by the turn's end, or a
  // release.
  const stopping = state === 'stopping' || !!conversation.interrupting
  const blocked = state === 'unsupervised' || state === 'starting' || state === 'stopping' || active
  const activity = describeActivity(conversation, log, stale)
  // the placeholder says what is true: the activity line's own words while a
  // prompt cannot be sent.
  const placeholder = blocked && activity ? activity.text : heldBy ??  'Reply (Ctrl+Enter to send)'

  // expanded, the box opens to the remembered height and grows with its
  // content to 60% of the viewport, then scrolls. collapsed, it is one line
  // showing the start of any draft.
  //
  // when the box opens, closes, grows or is dragged, a view following the
  // bottom stays there, as for any change in the panel's height (latest.ts).
  // a reader who is not at the bottom keeps their place: the history scrolls
  // by exactly how far the panel's top edge moved, so the line just above the
  // panel stays just above it. after a send, once the box has closed, the
  // view goes to the newest activity and follows it.
  //
  // fit pins a following view itself, rather than leaving it to the
  // conversation's resize observer: measuring the box's content collapses it
  // for a moment, which can clamp the column's scroll. pinned here, and
  // recorded through place, the scroll event that clamp queues finds the view
  // where mega left it. whether the view follows is the conversation's one
  // reckoning (isFollowing), taken before the box changes.
  const fit = useCallback(() => {
    const box = boxRef.current
    if (!box) {
      return
    }
    const cluster = clusterRef.current
    const scroller = cluster?.closest<HTMLElement>('.main') ?? null
    const topBefore = cluster?.getBoundingClientRect().top ?? 0
    const following = isFollowing()
    if (!expanded) {
      box.style.height = ''
      box.scrollTop = 0
    } else {
      box.style.height = '0px'
      const content = box.scrollHeight + (box.offsetHeight - box.clientHeight)
      box.style.height = `${Math.min(Math.max(height, content), maxHeight())}px`
    }
    if (scroller && cluster && fitted.current) {
      // mega's own moves, recorded through place (latest.ts), so the scroll
      // events they queue are never read as the reader's.
      if (following) {
        place(scroller, scroller.scrollHeight)
      } else {
        place(scroller, scroller.scrollTop + topBefore - cluster.getBoundingClientRect().top)
      }
    }
    fitted.current = true
    if (toLatestAfterSend.current) {
      toLatestAfterSend.current = false
      latest.current()
    }
  }, [expanded, height, isFollowing, clusterRef])
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
  }, [expanded, clusterRef])

  // the box opens at the first character typed, and when focus enters the
  // reply box or the control bar while it holds a draft; focus alone on an
  // empty box leaves it at one line. emptied, it stays open until focus
  // leaves. it closes only when focus leaves the whole cluster: the menu and
  // the dialogs opened from it are inside. focus on a docked card neither
  // opens nor closes it, since either would move the card under the pointer
  // mid-press.
  const onFocus = (event: FocusEvent) => {
    if (draft !== '' && formRef.current?.contains(event.target as Node)) {
      setExpanded(true)
    }
  }
  // focus that goes nowhere (an answered card removed under it, the window
  // losing focus, a press on the history, which takes focus itself so its
  // keys scroll) is not leaving: a press outside closes the box, and so do
  // Escape and a send.
  const onBlur = (event: FocusEvent) => {
    const to = event.relatedTarget as Node | null
    if (to !== null && to !== historyOf(clusterRef.current) && !clusterRef.current?.contains(to)) {
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

  // when each docked request first appeared on this page, so a key pressed
  // for something else cannot answer one that docked under it.
  const dockedAt = useRef(new Map<string, number>())
  useLayoutEffect(() => {
    const seen = dockedAt.current
    const now = performance.now()
    const ids = new Set(pending.map((request) => request.id))
    for (const id of ids) {
      if (!seen.has(id)) {
        seen.set(id, now)
      }
    }
    for (const id of seen.keys()) {
      if (!ids.has(id)) {
        seen.delete(id)
      }
    }
  }, [pending])

  // allowByKey answers the keyed approval as its own button does, by pressing
  // it: Ctrl+Enter its Allow, Ctrl+Shift+Enter its "Allow this tool for the
  // session", which a request that does not offer it has not, so the key does
  // nothing there. only a deliberate press from an empty reply box or the
  // page itself, with no dialog open: never a repeat, never on a request
  // docked less than settleMs before, and one request per press, since a
  // card being answered has its buttons disabled.
  const allowByKey = (event: globalThis.KeyboardEvent) => {
    if (event.repeat || event.isComposing || event.altKey || draft !== '' || !keyed || stale || stopping) {
      return
    }
    if (dialogOpen()) {
      return
    }
    const at = dockedAt.current.get(keyed.id)
    // the press's own time, on the same clock as performance.now().
    if (at === undefined || event.timeStamp - at < settleMs) {
      return
    }
    const decision = event.shiftKey ? 'allow_session' : 'allow'
    clusterRef.current?.querySelector<HTMLButtonElement>(`[data-request="${CSS.escape(keyed.id)}"] button[data-answer="${decision}"]`)?.click()
  }
  // the page itself: focus on nothing in particular, or on the history's
  // column, which takes focus so its keys scroll. anywhere else, another
  // field, a button, a menu, the keys are left alone.
  const allowRef = useRef(allowByKey)
  useEffect(() => {
    allowRef.current = allowByKey
  })
  useEffect(() => {
    const onKey = (event: globalThis.KeyboardEvent) => {
      const focus = document.activeElement
      const page = focus === null || focus === document.body || focus === document.documentElement || focus === historyOf(clusterRef.current)
      if (event.key !== 'Enter' || !(event.ctrlKey || event.metaKey) || !page) {
        return
      }
      event.preventDefault()
      allowRef.current(event)
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  },[clusterRef])

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
      const name = harnessLabel(conversation.harness)
      if (outcome === 'refused') {
        throw new Error(`${name} refused the change: ${message ?? 'no reason given'}`)
      }
      if (outcome === 'unconfirmed') {
        throw new Error(message ?? `${name} did not confirm the change in time.`)
      }
    })

  const settling = state === 'starting' || state === 'stopping'
  const canResume = state === 'unsupervised'
  // start: a stopped conversation with a session to resume, sending nothing.
  // never while unsupervised, which Resume settles first, and never on its
  // own: only this gesture starts a runtime without a prompt.
  const canStart = state === 'none' && !!conversation.native_id
  const harness = harnessOf(conversation.harness)
  const terminal =
    conversation.native_id && harness?.terminal ? `cd ${project.checkout} && ${harness.terminal} ${conversation.native_id}` : null
  // a pi dialog holds pi open: closing its input does not stop it, so a
  // release ends its process, and the command the dialog gates never runs.
  const piDialogPending = conversation.harness === 'pi' && pending.length > 0
  // the environment is on the bar only when it needs attention: the runtime
  // started in an older environment than the project's now, or the project's
  // environment refuses new work. the fingerprint and the names stay in the
  // header, where they are identity.
  const environmentNote = conversation.environment_stale ? staleEnvironment : environmentReason(project)
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
            <RequestCard key={request.id} conversationId={conversation.id} request={request} keyed={request.id === keyed?.id} held={stale ? heldWhileStale : null} stopping={stopping ? heldWhileStopping : null} />
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
          title={`Reply: ${replyShortcut} puts the cursor here from anywhere on the page, Escape leaves it for the history, and Ctrl+Enter sends`}
          aria-keyshortcuts="Control+Space"
          placeholder={placeholder}
          value={draft}
          aria-autocomplete="list"
          aria-controls={search.mention && search.paths.length > 0 ? listId : undefined}
          aria-activedescendant={search.mention && search.paths.length > 0 ? `${listId}-${search.selected}` : undefined}
          onChange={(event) => {
            changeDraft(event.target.value)
            if (event.target.value !== '') {
              setExpanded(true)
            }
            search.track()
          }}
          onSelect={search.track}
          onScroll={search.track}
          onBlur={search.close}
          onKeyDown={(event) => {
            // Ctrl+Enter always sends a draft, list or no list; from an
            // empty box it answers the oldest docked approval (allowByKey).
            if (event.key === 'Enter' && (event.ctrlKey || event.metaKey)) {
              event.preventDefault()
              if (draft === '') {
                allowByKey(event.nativeEvent)
              } else {
                void onSend()
              }
              return
            }
            if (search.onKeyDown(event)) {
              event.preventDefault()
              event.stopPropagation()
              return
            }
            const plain = !event.ctrlKey && !event.metaKey && !event.altKey && !event.shiftKey && !event.nativeEvent.isComposing
            // Escape, with the path list closed (it took the key above),
            // leaves the box for the history, so the keys scroll it. the box
            // closes as it would for any press outside, its draft kept.
            if (event.key === 'Escape' && plain) {
              const history = historyOf(clusterRef.current)
              if (history) {
                event.preventDefault()
                setExpanded(false)
                setMenu(false)
                history.focus({ preventScroll: true })
              }
              return
            }
            // Page Up and Page Down read the history while the box is empty:
            // the reader's own scroll, which ends following the bottom as a
            // wheel does (latest.ts). with a draft they are the box's.
            if ((event.key === 'PageUp' || event.key === 'PageDown') && plain && draft === '') {
              const history = historyOf(clusterRef.current)
              if (history) {
                event.preventDefault()
                const page = Math.max(history.clientHeight - (clusterRef.current?.offsetHeight ?? 0), 40) * 0.875
                history.scrollBy({ top: event.key === 'PageUp' ? -page : page, behavior: 'instant' })
              }
            }
          }}
        />
        <PathList search={search} id={listId} />
        <div className="control-bar">
          <div className="bar-start">
            {/* which harness this conversation runs on, always first. */}
            <HarnessIcon harness={conversation.harness} />
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
            {environmentNote && (
              <span className="bar-mark env-mark" role="img" aria-label={environmentNote} title={environmentNote}>
                <Icon name="warning_amber" />
                <span className="word">environment</span>
              </span>
            )}
          </div>
          <div className="bar-end">
            {harness?.permissionModes && state !== 'unsupervised' && (
              <ModeControl
                mode={conversation.permission_mode_choice ?? ''}
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
              {harness?.listsMcp && mcpTrouble(conversation) && <span className="more-mark" role="img" aria-label={mcpTroubleText} title={mcpTroubleText} />}
              {menu && (
                <div className="menu" role="menu">
                  {harness?.listsMcp && <MenuItem icon="extension" word={mcpSummaryText(conversation)} onClick={() => choose('mcp')} />}
                  <MenuItem icon="logout" word="Release" disabled={busy || state !== 'running'} onClick={() => choose('release')} />
                  {onTranscript && (
                    <MenuItem icon="notes" word="View native transcript" onClick={() => { choose(null); onTranscript() }} />
                  )}
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

      {open === 'mcp' && <McpPanel conversation={conversation} onClose={() => setOpen(null)} />}
      {open === 'release' && (
        <Dialog title="Release this conversation" onClose={() => setOpen(null)}>
          <p>
            Release stops the runtime and frees the session, so you can continue in a terminal.
            {(active || pending.length > 0) && ' The turn in flight is interrupted first.'}
          </p>
          {piDialogPending && (
            <p className="notice release-pi-dialog">
              A pi dialog is waiting for your answer. pi cannot be stopped politely while it waits, so releasing ends its process, and the
              command the dialog gates does not run. The dialog is recorded as never answered.
            </p>
          )}
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
          {conversation.harness === 'pi' ? (
            <p>
              Close the terminal before you start or send a prompt here again. pi has no lock, so a terminal still open on the session and mega
              would both write to its file, and mega cannot tell that one is open.
            </p>
          ) : (
            <p>Close the terminal before you start or send a prompt here again; an idle terminal still holds the session.</p>
          )}
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

function MenuItem({ icon, word, disabled, onClick }: { icon: 'logout' | 'play_arrow' | 'edit' | 'delete' | 'extension' | 'notes'; word: string; disabled?: boolean; onClick: () => void }) {
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
