import {useEffect, useRef, useState} from 'react'
import {Terminal} from '@xterm/xterm'
import {FitAddon} from '@xterm/addon-fit'
import {WebLinksAddon} from '@xterm/addon-web-links'
import '@xterm/xterm/css/xterm.css'
import * as api from '../lib/api'
import * as aiApi from '../lib/ai'
import * as transfer from '../lib/transfer'
import {subscribe} from '../lib/sessionBus'
import {useAppStore} from '../state/useAppStore'
import type {Tab, AIMessage} from '../state/useAppStore'
import type {PrepareUploadResult} from '../lib/transfer'

// Terminal palette tuned to the spec §9 dark theme.
const THEME = {
  background: '#0b0f14',
  foreground: '#e5e7eb',
  cursor: '#5eead4',
  selectionBackground: '#1a2230',
  black: '#0b0f14',
  red: '#f87171',
  green: '#5eead4',
  yellow: '#fcd34d',
  blue: '#60a5fa',
  magenta: '#c084fc',
  cyan: '#2dd4bf',
  white: '#e5e7eb',
  brightBlack: '#94a3b8',
  brightRed: '#fca5a5',
  brightGreen: '#99f6e4',
  brightYellow: '#fde68a',
  brightBlue: '#93c5fd',
  brightMagenta: '#d8b4fe',
  brightCyan: '#5eead4',
  brightWhite: '#ffffff',
}

