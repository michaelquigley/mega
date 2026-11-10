// plainText is a rendered message as plain text, without markdown syntax:
// headings and emphasis as their words, each list item on its own line with
// no marker, code as its text, a link as its text followed by its address in
// parentheses, a table as tab-separated rows, and a soft line break as the
// space it renders as. it reads the rendered element, so it copies what the
// reader sees; controls inside it (a code block's header and toggle) are
// left out.
export function plainText(root: Element): string {
  return text(root)
    .replace(/[ \t]*\n[ \t]*/g, '\n')
    .replace(/\n{3,}/g, '\n\n')
    .trim()
}

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
    case 'OL': {
      const items = [...node.children].filter((child) => child.tagName === 'LI').map((li) => text(li).trim())
      return `\n\n${items.join('\n')}\n\n`
    }
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
