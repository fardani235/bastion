import {useState} from 'react'
import Modal, {Field, inputClass} from './Modal'
import * as api from '../lib/api'
import type {Group} from '../lib/api'
import {useAppStore} from '../state/useAppStore'

// GroupEditModal adds a new group or renames an existing one.
export default function GroupEditModal({
  group,
  onClose,
}: {
  group: Group | null
  onClose: () => void
}) {
  const refreshGroups = useAppStore((s) => s.refreshGroups)
  const editing = group !== null
  const [name, setName] = useState(group?.name ?? '')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  async function save() {
    if (!name || busy) return
    setBusy(true)
    setError('')
    try {
      if (editing) await api.renameGroup(group!.id, name)
      else await api.createGroup(name)
      await refreshGroups()
      onClose()
    } catch (e) {
      setError(String(e))
      setBusy(false)
    }
  }

  return (
    <Modal title={editing ? 'Rename group' : 'Add group'} onClose={onClose}>
      <Field label="Group name">
        <input
          className={inputClass}
          autoFocus
          value={name}
          onChange={(e) => setName(e.target.value)}
          onKeyDown={(e) => e.key === 'Enter' && save()}
        />
      </Field>
      {error && <p className="mb-3 text-sm text-danger">{error}</p>}
      <div className="mt-2 flex justify-end gap-2">
        <button onClick={onClose} className="rounded-md border border-border px-4 py-2 text-sm text-muted hover:text-text">
          Cancel
        </button>
        <button
          disabled={!name || busy}
          onClick={save}
          className="rounded-md bg-accent px-4 py-2 text-sm font-semibold text-bg hover:bg-accent-dim disabled:opacity-40"
        >
          {busy ? 'Saving…' : 'Save'}
        </button>
      </div>
    </Modal>
  )
}