// TerminalPane mounts one xterm.js instance bound to a tab's session. It is
// kept mounted but hidden when not the active tab (so scrollback survives tab
// switches). Output arrives via sessionBus; input/resize go back over IPC.
export default function TerminalPane({tab, visible, onUpload}: {tab: Tab; visible: boolean; onUpload?: (sessionId: string, hostLabel: string, res: PrepareUploadResult) => void}) {
  const containerRef = useRef<HTMLDivElement>(null)
  const termRef = useRef<Terminal | null>(null)
  const [ctxMenu, setCtxMenu] = useState<{x: number; y: number} | null>(null)
  const ctxRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!ctxMenu) return
    const handler = (e: MouseEvent) => {
      if (ctxRef.current && !ctxRef.current.contains(e.target as Node)) {
        setCtxMenu(null)
      }
    }
    document.addEventListener('mousedown', handler)
    return () => document.removeEventListener('mousedown', handler)
  }, [ctxMenu])
  const fitRef = useRef<FitAddon | null>(null)
  const updateTab = useAppStore((s) => s.updateTab)
  const decoder = useRef(new TextDecoder())

  // Mount xterm once the session is live.
  useEffect(() => {
    if (!tab.sessionId || !containerRef.current || termRef.current) return

    const term = new Terminal({
      fontFamily: '"JetBrains Mono", ui-monospace, monospace',
      fontSize: tab.fontSize ?? 13,
      theme: THEME,
      cursorBlink: true,
      scrollback: 5000,
    })
    const fit = new FitAddon()
    term.loadAddon(fit)
    term.loadAddon(new WebLinksAddon())
    term.open(containerRef.current)
    fit.fit()
    term.focus()

    termRef.current = term
    fitRef.current = fit

    // Keystrokes -> remote stdin.
    const dataDisp = term.onData((data) => {
      void api.writeToSession(tab.sessionId, data)
    })
    // Local resize -> remote PTY + tell Go the new size.
    const resizeDisp = term.onResize(({cols, rows}) => {
      void api.resizeSession(tab.sessionId, cols, rows)
    })

    // Output + close from Go.
    const unsub = subscribe(tab.sessionId, {
      onOutput: (bytes) => {
        const text = decoder.current.decode(bytes, {stream: true})
        // Handle terminal queries that remote apps (vim, etc.) send.
        // xterm.js handles most internally and responds via triggerDataEvent
        // → onData → writeToSession. We intercept here as a safety net so
        // no query goes unanswered, which would cause the remote app to
        // hang waiting for a response.
        const queryRe = /\x1b\[(\??)6n|\x1b\[[>?]?c|\x1b\[18t/g
        let match: RegExpExecArray | null
        let last = 0
        let out = ''
        const deferred: (() => void)[] = []
        while ((match = queryRe.exec(text)) !== null) {
          const full = match[0]
          out += text.slice(last, match.index)
          last = queryRe.lastIndex
          if (full.endsWith('6n')) {
            // DSR / CPR: respond with current cursor position.
            const hasQ = match[1] === '?'
            deferred.push(() => {
              const row = term.buffer.active.cursorY + 1
              const col = term.buffer.active.cursorX + 1
              void api.writeToSession(tab.sessionId, `\x1b[${hasQ ? '?' : ''}${row};${col}R`)
            })
          } else if (full.endsWith('c')) {
            // Device Attributes (primary: \x1b[c, secondary: \x1b[>c, ?).
            if (full.startsWith('\x1b[>')) {
              void api.writeToSession(tab.sessionId, '\x1b[>0;276;0c')
            } else {
              void api.writeToSession(tab.sessionId, '\x1b[?1;2c')
            }
          } else if (full.endsWith('t')) {
            // Report Window Size in Characters.
            void api.writeToSession(tab.sessionId, `\x1b[8;${term.rows};${term.cols}t`)
          }
        }
        out += text.slice(last)
        term.write(out)
        // After term.write() the buffer state is updated synchronously.
        for (const fn of deferred) fn()

        // Error detection: check for common error patterns and suggest fixes.
        // Gated on aiAutoExplainErrors — this path ships terminal output to a
        // third-party provider automatically, so it is opt-in (off by default).
        if (useAppStore.getState().aiConfigured && useAppStore.getState().aiAutoExplainErrors && detectError(text)) {
          clearTimeout(errorTimerRef.current)
          errorTimerRef.current = setTimeout(() => {
            const state = useAppStore.getState()
            if (!state.aiConfigured || !state.aiAutoExplainErrors) return
            aiApi.explainError(tab.sessionId, text).then((result) => {
              const msg: AIMessage = {
                id: `ai-sug-${Date.now()}`,
                role: 'suggestion',
                content: result.explanation,
                command: result.fixCommand || undefined,
                timestamp: Date.now(),
              }
              state.addAIMessage(msg)
            }).catch(() => {
              // Silent — errors are logged in ai.ts
            })
          }, 2000)
        }
      },
      onClosed: (reason) => updateTab(tab.tabId, {status: 'disconnected', disconnectReason: reason}),
    })

    // Push the true terminal size to the backend now that it's measured.
    void api.resizeSession(tab.sessionId, term.cols, term.rows)

    return () => {
      clearTimeout(errorTimerRef.current)
      dataDisp.dispose()
      resizeDisp.dispose()
      unsub()
      term.dispose()
      termRef.current = null
      fitRef.current = null
    }
  }, [tab.sessionId, tab.tabId, updateTab])

  // Refit when the terminal container changes size (window resize, drawer
  // open/close, etc.). Debounced to avoid rapid resize events triggering SSH
  // PTY resizes (which send SIGWINCH to vim — the DSR/DA intercept below
  // handles any terminal queries vim sends in response).
  const resizeTimerRef = useRef<ReturnType<typeof setTimeout>>()
  const errorTimerRef = useRef<ReturnType<typeof setTimeout>>()
  useEffect(() => {
    if (!visible || !containerRef.current) return
    const refit = () => {
      fitRef.current?.fit()
      termRef.current?.focus()
    }
    refit()
    const debouncedRefit = () => {
      clearTimeout(resizeTimerRef.current)
      resizeTimerRef.current = setTimeout(refit, 200)
    }
    window.addEventListener('resize', debouncedRefit)
    const ro = new ResizeObserver(debouncedRefit)
    ro.observe(containerRef.current)
    return () => {
      clearTimeout(resizeTimerRef.current)
      window.removeEventListener('resize', debouncedRefit)
      ro.disconnect()
    }
  }, [visible])

  // Update font size on the fly when tab.fontSize changes.
  useEffect(() => {
    if (termRef.current) {
      termRef.current.options.fontSize = tab.fontSize ?? 13
    }
  }, [tab.fontSize])

  function ctxItem(label: string, onClick: () => void) {
    return (
      <button
        className="flex w-full items-center px-3 py-1.5 text-left text-xs text-muted hover:bg-surface-2 hover:text-text"
        onMouseDown={(e) => e.preventDefault()}
        onClick={() => {
          setCtxMenu(null)
          onClick()
        }}
      >
        {label}
      </button>
    )
  }

  return (
    <div className={`absolute inset-0 ${visible ? 'block' : 'hidden'}`}>
      <div
        ref={containerRef}
        className="h-full w-full p-2"
        onContextMenu={(e) => {
          e.preventDefault()
          if (tab.status === 'connected') setCtxMenu({x: e.clientX, y: e.clientY})
        }}
      />
      {tab.status === 'disconnected' && (
        <DisconnectedOverlay reason={tab.disconnectReason} />
      )}
      {tab.status === 'connecting' && (
        <div className="absolute inset-0 flex items-center justify-center text-sm text-muted">
          Connecting…
        </div>
      )}
      {ctxMenu && (
        <div
          ref={ctxRef}
          className="fixed z-50 w-40 overflow-hidden rounded-md border border-border bg-surface shadow-lg"
          style={{left: ctxMenu.x, top: ctxMenu.y}}
        >
          {ctxItem('Copy', () => {
            const t = termRef.current
            if (t?.hasSelection()) {
              navigator.clipboard.writeText(t.getSelection()).catch(() => {})
              t.clearSelection()
            }
          })}
          {ctxItem('Paste', () => {
            navigator.clipboard.readText().then((text) => {
              void api.writeToSession(tab.sessionId, text.replace(/\r?\n/g, '\r'))
            })
          })}
          <div className="my-1 border-t border-border" />
          {ctxItem('Upload Files…', () => {
            void transfer.pickFilesForUpload(tab.sessionId).then((res) => {
              if (onUpload) onUpload(tab.sessionId, tab.title, res)
            })
          })}
          {ctxItem('Upload Folder…', () => {
            void transfer.pickFolderForUpload(tab.sessionId).then((res) => {
              if (onUpload) onUpload(tab.sessionId, tab.title, res)
            })
          })}
        </div>
      )}
    </div>
  )
}

// detectError returns true if the text contains a pattern that likely indicates
// a command failure. The regex is deliberately narrow to avoid false positives.
function detectError(text: string): boolean {
  // Strip ANSI escape sequences before checking.
  const clean = text.replace(/\x1b\[[0-9;]*[a-zA-Z]/g, '').trim()
  if (!clean) return false
  return /(?:^|\s)(Error|ERROR|error):/.test(clean) ||
    /(?:^|\s)(fatal|FAILED|Killed)/.test(clean) ||
    /\b(permission denied|No such file|command not found)\b/i.test(clean) ||
    /\b(Connection refused|timed out|cannot)\b/i.test(clean)
}

function DisconnectedOverlay({reason}: {reason?: string}) {
  return (
    <div className="absolute inset-0 flex flex-col items-center justify-center gap-1 bg-bg/70">
      <div className="text-sm font-semibold text-danger">Disconnected</div>
      {reason && <div className="text-xs text-muted">{reason}</div>}
      <div className="mt-1 text-xs text-muted/70">Close this tab to dismiss.</div>
    </div>
  )
}
