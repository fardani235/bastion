import {useState} from 'react'
import * as api from '../lib/api'

// Setup is shown on first launch: the user picks a master password that
// initializes the vault. There is NO recovery, so we require confirmation and
// state the consequence plainly.
export default function Setup({onDone}: {onDone: () => void}) {
  const [password, setPassword] = useState('')
  const [confirm, setConfirm] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  const canSubmit = password.length >= 8 && password === confirm && !busy

  async function submit() {
    if (password !== confirm) {
      setError('Passwords do not match.')
      return
    }
    if (password.length < 8) {
      setError('Use at least 8 characters.')
      return
    }
    setBusy(true)
    setError('')
    try {
      await api.setup(password)
      onDone()
    } catch (e) {
      setError(String(e))
      setBusy(false)
    }
  }

  return (
    <div className="flex h-full items-center justify-center bg-bg">
      <div className="w-[380px] rounded-xl border border-border bg-surface p-7 shadow-xl">
        <h1 className="mb-1 font-mono text-xl text-accent">bastion</h1>
        <p className="mb-5 text-sm text-muted">Create your master password to set up the vault.</p>

        <label className="mb-1 block text-xs text-muted">Master password</label>
        <input
          type="password"
          autoFocus
          value={password}
          onChange={(e) => setPassword(e.target.value)}
          className="mb-3 w-full rounded-md border border-border bg-surface-2 px-3 py-2 text-sm outline-none focus:border-accent"
        />

        <label className="mb-1 block text-xs text-muted">Confirm password</label>
        <input
          type="password"
          value={confirm}
          onChange={(e) => setConfirm(e.target.value)}
          onKeyDown={(e) => e.key === 'Enter' && canSubmit && submit()}
          className="mb-4 w-full rounded-md border border-border bg-surface-2 px-3 py-2 text-sm outline-none focus:border-accent"
        />

        {error && <p className="mb-3 text-sm text-danger">{error}</p>}

        <div className="mb-4 rounded-md border border-danger/40 bg-danger/10 px-3 py-2 text-xs text-danger">
          There is no password recovery. If you forget this password, the vault is permanently
          inaccessible.
        </div>

        <button
          disabled={!canSubmit}
          onClick={submit}
          className="w-full rounded-md bg-accent px-4 py-2 text-sm font-semibold text-bg transition-colors hover:bg-accent-dim disabled:cursor-not-allowed disabled:opacity-40"
        >
          {busy ? 'Setting up…' : 'Create vault'}
        </button>
      </div>
    </div>
  )
}
