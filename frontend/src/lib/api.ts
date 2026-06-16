// Typed wrapper around the generated Wails bindings. Keeping all IPC behind one
// module means components import from here, not from the deep wailsjs paths, and
// gives us one place to adjust if the surface changes.
import * as App from '../../wailsjs/go/main/App'
import {main, store} from '../../wailsjs/go/models'

// fire runs a promise and logs any rejection instead of letting it become an
// unhandled promise rejection. Use for fire-and-forget IPC calls.
export function fire(p: Promise<unknown>, ctx?: string) {
  p.catch((e) => console.error(ctx ? `[${ctx}]` : '[fire]', e))
}

export type HostDTO = main.HostDTO
export type HostInput = main.HostInput
export type OpenSessionResult = main.OpenSessionResult
export type UnknownHostKeyInfo = main.UnknownHostKeyInfo
export type Group = store.Group
export type Snippet = store.Snippet

// --- Auth ---
export const isFirstRun = () => App.IsFirstRun()
export const isUnlocked = () => App.IsUnlocked()
export const setup = (password: string) => App.Setup(password)
export const unlock = (password: string) => App.Unlock(password)
export const lock = () => App.Lock()

// --- Hosts ---
export const listHosts = () => App.ListHosts()
export const createHost = (input: HostInput) => App.CreateHost(input)
export const updateHost = (id: string, input: HostInput) => App.UpdateHost(id, input)
export const deleteHost = (id: string) => App.DeleteHost(id)
export const setHostFontSize = (hostId: string, fontSize: number | null) => App.SetHostFontSize(hostId, fontSize)
export const importSSHConfig = () => App.ImportSSHConfig()
export const scanSSHConfig = () => App.ScanSSHConfig()

// --- Scanned SSH Host ---
export interface ScannedHost {
  label: string
  hostname: string
  port: number
  username: string
  identityFile: string
}

// --- Groups ---
export const listGroups = () => App.ListGroups()
export const createGroup = (name: string) => App.CreateGroup(name)
export const renameGroup = (id: string, name: string) => App.RenameGroup(id, name)
export const deleteGroup = (id: string) => App.DeleteGroup(id)

// --- Snippets ---
export const listSnippets = () => App.ListSnippets()
export const createSnippet = (label: string, body: string) => App.CreateSnippet(label, body)
export const updateSnippet = (id: string, label: string, body: string) =>
  App.UpdateSnippet(id, label, body)
export const deleteSnippet = (id: string) => App.DeleteSnippet(id)

// --- Port Forwards ---
export interface PortForwardDTO {
  id: string
  hostId: string
  label: string
  localPort: number
  remoteHost: string
  remotePort: number
  enabled: boolean
  createdAt: number
}
export interface ActiveForwardInfo {
  id: string
  localPort: number
  remoteHost: string
  remotePort: number
  activeConns: number
  totalConns: number
}
export const listPortForwards = (hostId: string) => App.ListPortForwards(hostId)
export const createPortForward = (hostId: string, label: string, localPort: number, remoteHost: string, remotePort: number, enabled: boolean) =>
  App.CreatePortForward(hostId, label, localPort, remoteHost, remotePort, enabled)
export const updatePortForward = (id: string, hostId: string, label: string, localPort: number, remoteHost: string, remotePort: number, enabled: boolean) =>
  App.UpdatePortForward(id, hostId, label, localPort, remoteHost, remotePort, enabled)
export const deletePortForward = (id: string) => App.DeletePortForward(id)
export const listActiveForwards = () => App.ListActiveForwards()

// --- Sessions ---
export const openSession = (hostId: string, cols: number, rows: number) =>
  App.OpenSession(hostId, cols, rows)
export const trustHostKey = (hostname: string, port: number, keyType: string, base64Key: string) =>
  App.TrustHostKey(hostname, port, keyType, base64Key)
export const writeToSession = (sessionId: string, data: string) =>
  App.WriteToSession(sessionId, data)
export const resizeSession = (sessionId: string, cols: number, rows: number) =>
  App.ResizeSession(sessionId, cols, rows)
export const closeSession = (sessionId: string) => App.CloseSession(sessionId)
export const listSessionHealth = () => App.ListSessionHealth()

// --- Session Health ---
export interface SessionInfo {
  id: string
  startedAt: string
  uptime: string
}
export interface SessionHealthDTO {
  count: number
  uptimes?: SessionInfo[]
}
