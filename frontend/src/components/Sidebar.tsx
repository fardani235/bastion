import {useMemo, useState} from 'react'
import {useAppStore} from '../state/useAppStore'
import * as api from '../lib/api'
import type {HostDTO, Group, PortForwardDTO, ScannedHost} from '../lib/api'
import {openHostSession} from '../lib/sessions'
import HostItem from './HostItem'
import HostEditModal from './HostEditModal'
import HostContextMenu from './HostContextMenu'
import GroupEditModal from './GroupEditModal'
import PortForwardModal from './PortForwardModal'
import Modal from './Modal'

const UNGROUPED = '__ungrouped__'

// Sidebar lists groups (collapsible) and their hosts, plus add actions. It owns
// the modal open-state and dispatches connect requests through openHostSession.
export default function Sidebar() {
  const hosts = useAppStore((s) => s.hosts)
  const groups = useAppStore((s) => s.groups)
  const refreshHosts = useAppStore((s) => s.refreshHosts)
  const refreshGroups = useAppStore((s) => s.refreshGroups)
  const setTrustPrompt = useAppStore((s) => s.setTrustPrompt)
  const refreshActiveForwards = useAppStore((s) => s.refreshActiveForwards)

  const [sidebarCollapsed, setSidebarCollapsed] = useState(false)
  const [collapsed, setCollapsed] = useState<Record<string, boolean>>({})
  const [hostModal, setHostModal] = useState<{host: HostDTO | null; groupId?: string | null} | null>(null)
  const [groupModal, setGroupModal] = useState<{group: Group | null} | null>(null)
  const [pfHostId, setPFHostId] = useState<string | null>(null)
  const [ctxMenu, setCtxMenu] = useState<{host: HostDTO; x: number; y: number} | null>(null)
  const [scannedHosts, setScannedHosts] = useState<ScannedHost[] | null>(null)
  const [importError, setImportError] = useState<string | null>(null)
  const [importing, setImporting] = useState(false)
  const [importDone, setImportDone] = useState<number | null>(null)

  function openPortForwards(hostId: string) {
    setPFHostId(hostId)
  }

  // Group hosts by their group id (null -> Ungrouped bucket).
  const byGroup = useMemo(() => {
    const m = new Map<string, HostDTO[]>()
    for (const h of hosts) {
      const key = h.groupId ?? UNGROUPED
      if (!m.has(key)) m.set(key, [])
      m.get(key)!.push(h)
    }
    return m
  }, [hosts])

  function connect(host: HostDTO) {
    void openHostSession(host, {
      onTrustPrompt: (info, retry) => setTrustPrompt({info, retry}),
    })
  }

  async function deleteHost(h: HostDTO) {
    try {
      await api.deleteHost(h.id)
      await refreshHosts()
    } catch (e) {
      console.error('deleteHost:', e)
    }
  }

  async function deleteGroup(g: Group) {
    try {
      await api.deleteGroup(g.id)
      await Promise.all([refreshGroups(), refreshHosts()])
    } catch (e) {
      console.error('deleteGroup:', e)
    }
  }

  async function handleImportSSH() {
    if (importing) return
    setImporting(true)
    setImportError(null)
    setScannedHosts(null)
    try {
      const hosts = await api.scanSSHConfig()
      if (hosts.length === 0) {
        setImportError('No hosts found in ~/.ssh/config.')
      } else {
        setScannedHosts(hosts)
      }
    } catch (e) {
      setImportError('Failed to scan SSH config: ' + (e instanceof Error ? e.message : String(e)))
    } finally {
      setImporting(false)
    }
  }

  async function confirmImport() {
    setImporting(true)
    setImportError(null)
    try {
      const imported = await api.importSSHConfig()
      await refreshHosts()
      setScannedHosts(null)
      setImportDone(imported.length)
    } catch (e) {
      setImportError('Import failed: ' + (e instanceof Error ? e.message : String(e)))
    } finally {
      setImporting(false)
    }
  }

  function closeImportModals() {
    setScannedHosts(null)
    setImportError(null)
    setImportDone(null)
  }

  const sections: {id: string; name: string; group: Group | null}[] = [
    ...groups.map((g) => ({id: g.id, name: g.name, group: g})),
    {id: UNGROUPED, name: 'Ungrouped', group: null},
  ]

  return (
    <div className={`relative flex shrink-0 flex-col border-r border-border bg-surface transition-all duration-200 overflow-hidden ${sidebarCollapsed ? 'w-8' : 'w-[260px]'}`}>
      <div className={`flex min-w-[260px] flex-col ${sidebarCollapsed ? 'opacity-0 pointer-events-none' : ''}`}>
        <div className="flex items-center justify-between px-3 py-3">
          <span className="font-mono text-sm text-accent">bastion</span>
          <div className="flex gap-2 text-xs">
            <button onClick={() => setGroupModal({group: null})} className="text-muted hover:text-text" title="Add group">
              + Group
            </button>
            <button onClick={() => setHostModal({host: null})} className="text-accent hover:text-accent-dim" title="Add host">
              + Host
            </button>
            <button onClick={handleImportSSH} disabled={importing} className="text-muted hover:text-text" title="Import from ~/.ssh/config">
              {importing ? '…' : '↶ SSH'}
            </button>
          </div>
        </div>

        <div className="flex-1 overflow-y-auto px-2 pb-2">
          {sections.map((section) => {
            const items = byGroup.get(section.id) ?? []
            // Hide the empty Ungrouped bucket; always show named groups.
            if (section.id === UNGROUPED && items.length === 0) return null
            const isCollapsed = collapsed[section.id]
            return (
              <div key={section.id} className="mb-1">
                <div className="group flex items-center gap-1 rounded-md px-1 py-1 text-xs uppercase tracking-wide text-muted">
                  <button
                    onClick={() => setCollapsed((c) => ({...c, [section.id]: !c[section.id]}))}
                    className="flex flex-1 items-center gap-1 text-left hover:text-text"
                  >
                    <span className="inline-block w-3">{isCollapsed ? '▸' : '▾'}</span>
                    {section.name}
                    <span className="ml-1 text-muted/60">{items.length}</span>
                  </button>
                  {section.group && (
                    <span className="hidden gap-1 group-hover:flex">
                      <button onClick={() => setGroupModal({group: section.group})} className="hover:text-text" title="Rename">
                        ✎
                      </button>
                      <button onClick={() => deleteGroup(section.group!)} className="hover:text-danger" title="Delete group">
                        ✕
                      </button>
                    </span>
                  )}
                </div>
                {!isCollapsed && (
                  <div className="ml-1">
                    {items.map((h) => (
                      <HostItem
                        key={h.id}
                        host={h}
                        onConnect={() => connect(h)}
                        onEdit={() => setHostModal({host: h})}
                        onDelete={() => deleteHost(h)}
                        onPortForwards={() => openPortForwards(h.id)}
                        onContextMenu={(e, host) => {
                          e.preventDefault()
                          setCtxMenu({host, x: e.clientX, y: e.clientY})
                        }}
                      />
                    ))}
                    {items.length === 0 && <div className="px-2 py-1 text-xs text-muted/60">No hosts</div>}
                  </div>
                )}
              </div>
            )
          })}
        </div>
      </div>

      <div className="mt-auto flex h-7 items-center justify-center">
        <button
          onClick={(e) => { e.stopPropagation(); setSidebarCollapsed((c) => !c) }}
          className="flex h-5 w-5 items-center justify-center rounded text-xs text-muted hover:text-text hover:bg-surface-2"
          title={sidebarCollapsed ? 'Expand sidebar' : 'Collapse sidebar'}
        >
          {sidebarCollapsed ? '▸' : '◂'}
        </button>
      </div>

      {hostModal && (
        <HostEditModal
          key={hostModal.host?.id ?? '__new__'}
          host={hostModal.host}
          defaultGroupId={hostModal.groupId}
          onClose={() => setHostModal(null)}
        />
      )}
      {groupModal && (
        <GroupEditModal
          key={groupModal.group?.id ?? '__new__'}
          group={groupModal.group}
          onClose={() => setGroupModal(null)}
        />
      )}
      {pfHostId && (
        <PortForwardModal
          key={pfHostId}
          hostId={pfHostId}
          onClose={() => {
            setPFHostId(null)
            void api.fire(refreshActiveForwards(), 'refreshActiveForwards')
          }}
        />
      )}
      {scannedHosts && (
        <Modal title="Import from ~/.ssh/config" onClose={closeImportModals} width={480}>
          <p className="mb-3 text-sm text-muted">
            Found {scannedHosts.length} host{scannedHosts.length === 1 ? '' : 's'}:
          </p>
          <div className="max-h-52 overflow-y-auto rounded border border-border bg-surface-2">
            {scannedHosts.map((h, i) => (
              <div key={i} className="flex items-center gap-3 border-b border-border px-3 py-2 text-xs last:border-0">
                <span className="font-medium text-text">{h.label}</span>
                <span className="text-muted">{h.username ? h.username + '@' : ''}{h.hostname}:{h.port}</span>
                {h.identityFile && <span className="ml-auto truncate text-muted/60">{h.identityFile}</span>}
              </div>
            ))}
          </div>
          <div className="mt-4 flex justify-end gap-2">
            <button onClick={closeImportModals} className="rounded-md border border-border px-4 py-1.5 text-sm text-muted hover:text-text">
              Cancel
            </button>
            <button onClick={confirmImport} disabled={importing} className="rounded-md bg-accent px-4 py-1.5 text-sm text-white hover:bg-accent-dim disabled:opacity-50">
              {importing ? 'Importing…' : `Import ${scannedHosts.length}`}
            </button>
          </div>
        </Modal>
      )}
      {importError && (
        <Modal title="Import SSH Config" onClose={closeImportModals} width={380}>
          <p className="text-sm text-muted">{importError}</p>
          <div className="mt-4 flex justify-end">
            <button onClick={closeImportModals} className="rounded-md bg-accent px-4 py-1.5 text-sm text-white hover:bg-accent-dim">
              OK
            </button>
          </div>
        </Modal>
      )}
      {importDone !== null && (
        <Modal title="Import SSH Config" onClose={closeImportModals} width={380}>
          <p className="text-sm text-muted">
            Imported {importDone} host{importDone === 1 ? '' : 's'} from ~/.ssh/config.
          </p>
          <div className="mt-4 flex justify-end">
            <button onClick={closeImportModals} className="rounded-md bg-accent px-4 py-1.5 text-sm text-white hover:bg-accent-dim">
              OK
            </button>
          </div>
        </Modal>
      )}

      {ctxMenu && (
        <HostContextMenu
          host={ctxMenu.host}
          x={ctxMenu.x}
          y={ctxMenu.y}
          onClose={() => setCtxMenu(null)}
        />
      )}
    </div>
  )
}
