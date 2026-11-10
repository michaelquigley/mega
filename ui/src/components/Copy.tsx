import { useRef, useState } from 'react'
import { plainText } from '../plain'
import { IconButton, type IconName } from './Icon'

// CopyButton copies text to the clipboard, and confirms on itself: its icon
// turns to a check for a moment, or to an error mark if the copy failed. no
// toast. getText reads the text at the moment of the copy.
export function CopyButton({
  text,
  getText,
  icon = 'content_copy',
  label = 'Copy',
  className = 'copy',
}: {
  text?: string
  getText?: () => string
  icon?: IconName
  label?: string
  className?: string
}) {
  const [state, setState] = useState<'idle' | 'copied' | 'failed'>('idle')
  const copy = async () => {
    try {
      await navigator.clipboard.writeText(getText ? getText() : (text ?? ''))
      setState('copied')
    } catch {
      setState('failed')
    }
    window.setTimeout(() => setState('idle'), 1500)
  }
  return (
    <IconButton
      className={className}
      icon={state === 'copied' ? 'check' : state === 'failed' ? 'error_outline' : icon}
      label={state === 'copied' ? 'Copied' : state === 'failed' ? 'Copy failed' : label}
      onClick={() => void copy()}
    />
  )
}

// BlockCopy is a message block's two copy controls, at its top right: the
// recorded source, exactly, as markdown; and what is rendered, as plain text.
export function BlockCopy({ source }: { source: string }) {
  const ref = useRef<HTMLDivElement>(null)
  const rendered = () => {
    const markdown = ref.current?.closest('.message')?.querySelector('.markdown')
    return markdown ? plainText(markdown) : source
  }
  return (
    <div className="block-copy" ref={ref}>
      <CopyButton text={source} icon="code" label="Copy as markdown" />
      <CopyButton getText={rendered} icon="notes" label="Copy as plain text" />
    </div>
  )
}
