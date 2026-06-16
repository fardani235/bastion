import {useEffect, useState} from 'react'
import {EventsOn} from '../wailsjs/runtime/runtime'
import * as api from './lib/api'
import {useAppStore} from './state/useAppStore'
import Setup from './routes/Setup'
import Unlock from './routes/Unlock'
import Main from './routes/Main'

type Screen = 'loading' | 'setup' | 'unlock' | 'main'

function App() {
  const [screen, setScreen] = useState<Screen>('loading')
  const setUnlocked = useAppStore((s) => s.setUnlocked)

  // On boot, decide the initial screen from vault state.
  useEffect(() => {
    void (async () => {
      try {
        const firstRun = await api.isFirstRun()
        setScreen(firstRun ? 'setup' : 'unlock')
      } catch (e) {
        console.error('bootstrap:', e)
        setScreen('unlock')
      }
    })()

    // Listen for auto-lock from the Go side.
    const unsub = EventsOn('vault:locked', () => {
      setUnlocked(false)
      setScreen('unlock')
    })
    return unsub
  }, [])

  function enterMain() {
    setUnlocked(true)
    setScreen('main')
  }

  async function lock() {
    await api.lock()
    setUnlocked(false)
    setScreen('unlock')
  }

  switch (screen) {
    case 'loading':
      return <div className="h-full bg-bg" />
    case 'setup':
      return <Setup onDone={enterMain} />
    case 'unlock':
      return <Unlock onDone={enterMain} />
    case 'main':
      return <Main onLock={lock} />
  }
}

export default App
