import {useCallback, useEffect, useRef, useState} from 'react'
import Modal, {inputClass} from './Modal'
import * as transfer from '../lib/transfer'
import type {RemoteEntry, DownloadProgress, DownloadFileResult} from '../lib/transfer'

interface FileStatus {
  name: string
  total: number
  bytes: number
  done: boolean
  ok: boolean
  error?: string
}

type Phase = 'browsing' | 'downloading' | 'done'

function humanSize(n: number): string {
  if (n < 1024) return `${n} B`
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`
  if (n < 1024 * 1024 * 1024) return `${(n / (1024 * 1024)).toFixed(1)} MB`
  return `${(n / (1024 * 1024 * 1024)).toFixed(1)} GB`
}

function formatTime(ms: number): string {
  const d = new Date(ms)
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`
}

function dirname(p: string): string {
  if (p === '/' || p === '~') return p
  const clean = p.replace(/\/+$/, '')
  const idx = clean.lastIndexOf('/')
  if (idx <= 0) return '/'
  return clean.slice(0, idx)
}

function joinPath(parent: string, child: string): string {
  if (parent === '~') return `~/${child}`
  if (parent.endsWith('/')) return `${parent}${child}`
  return `${parent}/${child}`
}

function FolderIcon() {
  return (
    <svg className="h-4 w-4 shrink-0 text-accent" fill="none" viewBox="0 0 24 24" stroke="currentColor">
      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M3 7v10a2 2 0 002 2h14a2 2 0 002-2V9a2 2 0 00-2-2h-6l-2-2H5a2 2 0 00-2 2z" />
    </svg>
  )
}

function FileIcon() {
  return (
    <svg className="h-4 w-4 shrink-0 text-muted" fill="none" viewBox="0 0 24 24" stroke="currentColor">
      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M7 21h10a2 2 0 002-2V9.414a1 1 0 00-.293-.707l-5.414-5.414A1 1 0 0012.586 3H7a2 2 0 00-2 2v14a2 2 0 002 2z" />
    </svg>
  )
}

function ArrowUpIcon() {
  return (
    <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M5 10l7-7m0 0l7 7m-7-7v18" />
    </svg>
  )
}

