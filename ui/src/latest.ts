import { useLayoutEffect, useRef, useState, type RefObject } from 'react'

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
// ended at the bottom. the controls and the reply box sit below the request
// cards, so a tall card can end above the top; the card is what needs the
// operator, so it wins.
function moveToLatest(content: HTMLElement, scroller: HTMLElement): boolean {
  scroller.scrollTop = scroller.scrollHeight
  const card = content.querySelector<HTMLElement>('.request-card')
  if (card && card.getBoundingClientRect().top < scroller.getBoundingClientRect().top) {
    card.scrollIntoView({ block: 'start' })
  }
  return atBottom(scroller)
}

// useLatest keeps an open conversation at its newest activity. content is the
// conversation's element inside the scrolling main column. the view starts at
// the top, so nothing carries over from whatever was shown before; once the
// history has loaded it moves to the newest event, with a pending request card
// in view. after that, new activity (a change in activity) is followed only
// while the view is at the bottom: a reader who scrolled up stays put, and
// newer is true until they return, by scrolling or through toLatest.
export function useLatest(content: RefObject<HTMLElement | null>, loaded: boolean, activity: string) {
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
      following.current = moveToLatest(content.current, scroller)
    } else if (following.current) {
      scroller.scrollTop = scroller.scrollHeight
    } else {
      setNewer(true)
    }
  }, [content, loaded, activity])

  const toLatest = () => {
    const scroller = scrollerOf(content.current)
    if (content.current && scroller) {
      following.current = moveToLatest(content.current, scroller)
    }
    setNewer(false)
  }

  return { newer, toLatest }
}
