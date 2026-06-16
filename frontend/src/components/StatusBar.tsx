import {useEffect, useState} from 'react'
import {useAppStore} from '../state/useAppStore'
import AboutModal from './AboutModal'

// StatusBar is the thin bottom bar: active session count with longest uptime,
// active forward count, a snippets toggle, and the lock button.
export default function StatusBar({onLock}: {onLock: () => void}) {
  const tabs = useAppStore((s) => s.tabs)
  const activeForwards = useAppStore((s) => s.activeForwards)
  const sessionHealth = useAppStore((s) => s.sessionHealth)
  const refreshActiveForwards = useAppStore((s) => s.refreshActiveForwards)
  const refreshSessionHealth = useAppStore((s) => s.refreshSessionHealth)
  const toggleSnippets = useAppStore((s) => s.toggleSnippets)
  const toggleAI = useAppStore((s) => s.toggleAI)
  const liveCount = tabs.filter((t) => t.status === 'connected').length
  const [showAbout, setShowAbout] = useState(false)

  useEffect(() => {
    void refreshActiveForwards()
    void refreshSessionHealth()
    const id = setInterval(() => {
      void refreshActiveForwards()
      void refreshSessionHealth()
    }, 5000)
    return () => clearInterval(id)
  }, [refreshActiveForwards, refreshSessionHealth])

  // Find the longest uptime among live sessions.
  let oldest = ''
  if (liveCount > 0 && sessionHealth.uptimes?.length) {
    let maxSecs = 0
    for (const si of sessionHealth.uptimes) {
      // uptime is like "5m3s" — approximate comparison by string length + value
      const secs = parseUptime(si.uptime)
      if (secs > maxSecs) {
        maxSecs = secs
        oldest = si.uptime
      }
    }
  }

  return (
    <>
      <div className="flex h-7 items-center justify-end gap-4 border-t border-border bg-surface px-3 text-xs text-muted">
        {activeForwards.length > 0 && (
          <span>
            {activeForwards.length} forward{activeForwards.length === 1 ? '' : 's'}
          </span>
        )}
        <span title={oldest ? `Longest session: ${oldest}` : undefined}>
          {liveCount} session{liveCount === 1 ? '' : 's'}
        </span>
        {oldest && <span className="text-muted/60">↑ {oldest}</span>}
        <button onClick={toggleAI} className="hover:text-text" title="Toggle AI assistant">
          AI
        </button>
        <button onClick={toggleSnippets} className="hover:text-text" title="Toggle snippets">
          Snippets
        </button>
        <button onClick={() => setShowAbout(true)} className="hover:text-text" title="About">
          About
        </button>
        <button onClick={onLock} className="hover:text-accent" title="Lock vault">
          Lock
        </button>
      </div>
      {showAbout && <AboutModal onClose={() => setShowAbout(false)} />}
    </>
  )
}

function parseUptime(s: string): number {
  // Parse Go duration strings like "5m3s", "1h2m5s", "30s" into total seconds.
  let total = 0
  let acc = ''
  for (const ch of s) {
    if (ch >= '0' && ch <= '9') {
      acc += ch
    } else {
      const n = parseInt(acc, 10) || 0
      if (ch === 'h') total += n * 3600
      else if (ch === 'm') total += n * 60
      else if (ch === 's') total += n
      acc = ''
    }
  }
  return total
}
