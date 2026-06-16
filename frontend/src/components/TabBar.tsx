import {useRef, useState} from 'react'
import {useAppStore} from '../state/useAppStore'
import type {Tab} from '../state/useAppStore'

// Deterministic color palette for tab host labels. Each host always gets the
// same color derived from its title via a simple hash.
const TAB_COLORS = [
  '#3b82f6', // blue-500
  '#0ea5e9', // sky-500
  '#10b981', // emerald-500
  '#8b5cf6', // violet-500
  '#f43f5e', // rose-500
  '#f59e0b', // amber-500
  '#06b6d4', // cyan-500
  '#ec4899', // pink-500
  '#84cc16', // lime-500
  '#6366f1', // indigo-500
]

function tabColor(title: string): string {
  let h = 0
  for (let i = 0; i < title.length; i++) h = (h * 31 + title.charCodeAt(i)) | 0
  return TAB_COLORS[((h >>> 0) % TAB_COLORS.length)]
}

// TabBar shows open session tabs. Clicking focuses; the × closes the tab and
// the Go session (handled by removeTab in the store). Tabs can be reordered
// by dragging.
export default function TabBar() {
  const tabs = useAppStore((s) => s.tabs)
  const activeTabId = useAppStore((s) => s.activeTabId)
  const setActiveTab = useAppStore((s) => s.setActiveTab)
  const removeTab = useAppStore((s) => s.removeTab)
  const reorderTab = useAppStore((s) => s.reorderTab)

  const dragIndex = useRef<number | null>(null)
  const [overIdx, setOverIdx] = useState<number | null>(null)

  if (tabs.length === 0) return <div className="h-9 border-b border-border bg-surface" />

  function close(tab: Tab) {
    void removeTab(tab.tabId)
  }

  function dot(status: Tab['status']) {
    if (status === 'connected') return 'bg-accent-dim'
    if (status === 'connecting') return 'bg-yellow-400'
    return 'bg-danger'
  }

  function handleDragStart(e: React.DragEvent, idx: number) {
    e.dataTransfer.setData('text/plain', '')
    e.dataTransfer.effectAllowed = 'move'
    dragIndex.current = idx
  }

  function handleDragOver(e: React.DragEvent, idx: number) {
    e.preventDefault()
    e.dataTransfer.dropEffect = 'move'
    if (dragIndex.current === null || dragIndex.current === idx) {
      setOverIdx(null)
      return
    }
    setOverIdx(idx)
  }

  function handleDrop(e: React.DragEvent, idx: number) {
    e.preventDefault()
    if (dragIndex.current !== null && dragIndex.current !== idx) {
      reorderTab(dragIndex.current, idx)
    }
    dragIndex.current = null
    setOverIdx(null)
  }

  function handleDragLeave() {
    setOverIdx(null)
  }

  function handleDragEnd() {
    dragIndex.current = null
    setOverIdx(null)
  }

  return (
    <div className="flex h-9 items-stretch gap-px overflow-x-auto border-b border-border bg-surface">
      {tabs.map((tab, idx) => (
        <div
          key={tab.tabId}
          draggable
          onClick={() => setActiveTab(tab.tabId)}
          onDragStart={(e) => handleDragStart(e, idx)}
          onDragOver={(e) => handleDragOver(e, idx)}
          onDrop={(e) => handleDrop(e, idx)}
          onDragLeave={handleDragLeave}
          onDragEnd={handleDragEnd}
          className={`group flex min-w-[140px] max-w-[220px] cursor-pointer items-center gap-2 px-3 text-sm transition-colors border-l-[3px] ${
            tab.tabId === activeTabId ? 'bg-bg text-text' : 'bg-surface text-muted hover:bg-surface-2'
          }`}
          style={{cursor: 'grab', borderLeftColor: overIdx === idx ? '#3b82f6' : tabColor(tab.title)}}
        >
          <span className={`h-1.5 w-1.5 shrink-0 rounded-full ${dot(tab.status)}`} />
          <span className="min-w-0 flex-1 truncate">{tab.title}</span>
          <button
            onClick={(e) => {
              e.stopPropagation()
              void close(tab)
            }}
            className="shrink-0 text-muted opacity-0 hover:text-danger group-hover:opacity-100"
            title="Close tab"
          >
            ✕
          </button>
        </div>
      ))}
    </div>
  )
}
