import {useState} from 'react'
import {useAppStore} from '../state/useAppStore'
import * as api from '../lib/api'
import type {Snippet} from '../lib/api'
import SnippetEditModal from './SnippetEditModal'

// SnippetsDrawer slides in from the right. Clicking a snippet writes its body
// followed by a newline into the active session's stdin so the command
// executes immediately.
export default function SnippetsDrawer() {
  const open = useAppStore((s) => s.snippetsOpen)
  const toggle = useAppStore((s) => s.toggleSnippets)
  const snippets = useAppStore((s) => s.snippets)
  const refreshSnippets = useAppStore((s) => s.refreshSnippets)
  const tabs = useAppStore((s) => s.tabs)
  const activeTabId = useAppStore((s) => s.activeTabId)
  const [modal, setModal] = useState<{snippet: Snippet | null} | null>(null)

  if (!open) return null

  const activeTab = tabs.find((t) => t.tabId === activeTabId)

  async function paste(s: Snippet) {
    if (!activeTab?.sessionId || activeTab.status !== 'connected') {
      console.warn('paste: no active connected session', {sessionId: activeTab?.sessionId, status: activeTab?.status})
      return
    }
    try {
      console.log('paste: injecting snippet', {sessionId: activeTab.sessionId, label: s.label, bodyLength: s.body.length})
      await api.writeToSession(activeTab.sessionId, s.body + '\n')
    } catch (e) {
      console.error('paste: writeToSession failed', e)
    }
  }

  async function del(s: Snippet) {
    try {
      await api.deleteSnippet(s.id)
      await refreshSnippets()
    } catch (e) {
      console.error('deleteSnippet:', e)
    }
  }

  return (
    <div className="flex w-[300px] shrink-0 flex-col border-l border-border bg-surface">
      <div className="flex items-center justify-between px-3 py-3">
        <span className="text-sm font-semibold text-text">Snippets</span>
        <div className="flex gap-2 text-xs">
          <button onClick={() => setModal({snippet: null})} className="text-accent hover:text-accent-dim">
            + New
          </button>
          <button onClick={toggle} className="text-muted hover:text-text" title="Close">
            ✕
          </button>
        </div>
      </div>

      <div className="flex-1 overflow-y-auto px-2 pb-2">
        {snippets.length === 0 && (
          <div className="px-2 py-2 text-xs text-muted/60">No snippets yet.</div>
        )}
        {snippets.map((s) => (
          <div key={s.id} className="group mb-1 rounded-md px-2 py-1.5 hover:bg-surface-2">
            <div className="flex items-center gap-2">
              <button
                onClick={() => paste(s)}
                className="min-w-0 flex-1 truncate text-left text-sm text-text"
                title={activeTab ? 'Paste into active session' : 'No active session'}
              >
                {s.label}
              </button>
              <span className="hidden gap-1 group-hover:flex">
                <button onClick={() => setModal({snippet: s})} className="text-muted hover:text-text" title="Edit">
                  ✎
                </button>
                <button onClick={() => del(s)} className="text-muted hover:text-danger" title="Delete">
                  ✕
                </button>
              </span>
            </div>
            <div className="truncate font-mono text-xs text-muted">{s.body}</div>
          </div>
        ))}
      </div>

      {modal && <SnippetEditModal key={modal.snippet?.id ?? '__new__'} snippet={modal.snippet} onClose={() => setModal(null)} />}
    </div>
  )
}
