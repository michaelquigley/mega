import { memo, useState, type ReactNode } from 'react'
import ReactMarkdown, { type Components, type ExtraProps } from 'react-markdown'
import remarkGfm from 'remark-gfm'
import { CopyButton } from './Copy'
import { IconButton } from './Icon'

// Markdown renders message text: assistant replies, the streaming delta, and
// the operator's prompts, all through this one path so a reply looks the same
// while it arrives and after it lands. the text is untrusted model output, so
// raw html is dropped, links keep only http, https and mailto, and images
// become links: nothing in a message makes the browser load a resource.
// partial input is fine: an unclosed fence runs to the end as code.
export const Markdown = memo(function Markdown({ text }: { text: string }) {
  return (
    <div className="markdown">
      <ReactMarkdown remarkPlugins={plugins} skipHtml urlTransform={safeUrl} components={components}>
        {text}
      </ReactMarkdown>
    </div>
  )
})

const plugins = [remarkGfm]

// safeUrl keeps absolute http, https and mailto urls and drops everything
// else, relative and fragment urls included, which would only point back into
// mega.
function safeUrl(url: string): string {
  return /^(https?:|mailto:)/i.test(url.trim()) ? url : ''
}

// headings step down so an agent's # is a heading inside the message, below
// the page's own h1 and h2.
const components: Components = {
  h1: ({ children }) => <h3>{children}</h3>,
  h2: ({ children }) => <h4>{children}</h4>,
  h3: ({ children }) => <h5>{children}</h5>,
  h4: ({ children }) => <h6>{children}</h6>,
  h5: ({ children }) => <h6>{children}</h6>,
  h6: ({ children }) => <h6>{children}</h6>,
  a: ({ href, children }) => <Link href={href}>{children}</Link>,
  img: ({ src, alt }) => (
    <Link href={typeof src === 'string' ? src : undefined}>{alt ? `image: ${alt}` : 'image'}</Link>
  ),
  table: ({ children }) => (
    <div className="table-scroll">
      <table>{children}</table>
    </div>
  ),
  pre: CodeBlock,
}

function Link({ href, children }: { href?: string; children: ReactNode }) {
  if (!href) {
    return <span className="dead-link">{children}</span>
  }
  return (
    <a href={href} target="_blank" rel="noopener noreferrer">
      {children}
    </a>
  )
}

interface HastNode {
  type: string
  value?: string
  tagName?: string
  properties?: { className?: unknown }
  children?: HastNode[]
}

function textOf(node: HastNode): string {
  if (node.type === 'text') {
    return node.value ?? ''
  }
  return (node.children ?? []).map(textOf).join('')
}

// a fenced block longer than this many lines shows only that many until the
// reader asks for all of it.
const collapsedLines = 40

// CodeBlock is a fenced block: the language, if the fence named one, a copy
// control, and the code in a monospace block. no highlighting. a long block is
// collapsed to its first lines by height alone, so the whole block stays in
// the page, and copy always takes the whole source.
// expanded holds the code blocks the reader expanded, for the page's life, so
// a block expanded while its reply streams is still expanded once the reply
// lands and is rendered afresh. a block is known by its language and its first
// lines, which are final once it is long enough to collapse.
const expanded = new Set<string>()

function CodeBlock({ children, node }: { children?: ReactNode } & ExtraProps) {
  const code = (node?.children ?? []).find((child) => child.type === 'element' && child.tagName === 'code') as
    | HastNode
    | undefined
  const classes = code?.properties?.className
  const language = Array.isArray(classes)
    ? classes.map(String).find((name) => name.startsWith('language-'))?.slice('language-'.length)
    : undefined
  const source = code ? textOf(code).replace(/\n$/, '') : ''
  const lines = source.split('\n').length
  const long = lines > collapsedLines
  const key = `${language ?? ''}\n${source.split('\n').slice(0, collapsedLines).join('\n')}`
  const [open, setOpen] = useState(() => expanded.has(key))
  const toggle = () => {
    if (open) {
      expanded.delete(key)
    } else {
      expanded.add(key)
    }
    setOpen(!open)
  }
  return (
    <div className="code-block">
      <div className="code-head">
        <span className="code-language">{language ?? ''}</span>
        <CopyButton text={source} />
      </div>
      <pre className={long && !open ? 'collapsed' : undefined}>{children}</pre>
      {long && (
        <IconButton
          className="code-toggle"
          icon={open ? 'unfold_less' : 'unfold_more'}
          label={open ? `Show only the first ${collapsedLines} lines` : `Show all ${lines} lines`}
          onClick={toggle}
        />
      )}
    </div>
  )
}
