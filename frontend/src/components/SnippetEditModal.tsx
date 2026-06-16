import {useState} from 'react'
import Modal, {Field, inputClass} from './Modal'
import * as api from '../lib/api'
import type {Snippet} from '../lib/api'
import {useAppStore} from '../state/useAppStore'

// SnippetEditModal adds or edits a reusable command snippet.
export default function SnippetEditModal({
  snippet,
  onClose,
}: {
  snippet: Snippet | null
  onClose: () => void
}) {
  const refreshSnippets = useAppStore((s) => s.refreshSnippets)
  const editing = snippet !== null
  const [label, setLabel] = useState(snippet?.label ?? '')
  const [body, setBody] = useState(snippet?.body ?? '')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  async function save() {
    if (!label || !body || busy) return
    setBusy(true)
    setError('')
    try {
      if (editing) await api.updateSnippet(snippet!.id, label, body)
      else await api.createSnippet(label, body)
      await refreshSnippets()
      onClose()
    } catch (e) {
      setError(String(e))
      setBusy(false)
    }
  }

  return (
    <Modal title={editing ? 'Edit snippet' : 'Add snippet'} onClose={onClose} width={460}>
      <Field label="Label">
        <input className={inputClass} autoFocus value={label} onChange={(e) => setLabel(e.target.value)} />
      </Field>
      <Field label="Command">
        <textarea
          className={`${inputClass} h-28 resize-none font-mono`}
          value={body}
          onChange={(e) => setBody(e.target.value)}
        />
      </Field>
      {error && <p className="mb-3 text-sm text-danger">{error}</p>}
      <div className="mt-2 flex justify-end gap-2">
        <button onClick={onClose} className="rounded-md border border-border px-4 py-2 text-sm text-muted hover:text-text">
          Cancel
        </button>
        <button
          disabled={!label || !body || busy}
          onClick={save}
          className="rounded-md bg-accent px-4 py-2 text-sm font-semibold text-bg hover:bg-accent-dim disabled:opacity-40"
        >
          {busy ? 'Saving…' : 'Save'}
        </button>
      </div>
    </Modal>
  )
}
