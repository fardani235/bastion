import {useCallback, useEffect, useState} from 'react'
import Modal, {Field, inputClass} from './Modal'
import * as api from '../lib/api'
import type {PortForwardDTO, ActiveForwardInfo} from '../lib/api'
import {useAppStore} from '../state/useAppStore'

type View = 'list' | 'form'

export default function PortForwardModal({
  hostId,
  onClose,
}: {
  hostId: string
  onClose: () => void
}) {
  const activeForwards = useAppStore((s) => s.activeForwards)
  const [forwards, setForwards] = useState<PortForwardDTO[]>([])
  const [view, setView] = useState<View>('list')
  const [editing, setEditing] = useState<PortForwardDTO | null>(null)

  // form fields
  const [label, setLabel] = useState('')
  const [localPort, setLocalPort] = useState('')
  const [remoteHost, setRemoteHost] = useState('localhost')
  const [remotePort, setRemotePort] = useState('')
  const [enabled, setEnabled] = useState(true)
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  const loadForwards = useCallback(async () => {
    try {
      const items = await api.listPortForwards(hostId)
      setForwards(items ?? [])
    } catch {
      // ignore
    }
  }, [hostId])

  useEffect(() => {
    void loadForwards()
  }, [loadForwards])

  function startCreate() {
    setEditing(null)
    setLabel('')
    setLocalPort('')
    setRemoteHost('localhost')
    setRemotePort('')
    setEnabled(true)
    setError('')
    setView('form')
  }

  function startEdit(f: PortForwardDTO) {
    setEditing(f)
    setLabel(f.label)
    setLocalPort(String(f.localPort))
    setRemoteHost(f.remoteHost)
    setRemotePort(String(f.remotePort))
    setEnabled(f.enabled)
    setError('')
    setView('form')
  }

  function cancelForm() {
    setView('list')
    setError('')
  }

  function activeInfo(f: PortForwardDTO): ActiveForwardInfo | undefined {
    return activeForwards.find((a) => a.id === f.id)
  }

  async function save() {
    setBusy(true)
    setError('')
    try {
      const lp = parseInt(localPort, 10)
      const rp = parseInt(remotePort, 10)
      if (isNaN(lp) || lp < 1 || lp > 65535) throw new Error('Local port must be 1-65535')
      if (isNaN(rp) || rp < 1 || rp > 65535) throw new Error('Remote port must be 1-65535')
      if (editing) {
        await api.updatePortForward(editing.id, hostId, label, lp, remoteHost, rp, enabled)
      } else {
        await api.createPortForward(hostId, label, lp, remoteHost, rp, enabled)
      }
      setView('list')
      await loadForwards()
    } catch (e) {
      setError(String(e))
    } finally {
      setBusy(false)
    }
  }

  async function remove(id: string) {
    try {
      await api.deletePortForward(id)
      await loadForwards()
    } catch (e) {
      console.error('deletePortForward:', e)
    }
  }

  async function toggleEnabled(f: PortForwardDTO) {
    try {
      await api.updatePortForward(f.id, hostId, f.label, f.localPort, f.remoteHost, f.remotePort, !f.enabled)
      await loadForwards()
    } catch (e) {
      console.error('toggleEnabled:', e)
    }
  }

  const canSave = label && localPort && remoteHost && remotePort && !busy

  if (view === 'form') {
    return (
      <Modal title={editing ? 'Edit Port Forward' : 'Add Port Forward'} onClose={onClose}>
        <Field label="Label">
          <input className={inputClass} value={label} onChange={(e) => setLabel(e.target.value)} placeholder="e.g. web app" />
        </Field>
        <Field label="Local Port">
          <input className={inputClass} type="number" min={1} max={65535} value={localPort} onChange={(e) => setLocalPort(e.target.value)} />
        </Field>
        <Field label="Remote Host">
          <input className={inputClass} value={remoteHost} onChange={(e) => setRemoteHost(e.target.value)} placeholder="e.g. 10.0.0.42" />
        </Field>
        <Field label="Remote Port">
          <input className={inputClass} type="number" min={1} max={65535} value={remotePort} onChange={(e) => setRemotePort(e.target.value)} />
        </Field>
        <Field label="">
          <label className="flex items-center gap-2 text-sm">
            <input type="checkbox" checked={enabled} onChange={(e) => setEnabled(e.target.checked)} />
            Auto-start on connect
          </label>
        </Field>
        {error && <p className="text-red-500 text-sm">{error}</p>}
        <div className="flex justify-end gap-2 mt-4">
          <button className="px-3 py-1 rounded hover:bg-surface-alt" onClick={cancelForm}>
            Back
          </button>
          <button className="px-3 py-1 rounded bg-accent text-white disabled:opacity-50" disabled={!canSave} onClick={save}>
            {busy ? 'Saving...' : 'Save'}
          </button>
        </div>
      </Modal>
    )
  }

  return (
    <Modal title="Port Forwards" onClose={onClose} width={520}>
      <div className="flex justify-end mb-2">
        <button className="text-sm text-accent hover:text-accent-dim" onClick={startCreate}>
          + Add Forward
        </button>
      </div>
      {forwards.length === 0 ? (
        <p className="text-sm text-muted/60 py-4 text-center">No port forwards configured for this host.</p>
      ) : (
        <div className="space-y-1 max-h-80 overflow-y-auto">
          {forwards.map((f) => {
            const active = activeInfo(f)
            return (
              <div key={f.id} className="flex items-center gap-2 rounded px-2 py-1.5 text-sm hover:bg-surface-2 group">
                <button
                  onClick={() => toggleEnabled(f)}
                  className={`h-2 w-2 shrink-0 rounded-full ${f.enabled ? 'bg-green-500' : 'bg-muted/40'}`}
                  title={f.enabled ? 'Enabled (click to disable)' : 'Disabled (click to enable)'}
                />
                <span className="min-w-0 flex-1">
                  <span className="text-text">{f.label}</span>
                  <span className="text-muted ml-2">
                    {f.localPort} → {f.remoteHost}:{f.remotePort}
                  </span>
                  {active && (
                    <span className="text-muted/60 ml-2 text-xs">
                      ({active.activeConns} conn{active.activeConns === 1 ? '' : 's'})
                    </span>
                  )}
                  {!f.enabled && <span className="text-muted/40 ml-2 text-xs">disabled</span>}
                </span>
                <span className="hidden items-center gap-1 group-hover:flex">
                  <button onClick={() => startEdit(f)} className="text-muted hover:text-text" title="Edit">
                    ✎
                  </button>
                  <button onClick={() => remove(f.id)} className="text-muted hover:text-danger" title="Delete">
                    ✕
                  </button>
                </span>
              </div>
            )
          })}
        </div>
      )}
    </Modal>
  )
}
