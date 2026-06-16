import {useEffect} from 'react'
import {useAppStore} from '../state/useAppStore'
import Sidebar from '../components/Sidebar'
import TabBar from '../components/TabBar'
import TerminalArea from '../components/TerminalArea'
import StatusBar from '../components/StatusBar'
import SnippetsDrawer from '../components/SnippetsDrawer'
import AIDrawer from '../components/AIDrawer'

// Main is the unlocked application shell: a fixed-width sidebar on the left and
// the tab bar + terminal area + status bar stacked on the right.
export default function Main({onLock}: {onLock: () => void}) {
  const refreshAll = useAppStore((s) => s.refreshAll)

  useEffect(() => {
    void refreshAll()
  }, [refreshAll])

  return (
    <div className="flex h-full bg-bg text-text">
      <Sidebar />
      <div className="flex min-w-0 flex-1 flex-col">
        <TabBar />
        <TerminalArea />
        <StatusBar onLock={onLock} />
      </div>
      <SnippetsDrawer />
      <AIDrawer />
    </div>
  )
}
