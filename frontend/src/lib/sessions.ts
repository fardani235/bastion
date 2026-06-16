// openHostSession orchestrates the connect flow shared by the sidebar and the
// reconnect button: create a tab, call OpenSession, and either attach the
// returned sessionId or surface a trust prompt for the UI to resolve.
import {nanoid} from './nanoid'
import * as api from './api'
import type {HostDTO, UnknownHostKeyInfo} from './api'
import {useAppStore} from '../state/useAppStore'

// Default geometry until the real terminal mounts and reports its size.
const DEFAULT_COLS = 80
const DEFAULT_ROWS = 24

export interface ConnectCallbacks {
  // Called when the server key is untrusted; the UI shows a prompt and, on
  // accept, calls trustAndRetry.
  onTrustPrompt: (info: UnknownHostKeyInfo, retry: () => Promise<void>) => void
}

export async function openHostSession(host: HostDTO, cb: ConnectCallbacks): Promise<void> {
  const store = useAppStore.getState()
  const tabId = nanoid()
  store.addTab({
    tabId,
    hostId: host.id,
    title: host.label,
    sessionId: '',
    status: 'connecting',
    fontSize: host.fontSize ?? 13,
  })

  const attempt = async (afterTrust?: boolean) => {
    try {
      const res = await api.openSession(host.id, DEFAULT_COLS, DEFAULT_ROWS)
      if (res?.unknownHostKey) {
        if (afterTrust) {
          store.updateTab(tabId, {status: 'disconnected', disconnectReason: 'Host key still unknown after trust'})
          return
        }
        const info = res.unknownHostKey
        cb.onTrustPrompt(info, async () => {
          await api.trustHostKey(info.hostname, info.port, info.keyType, info.base64Key)
          await attempt(true)
        })
        return
      }
      if (res?.sessionId) {
        store.updateTab(tabId, {sessionId: res.sessionId, status: 'connected'})
      }
    } catch (e) {
      store.updateTab(tabId, {status: 'disconnected', disconnectReason: String(e)})
    }
  }

  await attempt()
}
