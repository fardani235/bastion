import {useState} from 'react'
import Modal from './Modal'
import {useAppStore} from '../state/useAppStore'

// TrustHostKeyModal shows the server's fingerprint the first time we connect to
// a host. Accepting calls the stored retry (which trusts the key, then retries
// OpenSession). Rejecting just dismisses.
export default function TrustHostKeyModal() {
  const trustPrompt = useAppStore((s) => s.trustPrompt)
  const setTrustPrompt = useAppStore((s) => s.setTrustPrompt)
  const [busy, setBusy] = useState(false)

  if (!trustPrompt) return null
  const {info, retry} = trustPrompt

  async function accept() {
    setBusy(true)
    try {
      await retry()
    } catch {
      // retry already transitions the tab to disconnected
    } finally {
      setBusy(false)
      setTrustPrompt(null)
    }
  }

  return (
    <Modal title="Unknown host key" onClose={() => setTrustPrompt(null)} width={480}>
      <p className="mb-3 text-sm text-muted">
        The authenticity of{' '}
        <span className="font-mono text-text">
          {info.hostname}:{info.port}
        </span>{' '}
        can't be established. Verify the fingerprint before trusting it.
      </p>

      <div className="mb-4 rounded-md border border-border bg-surface-2 px-3 py-2 font-mono text-xs">
        <div className="text-muted">{info.keyType}</div>
        <div className="break-all text-accent">{info.fingerprintSHA256}</div>
      </div>

      <div className="flex justify-end gap-2">
        <button
          onClick={() => setTrustPrompt(null)}
          className="rounded-md border border-border px-4 py-2 text-sm text-muted hover:text-text"
        >
          Cancel
        </button>
        <button
          disabled={busy}
          onClick={accept}
          className="rounded-md bg-accent px-4 py-2 text-sm font-semibold text-bg hover:bg-accent-dim disabled:opacity-40"
        >
          {busy ? 'Connecting…' : 'Trust and connect'}
        </button>
      </div>
    </Modal>
  )
}
