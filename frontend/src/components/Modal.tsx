import {ReactNode, useEffect, useRef, useState} from 'react'

// Modal is a centered dialog over a dimming backdrop. Escape and backdrop
// clicks close it. Shared by all the edit/trust dialogs.
export default function Modal({
  title,
  onClose,
  children,
  width = 420,
}: {
  title: string
  onClose: () => void
  children: ReactNode
  width?: number
}) {
  const onCloseRef = useRef(onClose)
  onCloseRef.current = onClose

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => e.key === 'Escape' && onCloseRef.current()
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [])

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/50"
      onMouseDown={onClose}
    >
      <div
        className="rounded-xl border border-border bg-surface p-6 shadow-2xl"
        style={{width}}
        onMouseDown={(e) => e.stopPropagation()}
      >
        <h2 className="mb-4 text-base font-semibold text-text">{title}</h2>
        {children}
      </div>
    </div>
  )
}

// Field is a labeled input row used across the modals.
export function Field({
  label,
  children,
}: {
  label: string
  children: ReactNode
}) {
  return (
    <label className="mb-3 block">
      <span className="mb-1 block text-xs text-muted">{label}</span>
      {children}
    </label>
  )
}

export const inputClass =
  'w-full rounded-md border border-border bg-surface-2 px-3 py-2 text-sm text-text outline-none focus:border-accent'

// CustomSelect is a fully styled dropdown replacement for native <select>.
// Native <option> elements inherit color from the parent <select> but WebKitGTK
// renders the dropdown popup as an OS widget that ignores CSS, making options
// unreadable. This component gives full control over styling.
export function CustomSelect<T extends string>({
  value,
  onChange,
  options,
}: {
  value: T
  onChange: (v: T) => void
  options: {value: T; label: string}[]
}) {
  const [open, setOpen] = useState(false)
  const ref = useRef<HTMLDivElement>(null)

  // Close on click outside.
  useEffect(() => {
    if (!open) return
    const handler = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false)
    }
    document.addEventListener('mousedown', handler)
    return () => document.removeEventListener('mousedown', handler)
  }, [open])

  const selected = options.find((o) => o.value === value)

  return (
    <div ref={ref} className="relative">
      <button
        type="button"
        onClick={() => setOpen(!open)}
        className={`${inputClass} flex items-center justify-between text-left`}
      >
        <span>{selected?.label ?? value}</span>
        <svg
          className={`h-4 w-4 text-muted transition-transform ${open ? 'rotate-180' : ''}`}
          fill="none"
          viewBox="0 0 24 24"
          stroke="currentColor"
        >
          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 9l-7 7-7-7" />
        </svg>
      </button>
      {open && (
        <div className="absolute left-0 right-0 top-full z-10 mt-1 overflow-hidden rounded-md border border-border bg-surface shadow-xl">
          {options.map((o) => (
            <button
              key={o.value}
              type="button"
              onMouseDown={() => {
                onChange(o.value)
                setOpen(false)
              }}
              className={`block w-full px-3 py-2 text-left text-sm transition-colors ${
                o.value === value
                  ? 'bg-accent/20 text-accent'
                  : 'text-text hover:bg-surface-2'
              }`}
            >
              {o.label}
            </button>
          ))}
        </div>
      )}
    </div>
  )
}
