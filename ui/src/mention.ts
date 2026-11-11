import { useCallback, useEffect, useLayoutEffect, useRef, useState, type KeyboardEvent, type RefObject } from 'react'
import { getPaths } from './api'

// a mention being typed: the @ that starts the word at the caret, what follows
// it up to the caret, and where the caret sits in the box, for the list.
export interface Mention {
  start: number
  query: string
  left: number
  top: number
}

// mentionAt finds the @-word the caret is in: a word that starts with @, at
// the start of the text or after whitespace, with no whitespace between the @
// and the caret.
export function mentionAt(text: string, caret: number): { start: number; query: string } | null {
  let start = caret
  while (start > 0 && !/\s/.test(text[start - 1])) {
    start--
  }
  if (text[start] !== '@' || start >= caret) {
    return null
  }
  return { start, query: text.slice(start + 1, caret) }
}

// insertPath puts a picked path in place of the @-word at start, through the
// end of that word, followed by a space. the leading @ stays only where the
// harness attaches mentioned files.
export function insertPath(text: string, start: number, path: string, keepAt: boolean): { text: string; caret: number } {
  let end = start
  while (end < text.length && !/\s/.test(text[end])) {
    end++
  }
  const inserted = `${keepAt ? '@' : ''}${path}`
  const rest = text.slice(end)
  const spaced = rest === '' || !/^\s/.test(rest) ? ' ' : ''
  return { text: text.slice(0, start) + inserted + spaced + rest, caret: start + inserted.length + 1 }
}

// the textarea styles that decide where text falls, copied to a mirror.
const mirrored = [
  'boxSizing', 'width', 'borderTopWidth', 'borderRightWidth', 'borderBottomWidth', 'borderLeftWidth', 'paddingTop', 'paddingRight',
  'paddingBottom', 'paddingLeft', 'fontStyle', 'fontVariant', 'fontWeight', 'fontStretch', 'fontSize', 'lineHeight', 'fontFamily',
  'textAlign', 'textTransform', 'textIndent', 'letterSpacing', 'wordSpacing', 'tabSize', 'whiteSpace', 'overflowWrap', 'wordBreak',
] as const

// caretPoint is where the character at index starts, relative to the box's
// visible top left, measured with a hidden copy of the box's text.
function caretPoint(box: HTMLTextAreaElement, index: number): { left: number; top: number } {
  const style = window.getComputedStyle(box)
  const mirror = document.createElement('div')
  for (const name of mirrored) {
    mirror.style[name] = style[name]
  }
  mirror.style.position = 'absolute'
  mirror.style.visibility = 'hidden'
  mirror.style.top = '0'
  mirror.style.left = '-9999px'
  mirror.style.whiteSpace = 'pre-wrap'
  mirror.style.overflowWrap = 'break-word'
  mirror.textContent = box.value.slice(0, index)
  const mark = document.createElement('span')
  mark.textContent = box.value.slice(index) || '.'
  mirror.appendChild(mark)
  document.body.appendChild(mirror)
  const point = { left: mark.offsetLeft - box.scrollLeft, top: mark.offsetTop - box.scrollTop }
  mirror.remove()
  return point
}

// how long typing settles before the service is asked.
const settle = 60

export interface PathSearch {
  mention: Mention | null
  // ready: the service has answered a search since the page loaded.
  ready: boolean
  paths: string[]
  error: string | null
  selected: number
  // track reads the box after any change of text or caret.
  track: () => void
  close: () => void
  // onKeyDown takes the list's keys while it is open; true if it took one.
  onKeyDown: (event: KeyboardEvent<HTMLTextAreaElement>) => boolean
  pick: (path: string) => void
  hover: (index: number) => void
}

// usePathSearch is the reply box's @ list: open while the caret is in an
// @-word, filtered by what follows the @, from the service's search of the
// checkout. Escape closes it for that word; a space ends the word.
export function usePathSearch(
  projectId: string,
  keepAt: boolean,
  box: RefObject<HTMLTextAreaElement | null>,
  onInsert: (text: string) => void,
): PathSearch {
  const [mention, setMention] = useState<Mention | null>(null)
  const [found, setFound] = useState<{ query: string; paths: string[]; error: string | null } | null>(null)
  const [selected, setSelected] = useState(0)
  // the @ that Escape closed, which stays closed until the caret leaves it.
  const dismissed = useRef<number | null>(null)
  const caretAfter = useRef<number | null>(null)
  // the word last tracked, so the pick resets only when it changes.
  const last = useRef<{ start: number; query: string } | null>(null)

  const track = useCallback(() => {
    const el = box.current
    if (!el || el.selectionStart !== el.selectionEnd) {
      setMention(null)
      return
    }
    const at = mentionAt(el.value, el.selectionStart)
    if (at === null || at.start !== dismissed.current) {
      dismissed.current = null
    }
    if (at === null || at.start === dismissed.current) {
      setMention(null)
      return
    }
    if (last.current?.query !== at.query || last.current.start !== at.start) {
      setSelected(0)
    }
    last.current = at
    const point = caretPoint(el, at.start)
    setMention({ ...at, left: el.offsetLeft + point.left, top: el.offsetTop + Math.max(point.top, 0) })
  }, [box])

  const query = mention?.query ?? null
  useEffect(() => {
    if (query === null) {
      return
    }
    let current = true
    const timer = window.setTimeout(() => {
      getPaths(projectId, query).then(
        (paths) => current && setFound({ query, paths, error: null }),
        (err: unknown) => current && setFound({ query, paths: [], error: err instanceof Error ? err.message : String(err) }),
      )
    }, settle)
    return () => {
      current = false
      window.clearTimeout(timer)
    }
  }, [projectId, query])

  // the caret goes after an inserted path once the box shows it.
  useLayoutEffect(() => {
    if (caretAfter.current !== null && box.current) {
      box.current.setSelectionRange(caretAfter.current, caretAfter.current)
      caretAfter.current = null
    }
  })

  const close = useCallback(() => setMention(null), [])
  const paths = mention && found ? found.paths : []
  const pick = (path: string) => {
    const el = box.current
    if (!el || !mention) {
      return
    }
    const next = insertPath(el.value, mention.start, path, keepAt)
    caretAfter.current = next.caret
    setMention(null)
    onInsert(next.text)
  }
  const onKeyDown = (event: KeyboardEvent<HTMLTextAreaElement>): boolean => {
    if (!mention || event.ctrlKey || event.metaKey || event.altKey) {
      return false
    }
    if (event.key === 'Escape') {
      dismissed.current = mention.start
      setMention(null)
      return true
    }
    if (paths.length === 0) {
      return false
    }
    if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
      const step = event.key === 'ArrowDown' ? 1 : -1
      setSelected((selected + step + paths.length) % paths.length)
      return true
    }
    if (event.key === 'Enter' && !event.shiftKey) {
      pick(paths[Math.min(selected, paths.length - 1)])
      return true
    }
    return false
  }
  return {
    mention,
    ready: found !== null,
    paths,
    error: mention && found ? found.error : null,
    selected: Math.min(selected, Math.max(paths.length - 1, 0)),
    track,
    close,
    onKeyDown,
    pick,
    hover: setSelected,
  }
}
