// plainText is a rendered message as plain text, without markdown syntax:
// headings and emphasis as their words, code as its text, a link as its text
// followed by its address in parentheses, a table as tab-separated rows, and a
// soft line break as the space it renders as. lists keep the markers plain
// text writes them with: each item on its own line, "1." counting from the
// list's start, "-" for an unordered item, "[x]" or "[ ]" for a task, and two
// spaces of indent per level of nesting. it reads the rendered element, so it
// copies what the reader sees; controls inside it (a code block's header and
// toggle) are left out.
export function plainText(root: Element): string {
  return text(root)
    .replace(/[ \t]*\n[ \t]*/g, '\n')
    .replace(/\n{3,}/g, '\n\n')
    .trim()
    .replaceAll(pad, ' ')
}

// pad stands for one space of a list's indent until the whitespace between
// blocks is tidied, which would otherwise take it for stray space.
const pad = '\u0001'

const blocks = new Set(['P', 'H1', 'H2', 'H3', 'H4', 'H5', 'H6', 'BLOCKQUOTE'])

function text(node: Node): string {
  if (node.nodeType === Node.TEXT_NODE) {
    // outside code, a newline in the source renders as a space.
    return (node.textContent ?? '').replace(/\n/g, ' ')
  }
  if (!(node instanceof Element)) {
    return ''
  }
  if (node.matches('button, input, .code-head')) {
    return ''
  }
  const inner = () => [...node.childNodes].map(text).join('')
  switch (node.tagName) {
    case 'BR':
      return '\n'
    case 'HR':
      return '\n\n'
    case 'PRE':
      return `\n\n${(node.textContent ?? '').replace(/\n$/, '')}\n\n`
    case 'A': {
      const words = inner()
      const href = node.getAttribute('href')
      return href && href !== words.trim() ? `${words} (${href})` : words
    }
    case 'UL':
    case 'OL':
      return `\n\n${list(node, 0)}\n\n`
    case 'TABLE': {
      const rows = [...node.querySelectorAll('tr')].map((tr) => [...tr.children].map((cell) => text(cell).trim()).join('\t'))
      return `\n\n${rows.join('\n')}\n\n`
    }
  }
  if (blocks.has(node.tagName)) {
    return `\n\n${inner()}\n\n`
  }
  return inner()
}

// list writes a list's items, one per line, each with its marker and any task
// box, and its nested lists below the item, indented one level further.
function list(node: Element, depth: number): string {
  const indent = pad.repeat(2 * depth)
  const ordered = node.tagName === 'OL'
  const start = ordered ? (node as HTMLOListElement).start : 1
  const lines: string[] = []
  ;[...node.children]
    .filter((child) => child.tagName === 'LI')
    .forEach((li, i) => {
      const marker = ordered ? `${start + i}.` : '-'
      const box = li.querySelector(':scope > input[type="checkbox"], :scope > p > input[type="checkbox"]') as HTMLInputElement | null
      const task = box ? (box.checked ? ' [x]' : ' [ ]') : ''
      const own = [...li.childNodes]
        .filter((child) => !(child instanceof Element && (child.tagName === 'UL' || child.tagName === 'OL')))
        .map(text)
        .join('')
        .split(/\n{2,}/)
        .map((part) => part.trim())
        .filter(Boolean)
        .join(`\n${indent}${pad}${pad}`)
      lines.push(`${indent}${marker}${task} ${own}`.trimEnd())
      for (const nested of li.children) {
        if (nested.tagName === 'UL' || nested.tagName === 'OL') {
          lines.push(list(nested, depth + 1))
        }
      }
    })
  return lines.join('\n')
}

