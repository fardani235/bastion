import {useEffect, useState} from 'react'
import Modal from './Modal'
import * as api from '../lib/api'
import {useAppStore} from '../state/useAppStore'

export default function SettingsModal({onClose}: {onClose: () => void}) {
  const refreshAutoLockSettings = useAppStore((s) => s.refreshAutoLockSettings)
  const storedIdle = useAppStore((s) => s.autoLockIdleEnabled)
  const storedScreensaver = useAppStore((s) => s.autoLockScreensaverEnabled)

  const [idleEnabled, setIdleEnabled] = useState(storedIdle)
  const [screensaverEnabled, setScreensaverEnabled] = useState(storedScreensaver)
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    setIdleEnabled(storedIdle)
    setScreensaverEnabled(storedScreensaver)
  }, [storedIdle, storedScreensaver])

  async function handleSave() {
    setSaving(true)
    try {
      await Promise.all([
        api.setAutoLockIdleEnabled(idleEnabled),
        api.setAutoLockScreensaverEnabled(screensaverEnabled),
      ])
      await refreshAutoLockSettings()
      onClose()
    } catch (e) {
      console.error('settings save error:', e)
    } finally {
      setSaving(false)
    }
  }

  return (
    <Modal title="Settings" onClose={onClose} width={420}>
      <div className="space-y-5">
        <label className="flex cursor-pointer items-start gap-2">
          <input
            type="checkbox"
            checked={idleEnabled}
            onChange={(e) => setIdleEnabled(e.target.checked)}
            className="mt-0.5"
          />
          <span className="text-sm text-text">
            Auto-lock on inactivity
            <span className="mt-0.5 block text-xs text-muted">
              Locks the vault and closes all sessions (including port forwards)
              after a period of inactivity. Off by default. Configure the timeout
              in the Lock menu.
            </span>
          </span>
        </label>

        <label className="flex cursor-pointer items-start gap-2">
          <input
            type="checkbox"
            checked={screensaverEnabled}
            onChange={(e) => setScreensaverEnabled(e.target.checked)}
            className="mt-0.5"
          />
          <span className="text-sm text-text">
            Auto-lock on screen lock
            <span className="mt-0.5 block text-xs text-muted">
              Locks the vault and closes all sessions when the operating system
              screen saver or lock screen activates. Off by default.
            </span>
          </span>
        </label>

        <div className="flex justify-end gap-2 pt-2">
          <button
            onClick={onClose}
            className="rounded-md border border-border bg-surface-2 px-3 py-2 text-sm text-text hover:bg-surface"
          >
            Cancel
          </button>
          <button
            onClick={handleSave}
            disabled={saving}
            className="rounded-md bg-accent px-3 py-2 text-sm text-white hover:bg-accent/80 disabled:opacity-50"
          >
            {saving ? 'Saving…' : 'Save'}
          </button>
        </div>
      </div>
    </Modal>
  )
}
