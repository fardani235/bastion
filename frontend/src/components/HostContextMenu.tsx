import {useEffect, useRef, useState} from 'react'
import * as api from '../lib/api'
import type {HostDTO} from '../lib/api'
import {useAppStore} from '../state/useAppStore'

export default function HostContextMenu({
  host,
  x,
  y,
  onClose,
}: {
  host: HostDTO
  x: number
  y: number
  onClose: () => void
}) {
  const refreshHosts = useAppStore((s) => s.refreshHosts)
  const updateTab = useAppStore((s) => s.updateTab)
  const tabs = useAppStore((s) => s.tabs)
  const ref = useRef<HTMLDivElement>(null)
  const [fontSize, setFontSize] = useState(host.fontSize ?? 13)

  useEffect(() => {
    const handler = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) {
        onClose()
      }
    }
    document.addEventListener('mousedown', handler, true)
    return () => document.removeEventListener('mousedown', handler, true)
  }, [onClose])

  function syncTabs(next: number) {
    for (const t of tabs) {
      if (t.hostId === host.id && t.fontSize !== next) {
        updateTab(t.tabId, {fontSize: next})
      }
    }
  }

  async function changeSize(delta: number) {
    const next = Math.min(72, Math.max(8, fontSize + delta))
    setFontSize(next)
    syncTabs(next)
    try {
      await api.setHostFontSize(host.id, next)
      await refreshHosts()
    } catch (e) {
      console.error('setHostFontSize:', e)
    }
  }

  async function setSize(val: number) {
    const clamped = Math.min(72, Math.max(8, val))
    setFontSize(clamped)
    syncTabs(clamped)
    try {
      await api.setHostFontSize(host.id, clamped)
      await refreshHosts()
    } catch (e) {
      console.error('setHostFontSize:', e)
    }
  }

  return (
    <div
      ref={ref}
      className="fixed z-50 rounded-lg border border-border bg-surface shadow-xl"
      style={{left: x, top: y}}
    >
      <div className="px-3 py-2 text-xs font-semibold uppercase tracking-wide text-muted">
        {host.label}
      </div>
      <div className="flex items-center gap-2 px-3 pb-2">
        <span className="text-xs text-muted">Font size</span>
        <button
          onClick={() => changeSize(-1)}
          className="flex h-6 w-6 items-center justify-center rounded border border-border text-sm hover:bg-surface-2"
        >
          −
        </button>
        <input
          type="number"
          min={8}
          max={72}
          value={fontSize}
          onChange={(e) => {
            const v = parseInt(e.target.value, 10)
            if (!isNaN(v)) setSize(v)
          }}
          className="w-14 rounded border border-border bg-surface-2 px-1 py-0.5 text-center text-sm text-text outline-none"
        />
        <button
          onClick={() => changeSize(1)}
          className="flex h-6 w-6 items-center justify-center rounded border border-border text-sm hover:bg-surface-2"
        >
          +
        </button>
      </div>
    </div>
  )
}
