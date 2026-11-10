import { useEffect, useLayoutEffect, useRef, type ReactNode } from 'react'
import { IconButton } from './Icon'

// Dialog is a modal with a title and a close control. it closes natively, by
// the control, by Escape, or by being unmounted, so the browser hands focus
// back to whatever opened it; then onClose runs.
export function Dialog({ title, onClose, children }: { title: string; onClose: () => void; children: ReactNode }) {
  const ref = useRef<HTMLDialogElement>(null)
  useEffect(() => {
    ref.current?.showModal()
  }, [])
  // closing before the element leaves the page returns focus to the opener;
  // removing an open dialog would drop focus on the page body instead.
  useLayoutEffect(() => {
    const dialog = ref.current
    return () => {
      if (dialog?.open) {
        dialog.close()
      }
    }
  }, [])
  return (
    <dialog ref={ref} onClose={onClose}>
      <div className="dialog-body">
        <div className="dialog-head">
          <h2>{title}</h2>
          <IconButton icon="close" label="Close" onClick={() => ref.current?.close()} />
        </div>
        {children}
      </div>
    </dialog>
  )
}
