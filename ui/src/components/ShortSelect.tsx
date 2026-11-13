import type { ReactNode } from 'react'
import { Icon } from './Icon'

interface ShortSelectProps {
  // what the control chooses, as its accessible name and title.
  label: string
  value: string
  // the closed control's short value; the options carry the long ones.
  short: string
  disabled: boolean
  onChange: (value: string) => void
  children: ReactNode
}

// ShortSelect is a native select that shows a short value while closed and
// its options' full descriptions when open. the select lies, transparent,
// over the short value, so pointer, keyboard and assistive technology all
// reach the native control, and its list is the browser's own.
export function ShortSelect({ label, value, short, disabled, onChange, children }: ShortSelectProps) {
  return (
    <span className={`short-select${disabled ? ' disabled' : ''}`}>
      <span className="short-value" aria-hidden="true">
        {short}
      </span>
      <Icon name="expand_more" />
      <select aria-label={label} title={label} value={value} disabled={disabled} onChange={(event) => onChange(event.target.value)}>
        {children}
      </select>
    </span>
  )
}
