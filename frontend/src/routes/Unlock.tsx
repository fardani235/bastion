import {useState} from 'react'
import * as api from '../lib/api'

// Unlock is shown on every subsequent launch. A wrong password is rejected by
// the Go side (verify-blob check) without decrypting any credential.
export default function Unlock({onDone}: {onDone: () => void}) {
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  async function submit() {
    if (!password || busy) return
    setBusy(true)
    setError('')
    try {
      await api.unlock(password)
      onDone()
    } catch (e) {
      setError('Incorrect master password.')
      setBusy(false)
      setPassword('')
    }
  }

  return (
    <div className="flex h-full items-center justify-center bg-bg">
      <div className="w-[360px] rounded-xl border border-border bg-surface p-7 shadow-xl">
        <img src="/icon64.png" alt="" className="mx-auto mb-3 h-16 w-16" />
        <h1 className="mb-1 text-center font-mono text-xl text-accent">bastion</h1>
        <p className="mb-5 text-center text-sm text-muted">Enter your master password to unlock.</p>

        <input
          type="password"
          autoFocus
          value={password}
          onChange={(e) => setPassword(e.target.value)}
          onKeyDown={(e) => e.key === 'Enter' && submit()}
          className="mb-3 w-full rounded-md border border-border bg-surface-2 px-3 py-2 text-sm outline-none focus:border-accent"
        />

        {error && <p className="mb-3 text-sm text-danger">{error}</p>}

        <button
          disabled={!password || busy}
          onClick={submit}
          className="w-full rounded-md bg-accent px-4 py-2 text-sm font-semibold text-bg transition-colors hover:bg-accent-dim disabled:cursor-not-allowed disabled:opacity-40"
        >
          {busy ? 'Unlocking…' : 'Unlock'}
        </button>
      </div>
    </div>
  )
}
