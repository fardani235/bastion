import {useState} from 'react'
import Modal, {Field, inputClass, CustomSelect} from './Modal'
import * as api from '../lib/api'
import type {HostDTO} from '../lib/api'
import {useAppStore} from '../state/useAppStore'

// HostEditModal adds or edits a host. On edit, credential fields start blank
// and a blank value means "keep the stored secret" — the plaintext is never
// sent to the renderer, so the form cannot pre-fill or resend it.
export default function HostEditModal({
  host,
  defaultGroupId,
  onClose,
}: {
  host: HostDTO | null
  defaultGroupId?: string | null
  onClose: () => void
}) {
  const groups = useAppStore((s) => s.groups)
  const refreshHosts = useAppStore((s) => s.refreshHosts)
  const editing = host !== null

  const [label, setLabel] = useState(host?.label ?? '')
  const [hostname, setHostname] = useState(host?.hostname ?? '')
  const [port, setPort] = useState(host?.port ?? 22)
  const [username, setUsername] = useState(host?.username ?? '')
  const [groupId, setGroupId] = useState<string>(host?.groupId ?? defaultGroupId ?? '')
  const [authKind, setAuthKind] = useState<'password' | 'key'>(
    (host?.authKind as 'password' | 'key') ?? 'password',
  )
  const [password, setPassword] = useState('')
  const [keyPath, setKeyPath] = useState(host?.keyPath ?? '')
  const [keyPassphrase, setKeyPassphrase] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  const canSave = label && hostname && username && port > 0 && !busy

  async function save() {
    setBusy(true)
    setError('')
    const input: api.HostInput = {
      groupId: groupId || undefined,
      label,
      hostname,
      port: Number(port),
      username,
      authKind,
      password: authKind === 'password' ? password : '',
      keyPath: authKind === 'key' ? keyPath : '',
      keyPassphrase: authKind === 'key' ? keyPassphrase : '',
    }
    try {
      if (editing) await api.updateHost(host!.id, input)
      else await api.createHost(input)
      await refreshHosts()
      onClose()
    } catch (e) {
      setError(String(e))
      setBusy(false)
    }
  }

  const pwPlaceholder = editing && host?.hasPassword ? 'leave blank to keep current' : ''
  const passPlaceholder = editing && host?.hasKeyPassphrase ? 'leave blank to keep current' : ''

  return (
    <Modal title={editing ? 'Edit host' : 'Add host'} onClose={onClose} width={460}>
      <Field label="Label">
        <input className={inputClass} autoFocus value={label} onChange={(e) => setLabel(e.target.value)} />
      </Field>

      <div className="flex gap-3">
        <div className="flex-1">
          <Field label="Hostname">
            <input className={inputClass} value={hostname} onChange={(e) => setHostname(e.target.value)} />
          </Field>
        </div>
        <div className="w-24">
          <Field label="Port">
            <input
              className={inputClass}
              type="number"
              value={port}
              onChange={(e) => setPort(Number(e.target.value))}
            />
          </Field>
        </div>
      </div>

      <Field label="Username">
        <input className={inputClass} value={username} onChange={(e) => setUsername(e.target.value)} />
      </Field>

      <Field label="Group">
        <CustomSelect
          value={groupId}
          onChange={setGroupId}
          options={[
            {value: '', label: 'Ungrouped'},
            ...groups.map((g) => ({value: g.id, label: g.name})),
          ]}
        />
      </Field>

      <Field label="Authentication">
        <div className="flex gap-2">
          <button
            type="button"
            onClick={() => setAuthKind('password')}
            className={`flex-1 rounded-md border px-3 py-1.5 text-sm ${
              authKind === 'password'
                ? 'border-accent bg-accent/10 text-accent'
                : 'border-border bg-surface-2 text-muted'
            }`}
          >
            Password
          </button>
          <button
            type="button"
            onClick={() => setAuthKind('key')}
            className={`flex-1 rounded-md border px-3 py-1.5 text-sm ${
              authKind === 'key'
                ? 'border-accent bg-accent/10 text-accent'
                : 'border-border bg-surface-2 text-muted'
            }`}
          >
            Key
          </button>
        </div>
      </Field>

      {authKind === 'password' ? (
        <Field label="Password">
          <input
            className={inputClass}
            type="password"
            value={password}
            placeholder={pwPlaceholder}
            onChange={(e) => setPassword(e.target.value)}
          />
        </Field>
      ) : (
        <>
          <Field label="Private key path">
            <input
              className={inputClass}
              value={keyPath}
              placeholder="/home/you/.ssh/id_ed25519"
              onChange={(e) => setKeyPath(e.target.value)}
            />
          </Field>
          <Field label="Key passphrase (if encrypted)">
            <input
              className={inputClass}
              type="password"
              value={keyPassphrase}
              placeholder={passPlaceholder}
              onChange={(e) => setKeyPassphrase(e.target.value)}
            />
          </Field>
        </>
      )}

      {error && <p className="mb-3 text-sm text-danger">{error}</p>}

      <div className="mt-2 flex justify-end gap-2">
        <button onClick={onClose} className="rounded-md border border-border px-4 py-2 text-sm text-muted hover:text-text">
          Cancel
        </button>
        <button
          disabled={!canSave}
          onClick={save}
          className="rounded-md bg-accent px-4 py-2 text-sm font-semibold text-bg hover:bg-accent-dim disabled:opacity-40"
        >
          {busy ? 'Saving…' : 'Save'}
        </button>
      </div>
    </Modal>
  )
}
