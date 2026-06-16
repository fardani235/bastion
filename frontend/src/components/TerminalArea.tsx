import {useAppStore} from '../state/useAppStore'
import TerminalPane from './TerminalPane'
import TrustHostKeyModal from './TrustHostKeyModal'

// TerminalArea hosts every open tab's TerminalPane. All panes stay mounted so
// scrollback survives tab switches; only the active one is visible. The trust
// prompt modal renders here, centered over the terminal region.
export default function TerminalArea() {
  const tabs = useAppStore((s) => s.tabs)
  const activeTabId = useAppStore((s) => s.activeTabId)
  const trustPrompt = useAppStore((s) => s.trustPrompt)

  return (
    <div className="relative min-h-0 flex-1 bg-bg">
      {tabs.length === 0 && (
        <div className="flex h-full items-center justify-center text-sm text-muted">
          Select a host to open a session.
        </div>
      )}
      {tabs.map((tab) => (
        <TerminalPane key={tab.tabId} tab={tab} visible={tab.tabId === activeTabId} />
      ))}
      {trustPrompt && <TrustHostKeyModal />}
    </div>
  )
}
