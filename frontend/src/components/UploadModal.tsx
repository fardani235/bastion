import {useEffect, useMemo, useState} from 'react'
import Modal, {Field, inputClass} from './Modal'
import * as transfer from '../lib/transfer'
import type {UploadCandidate, UploadProgress, UploadFileResult} from '../lib/transfer'

// Per-file UI status, keyed by file name, during an in-flight transfer.
interface FileStatus {
  name: string
  total: number
  bytes: number
  done: boolean
  ok: boolean
  error?: string
}

function humanSize(n: number): string {
  if (n < 1024) return `${n} B`
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`
  if (n < 1024 * 1024 * 1024) return `${(n / (1024 * 1024)).toFixed(1)} MB`
  return `${(n / (1024 * 1024 * 1024)).toFixed(1)} GB`
}

type Phase = 'confirm' | 'uploading' | 'done'

/**
 * UploadModal walks a drop through confirm -> upload -> result. It is given the
 * dropped candidates and the resolved destination (from PrepareUpload), shows an
 * editable destination path, and on confirm streams per-file progress from the
 * backend. The API key/secret boundary is irrelevant here — only paths and byte
 * counts cross IPC.
 */
export default function UploadModal({
  sessionId,
  hostLabel,
  destDir: initialDest,
  candidates,
  paths,
  onClose,
}: {
  sessionId: string
  hostLabel: string
  destDir: string
  candidates: UploadCandidate[]
  paths: string[]
  onClose: () => void
}) {
  const [destDir, setDestDir] = useState(initialDest)
  const [phase, setPhase] = useState<Phase>('confirm')
  const [error, setError] = useState('')
  const [statuses, setStatuses] = useState<Record<string, FileStatus>>({})

  const uploadable = useMemo(() => candidates.filter((c) => c.upload), [candidates])
  const skipped = useMemo(() => candidates.filter((c) => !c.upload), [candidates])

  async function handleUpload() {
    if (!destDir.trim()) {
      setError('Destination directory is required')
      return
    }
    if (uploadable.length === 0) {
      setError('No uploadable files (folders are skipped)')
      return
    }
    setError('')

    // Seed per-file statuses so every bar is visible from the start.
    const seed: Record<string, FileStatus> = {}
    for (const c of uploadable) seed[c.name] = {name: c.name, total: c.size, bytes: 0, done: false, ok: false}
    setStatuses(seed)
    setPhase('uploading')

    let unsub = () => {}
    try {
      const transferId = await transfer.uploadFiles(sessionId, destDir.trim(), paths)
      unsub = transfer.subscribeUpload(transferId, {
        onProgress: (p: UploadProgress) => {
          setStatuses((prev) => ({
            ...prev,
            [p.name]: {...prev[p.name], name: p.name, total: p.total, bytes: p.bytes, done: false, ok: false},
          }))
        },
        onDone: (results: UploadFileResult[]) => {
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
    } catch (e) {
      unsub()
      setError(e instanceof Error ? e.message : String(e))
      setPhase('confirm')
    }
  }

  // Clean up the event subscription if the modal is closed mid-flight.
  useEffect(() => () => { /* subscription is unsubbed in onDone/catch */ }, [])

  const rows = phase === 'confirm' ? uploadable : Object.values(statuses)
  const allOk = phase === 'done' && Object.values(statuses).every((s) => s.ok)

  return (
    <Modal title="Upload files" onClose={onClose} width={480}>
      <div className="space-y-4">
        <p className="text-xs text-muted">
          Uploading to <span className="text-text">{hostLabel}</span>
        </p>

        <Field label="Destination directory">
          <input
            value={destDir}
            onChange={(e) => setDestDir(e.target.value)}
            disabled={phase !== 'confirm'}
            placeholder="~"
            className={inputClass}
          />
        </Field>

        <div className="max-h-52 space-y-2 overflow-y-auto">
          {phase === 'confirm'
            ? uploadable.map((c) => (
                <div key={c.path} className="flex items-center justify-between text-sm">
                  <span className="truncate text-text">{c.name}</span>
                  <span className="ml-2 shrink-0 text-xs text-muted">{humanSize(c.size)}</span>
                </div>
              ))
            : (rows as FileStatus[]).map((s) => {
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

        {skipped.length > 0 && phase === 'confirm' && (
          <div className="rounded-md border border-border bg-surface-2 p-2 text-xs text-muted">
            {skipped.length} item{skipped.length > 1 ? 's' : ''} will be skipped:
            <ul className="mt-1 space-y-0.5">
              {skipped.map((c) => (
                <li key={c.path} className="truncate">
                  {c.name} — {c.reason}
                </li>
              ))}
            </ul>
          </div>
        )}

        {error && <p className="text-xs text-danger">{error}</p>}

        <div className="flex justify-end gap-2 pt-1">
          {phase === 'done' ? (
            <button
              onClick={onClose}
              className="rounded-md bg-accent px-3 py-2 text-sm text-white hover:bg-accent/80"
            >
              {allOk ? 'Done' : 'Close'}
            </button>
          ) : (
            <>
              <button
                onClick={onClose}
                disabled={phase === 'uploading'}
                className="rounded-md border border-border bg-surface-2 px-3 py-2 text-sm text-text hover:bg-surface disabled:opacity-50"
              >
                Cancel
              </button>
              <button
                onClick={handleUpload}
                disabled={phase === 'uploading' || uploadable.length === 0}
                className="rounded-md bg-accent px-3 py-2 text-sm text-white hover:bg-accent/80 disabled:opacity-50"
              >
                {phase === 'uploading' ? 'Uploading…' : `Upload ${uploadable.length} file${uploadable.length === 1 ? '' : 's'}`}
              </button>
            </>
          )}
        </div>
      </div>
    </Modal>
  )
}
