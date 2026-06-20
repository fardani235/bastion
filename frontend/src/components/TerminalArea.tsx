import {useEffect, useState} from 'react'
import {OnFileDrop, OnFileDropOff} from '../../wailsjs/runtime/runtime'
import {useAppStore} from '../state/useAppStore'
import TerminalPane from './TerminalPane'
import TrustHostKeyModal from './TrustHostKeyModal'
import UploadModal from './UploadModal'
import * as transfer from '../lib/transfer'
import type {UploadCandidate} from '../lib/transfer'

// State for an in-progress drop: the active session plus the prepared candidates.
interface DropState {
  sessionId: string
  hostLabel: string
  destDir: string
  candidates: UploadCandidate[]
  paths: string[]
}

// TerminalArea hosts every open tab's TerminalPane. All panes stay mounted so
// scrollback survives tab switches; only the active one is visible. The trust
// prompt and file-upload modals render here, centered over the terminal region.
//
// File drops are handled at this level (not per-pane) because Wails' OnFileDrop
// is a single global window callback — registering it once and routing to the
// active connected session avoids panes clobbering each other's handler. The
// useDropTarget flag scopes drops to elements carrying the wails drop-target
// style, which we put on the terminal region below.
export default function TerminalArea() {
  const tabs = useAppStore((s) => s.tabs)
  const activeTabId = useAppStore((s) => s.activeTabId)
  const trustPrompt = useAppStore((s) => s.trustPrompt)
  const [drop, setDrop] = useState<DropState | null>(null)

  useEffect(() => {
    OnFileDrop((_x, _y, paths) => {
      if (!paths || paths.length === 0) return
      const state = useAppStore.getState()
      const tab = state.tabs.find((t) => t.tabId === state.activeTabId)
      if (!tab || tab.status !== 'connected' || !tab.sessionId) return
      void transfer
        .prepareUpload(tab.sessionId, paths)
        .then((res) => {
          if (res.candidates.length === 0) return
          setDrop({
            sessionId: tab.sessionId,
            hostLabel: tab.title,
            destDir: res.destDir,
            candidates: res.candidates,
            paths: res.paths,
          })
        })
        .catch((e) => console.error('prepareUpload:', e))
    }, true)
    return () => OnFileDropOff()
  }, [])

  return (
    <div className="relative min-h-0 flex-1 bg-bg" style={{'--wails-drop-target': 'drop'} as React.CSSProperties}>
      {tabs.length === 0 && (
        <div className="flex h-full items-center justify-center text-sm text-muted">
          Select a host to open a session.
        </div>
      )}
      {tabs.map((tab) => (
        <TerminalPane key={tab.tabId} tab={tab} visible={tab.tabId === activeTabId} onUpload={(sessionId, hostLabel, res) => {
          if (res.candidates.length === 0) return
          setDrop({sessionId, hostLabel, destDir: res.destDir, candidates: res.candidates, paths: res.paths})
        }} />
      ))}
      {trustPrompt && <TrustHostKeyModal />}
      {drop && (
        <UploadModal
          sessionId={drop.sessionId}
          hostLabel={drop.hostLabel}
          destDir={drop.destDir}
          candidates={drop.candidates}
          paths={drop.paths}
          onClose={() => setDrop(null)}
        />
      )}
    </div>
  )
}
