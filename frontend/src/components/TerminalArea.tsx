import {useState} from 'react'
import {useAppStore} from '../state/useAppStore'
import TerminalPane from './TerminalPane'
import TrustHostKeyModal from './TrustHostKeyModal'
import UploadModal from './UploadModal'
import type {UploadCandidate} from '../lib/transfer'

// A pending upload: the active session plus the prepared candidates/paths from
// the file or folder picker, shown in the UploadModal.
interface PendingUpload {
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
// File upload is initiated from a TerminalPane's context menu (Upload Files /
// Upload Folder), which opens the native OS picker and calls back with the
// prepared result. Native drag-and-drop is intentionally not used: on
// Linux/WebKit2GTK a file dropped on the webview is opened by the webview
// instead of yielding its path (see main.go DisableWebViewDrop).
export default function TerminalArea() {
  const tabs = useAppStore((s) => s.tabs)
  const activeTabId = useAppStore((s) => s.activeTabId)
  const trustPrompt = useAppStore((s) => s.trustPrompt)
  const [upload, setUpload] = useState<PendingUpload | null>(null)

  return (
    <div className="relative min-h-0 flex-1 bg-bg">
      {tabs.length === 0 && (
        <div className="flex h-full items-center justify-center text-sm text-muted">
          Select a host to open a session.
        </div>
      )}
      {tabs.map((tab) => (
        <TerminalPane
          key={tab.tabId}
          tab={tab}
          visible={tab.tabId === activeTabId}
          onUpload={(sessionId, hostLabel, res) => {
            if (res.candidates.length === 0) return
            setUpload({sessionId, hostLabel, destDir: res.destDir, candidates: res.candidates, paths: res.paths})
          }}
        />
      ))}
      {trustPrompt && <TrustHostKeyModal />}
      {upload && (
        <UploadModal
          sessionId={upload.sessionId}
          hostLabel={upload.hostLabel}
          destDir={upload.destDir}
          candidates={upload.candidates}
          paths={upload.paths}
          onClose={() => setUpload(null)}
        />
      )}
    </div>
  )
}
