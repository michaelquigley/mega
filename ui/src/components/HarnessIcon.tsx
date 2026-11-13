import { harnesses } from '../harnesses'

// every mark in harness-icons, by path, as the URL the build gives it. they
// are embedded in mega's own build; nothing is fetched from a vendor.
const marks = import.meta.glob<string>('../harness-icons/*.svg', { eager: true, query: '?url', import: 'default' })

// HarnessIcon says which harness a conversation runs on: the harness's own
// mark, named in the harness table, with its label as its name and title, or
// the label in words where the table names no mark. a single-colour mark is
// drawn as a mask in the text colour, so a black file reads in the dark theme
// too; the file itself is used as it is.
export function HarnessIcon({ harness }: { harness: string }) {
  const entry = harnesses.find((h) => h.value === harness)
  const label = entry?.label ?? harness
  const url = entry?.icon ? marks[`../harness-icons/${entry.icon}`] : undefined
  if (!url) {
    return (
      <span className="harness-name" title={label}>
        {label}
      </span>
    )
  }
  if (entry?.iconMono) {
    const size = `${(entry.iconScale ?? 1) * 100}%`
    return <span className="harness-icon mono" role="img" aria-label={label} title={label} style={{ maskImage: `url(${JSON.stringify(url)})`, maskSize: size }} />
  }
  return <img className="harness-icon" src={url} alt={label} title={label} />
}