export default function RemoteFileBrowserModal({
  sessionId,
  hostLabel,
  onClose,
}: {
  sessionId: string
  hostLabel: string
  onClose: () => void
}) {
  const [path, setPath] = useState('~')
  const [entries, setEntries] = useState<RemoteEntry[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [localDir, setLocalDir] = useState('')
  const [phase, setPhase] = useState<Phase>('browsing')
  const [statuses, setStatuses] = useState<Record<string, FileStatus>>({})
  const [downloadError, setDownloadError] = useState('')
  const unsubRef = useRef<() => void>()

  const loadDir = useCallback(async (dir: string) => {
    setLoading(true)
    setError('')
    try {
      const result = await transfer.listRemoteDir(sessionId, dir)
      // Sort: directories first, then alphabetically
      result.sort((a, b) => {
        if (a.isDir !== b.isDir) return a.isDir ? -1 : 1
        return a.name.localeCompare(b.name)
      })
      setEntries(result)
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
      setEntries([])
    } finally {
      setLoading(false)
    }
  }, [sessionId])

  useEffect(() => {
    void loadDir(path)
  }, [path, loadDir])

  useEffect(() => {
    return () => unsubRef.current?.()
  }, [])

  function navigateTo(newPath: string) {
    setSelected(new Set())
    setPath(newPath)
  }

  function goUp() {
    navigateTo(dirname(path))
  }

  function toggleSelection(name: string) {
    const absPath = joinPath(path, name)
    setSelected((prev) => {
      const next = new Set(prev)
      if (next.has(absPath)) {
        next.delete(absPath)
      } else {
        next.add(absPath)
      }
      return next
    })
  }

  async function handleBrowseLocal() {
    try {
      const dir = await transfer.pickDownloadDestination(sessionId)
      if (dir) setLocalDir(dir)
    } catch {
      // user cancelled
    }
  }

  async function handleDownload() {
    if (selected.size === 0 || !localDir) return

    setDownloadError('')
    setStatuses({})  // start empty — progress events populate entries
    setPhase('downloading')

    const selectedPaths = Array.from(selected)
    try {
      const transferId = await transfer.downloadFiles(sessionId, localDir, selectedPaths)
      const unsub = transfer.subscribeDownload(transferId, {
        onProgress: (p: DownloadProgress) => {
          setStatuses((prev) => ({
            ...prev,
            [p.name]: {name: p.name, total: p.total, bytes: p.bytes, done: false, ok: false},
          }))
        },
        onDone: (results: DownloadFileResult[]) => {
          setStatuses((prev) => {
            const next = {...prev}
            for (const r of results) {
              next[r.name] = {
                name: r.name,
                total: next[r.name]?.total ?? r.bytes,
                bytes: r.bytes,
                done: true,
                ok: r.ok,
                error: r.error,
              }
            }
            return next
          })
          setPhase('done')
          unsub()
        },
      })
      unsubRef.current = unsub
    } catch (e) {
      setDownloadError(e instanceof Error ? e.message : String(e))
      setPhase('browsing')
    }
  }

  const numSelected = selected.size

  // Count how many selected are directories (for the button label).
  const selectedNames = Array.from(selected).map((p) => p.split('/').pop() || '')
  const numSelectedDirs = entries.filter((e) => e.isDir && selectedNames.includes(e.name)).length
  const numSelectedFiles = numSelected - numSelectedDirs

  return (
    <Modal title="Download files" onClose={onClose} width={560}>
      {phase === 'browsing' && (
        <div className="space-y-3">
          <p className="text-xs text-muted">
            Browse files on <span className="text-text">{hostLabel}</span>
          </p>

          {/* Path bar */}
          <div className="flex items-center gap-2">
            <button
              onClick={goUp}
              disabled={path === '/' || path === '~'}
              className="flex h-8 w-8 items-center justify-center rounded-md border border-border text-muted hover:text-text disabled:opacity-30"
              title="Go up"
            >
              <ArrowUpIcon />
            </button>
            <input
              value={path}
              onChange={(e) => setPath(e.target.value)}
              onKeyDown={(e) => { if (e.key === 'Enter') navigateTo(path) }}
              className={`${inputClass} flex-1 font-mono text-xs`}
            />
          </div>

          {/* File listing */}
          <div className="max-h-64 overflow-y-auto rounded-md border border-border bg-surface-2">
            {loading ? (
              <div className="flex items-center justify-center py-8 text-xs text-muted">Loading…</div>
            ) : error ? (
              <div className="px-3 py-4 text-xs text-danger">{error}</div>
            ) : entries.length === 0 ? (
              <div className="flex items-center justify-center py-8 text-xs text-muted">Empty directory</div>
            ) : (
              <table className="w-full table-auto text-xs">
                <thead>
                  <tr className="border-b border-border text-left text-muted">
                    <th className="w-8 px-2 py-1.5" />
                    <th className="px-2 py-1.5 font-normal">Name</th>
                    <th className="w-20 px-2 py-1.5 text-right font-normal">Size</th>
                    <th className="w-28 px-2 py-1.5 text-right font-normal">Modified</th>
                  </tr>
                </thead>
                <tbody>
                  {path !== '/' && path !== '~' && (
                    <tr
                      className="cursor-pointer text-muted hover:bg-surface"
                      onDoubleClick={goUp}
                    >
                      <td className="px-2 py-1.5" />
                      <td className="flex items-center gap-1.5 px-2 py-1.5">
                        <FolderIcon />
                        <span>..</span>
                      </td>
                      <td className="px-2 py-1.5 text-right" />
                      <td className="px-2 py-1.5 text-right" />
                    </tr>
                  )}
                  {entries.map((e) => {
                    const absPath = joinPath(path, e.name)
                    const checked = selected.has(absPath)
                    return (
                      <tr
                        key={e.name}
                        className="cursor-pointer hover:bg-surface"
                        onDoubleClick={() => { if (e.isDir) navigateTo(absPath) }}
                      >
                        <td className="px-2 py-1.5">
                          <input
                            type="checkbox"
                            checked={checked}
                            onChange={() => toggleSelection(e.name)}
                            className="h-3.5 w-3.5 accent-accent"
                          />
                        </td>
                        <td
                          className="flex items-center gap-1.5 px-2 py-1.5"
                          onClick={() => toggleSelection(e.name)}
                        >
                          {e.isDir ? <FolderIcon /> : <FileIcon />}
                          <span className="truncate text-text">{e.name}</span>
                        </td>
                        <td className="px-2 py-1.5 text-right text-muted">
                          {e.isDir ? '—' : humanSize(e.size)}
                        </td>
                        <td className="px-2 py-1.5 text-right text-muted">
                          {e.isDir ? '' : (e.modTime ? formatTime(e.modTime) : '—')}
                        </td>
                      </tr>
                    )
                  })}
                </tbody>
              </table>
            )}
          </div>

          {/* Local destination */}
          <div>
            <label className="mb-1 block text-xs text-muted">Local destination</label>
            <div className="flex gap-2">
              <input
                value={localDir}
                onChange={(e) => setLocalDir(e.target.value)}
                placeholder="Select a local folder…"
                className={`${inputClass} flex-1 font-mono text-xs`}
              />
              <button
                onClick={handleBrowseLocal}
                className="shrink-0 rounded-md border border-border bg-surface-2 px-3 py-2 text-xs text-text hover:bg-surface"
              >
                Browse…
              </button>
            </div>
          </div>

          {downloadError && <p className="text-xs text-danger">{downloadError}</p>}

          <div className="flex justify-end gap-2 pt-1">
            <button
              onClick={onClose}
              className="rounded-md border border-border bg-surface-2 px-3 py-2 text-sm text-text hover:bg-surface"
            >
              Cancel
            </button>
            <button
              onClick={handleDownload}
              disabled={numSelected === 0 || !localDir}
              className="rounded-md bg-accent px-3 py-2 text-sm text-white hover:bg-accent/80 disabled:opacity-50"
            >
              {numSelected === 0
                ? 'Download'
                : `Download ${numSelectedFiles > 0 ? `${numSelectedFiles} file${numSelectedFiles === 1 ? '' : 's'}` : ''}${numSelectedFiles > 0 && numSelectedDirs > 0 ? ' + ' : ''}${numSelectedDirs > 0 ? `${numSelectedDirs} folder${numSelectedDirs === 1 ? '' : 's'}` : ''}`
              }
            </button>
          </div>
        </div>
      )}

      {(phase === 'downloading' || phase === 'done') && (
        <div className="space-y-3">
          <p className="text-xs text-muted">
            Downloading to <span className="text-text">{localDir}</span>
          </p>

          <div className="max-h-64 space-y-2 overflow-y-auto">
            {Object.values(statuses).map((s) => {
              const pct = s.total > 0 ? Math.min(100, Math.round((s.bytes / s.total) * 100)) : s.done ? 100 : 0
              return (
                <div key={s.name} className="text-sm">
                  <div className="flex items-center justify-between">
                    <span className="truncate text-text">{s.name}</span>
                    <span className="ml-2 shrink-0 text-xs text-muted">
                      {s.done ? (s.ok ? '✓' : '✗') : `${pct}%`}
                    </span>
                  </div>
                  <div className="mt-1 h-1.5 w-full overflow-hidden rounded bg-surface-2">
                    <div
                      className={`h-full transition-all ${s.done && !s.ok ? 'bg-danger' : 'bg-accent'}`}
                      style={{width: `${pct}%`}}
                    />
                  </div>
                  {s.done && !s.ok && s.error && (
                    <p className="mt-0.5 text-xs text-danger">{s.error}</p>
                  )}
                </div>
              )
            })}
          </div>

          <div className="flex justify-end gap-2 pt-1">
            {phase === 'done' && (
              <button
                onClick={onClose}
                className="rounded-md bg-accent px-3 py-2 text-sm text-white hover:bg-accent/80"
              >
                Close
              </button>
            )}
          </div>
        </div>
      )}
    </Modal>
  )
}
