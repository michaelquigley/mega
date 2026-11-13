import { useCallback, useLayoutEffect, useRef, useState, type RefObject } from 'react'

// how far from the bottom still counts as at the bottom, for rounding and for
// the last line settling.
const slack = 32

// the main column is what scrolls; a conversation is one element inside it.
function scrollerOf(content: HTMLElement | null): HTMLElement | null {
  return content?.closest<HTMLElement>('.main') ?? null
}

function atBottom(scroller: HTMLElement): boolean {
  return scroller.scrollHeight - scroller.scrollTop - scroller.clientHeight <= slack
}

// moveToLatest scrolls to the newest activity and reports whether the view
// ended at the bottom. pending request cards are docked in the input panel,
// which stays in view, so the bottom of the history is all there is to reach.
function moveToLatest(scroller: HTMLElement): boolean {
  scroller.scrollTop = scroller.scrollHeight
  return atBottom(scroller)
}

// useLatest keeps an open conversation at its newest activity. content is the
// conversation's element inside the scrolling main column, and dock the input
// panel pinned to its foot. the view starts at the top, so nothing carries
// over from whatever was shown before; once the history has loaded it moves to
// the newest event; pending request cards are docked in the input panel,
// always in view. after that, new activity (a change in activity) is followed
// only while the view is at the bottom: a reader who scrolled up stays put,
// and newer is true until they return, by scrolling or through toLatest.
//
// the dock changes height with no new activity: the activity line appears as
// a turn starts, a request docks or leaves, the reply box opens, closes or
// grows. while the view follows the bottom, any change in the dock's size, or
// in the scrolling column's, keeps it at the bottom; a reader who scrolled up
// is not moved. following is the one reckoning of whether the view follows:
// it is set by scrolling alone, so it still holds the state from before the
// dock changed, which fires no scroll event, and a tall docked request cannot
// read as having scrolled away.
export function useLatest(content: RefObject<HTMLElement | null>, dock: RefObject<HTMLElement | null>, loaded: boolean, activity: string) {
  const following = useRef(true)
  const seen = useRef<string | null>(null)
  const [newer, setNewer] = useState(false)

  useLayoutEffect(() => {
    const scroller = scrollerOf(content.current)
    if (!scroller) {
      return
    }
    scroller.scrollTop = 0
    const onScroll = () => {
      following.current = atBottom(scroller)
      if (following.current) {
        setNewer(false)
      }
    }
    scroller.addEventListener('scroll', onScroll, { passive: true })
    return () => scroller.removeEventListener('scroll', onScroll)
  }, [content])

  useLayoutEffect(() => {
    const scroller = scrollerOf(content.current)
    if (!content.current || !scroller || !loaded || seen.current === activity) {
      return
    }
    const first = seen.current === null
    seen.current = activity
    if (first) {
      following.current = moveToLatest(scroller)
    } else if (following.current) {
      scroller.scrollTop = scroller.scrollHeight
    } else {
      setNewer(true)
    }
  }, [content, loaded, activity])

  // a ResizeObserver's callbacks run after layout and before paint, so the
  // re-pin lands in the same frame as the change: no line slides under the
  // dock and back. its first callback, as observing begins, pins nothing
  // until the history has loaded and been moved to.
  useLayoutEffect(() => {
    const scroller = scrollerOf(content.current)
    const panel = dock.current
    if (!scroller || !panel) {
      return
    }
    const observer = new ResizeObserver(() => {
      if (seen.current !== null && following.current) {
        scroller.scrollTop = scroller.scrollHeight
      }
    })
    observer.observe(panel)
    observer.observe(scroller)
    return () => observer.disconnect()
  }, [content, dock])

  const toLatest = () => {
    const scroller = scrollerOf(content.current)
    if (content.current && scroller) {
      following.current = moveToLatest(scroller)
    }
    setNewer(false)
  }

  // isFollowing tells the input panel whether the view follows the bottom, so
  // it keeps the reader's place only when it does not. it is the same
  // function for the conversation's life.
  const isFollowing = useCallback(() => following.current, [])

  return { newer, toLatest, isFollowing }
}
