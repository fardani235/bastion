import {create} from 'zustand'
import * as api from '../lib/api'
import * as aiApi from '../lib/ai'
import type {HostDTO, Group, Snippet, UnknownHostKeyInfo, ActiveForwardInfo, SessionHealthDTO} from '../lib/api'
import type {AIConfigStatus} from '../lib/ai'

// A pending trust prompt: the untrusted key plus the retry to run on accept.
export interface TrustPrompt {
  info: UnknownHostKeyInfo
  retry: () => Promise<void>
}

// A Tab is one open terminal. sessionId is empty until OpenSession succeeds;
// `status` drives the TerminalPane (connecting spinner, live terminal, or the
// disconnected overlay).
export interface Tab {
  tabId: string
  hostId: string
  title: string
  sessionId: string
  status: 'connecting' | 'connected' | 'disconnected'
  disconnectReason?: string
  fontSize?: number
}

interface AppState {
  // Server-mirrored data.
  hosts: HostDTO[]
  groups: Group[]
  snippets: Snippet[]

  // Client-only UI state.
  unlocked: boolean
  tabs: Tab[]
  activeTabId: string | null
  trustPrompt: TrustPrompt | null
  snippetsOpen: boolean
  activeForwards: ActiveForwardInfo[]
  sessionHealth: SessionHealthDTO

  // Data loading.
  refreshAll: () => Promise<void>
  refreshHosts: () => Promise<void>
  refreshGroups: () => Promise<void>
  refreshSnippets: () => Promise<void>
  refreshActiveForwards: () => Promise<void>
  refreshSessionHealth: () => Promise<void>

  // Auth state.
  setUnlocked: (v: boolean) => void

  // Tab management.
  addTab: (tab: Tab) => void
  updateTab: (tabId: string, patch: Partial<Tab>) => void
  removeTab: (tabId: string) => Promise<void>
  reorderTab: (fromIndex: number, toIndex: number) => void
  setActiveTab: (tabId: string) => void

  // Trust prompt.
  setTrustPrompt: (p: TrustPrompt | null) => void

  // Snippets drawer.
  toggleSnippets: () => void

  // AI drawer.
  aiOpen: boolean
  aiConfigured: boolean
  aiMessages: AIMessage[]
  toggleAI: () => void
  refreshAIConfig: () => Promise<void>
  addAIMessage: (msg: AIMessage) => void
  clearAIMessages: () => void
}

export interface AIMessage {
  id: string
  role: 'user' | 'assistant' | 'error' | 'suggestion'
  content: string
  command?: string    // injectable command for assistant/suggestion messages
  timestamp: number
}

export const useAppStore = create<AppState>((set, get) => ({
  hosts: [],
  groups: [],
  snippets: [],
  unlocked: false,
  tabs: [],
  activeTabId: null,
  trustPrompt: null,
  snippetsOpen: false,
  activeForwards: [],
  sessionHealth: {count: 0},

  refreshAll: async () => {
    const results = await Promise.allSettled([
      get().refreshHosts(), get().refreshGroups(),
      get().refreshSnippets(), get().refreshActiveForwards(),
      get().refreshSessionHealth(),
    ])
    for (const r of results) if (r.status === 'rejected') console.error('refreshAll:', r.reason)
  },
  refreshHosts: async () => { try { set({hosts: (await api.listHosts()) ?? []}) } catch (e) { console.error('refreshHosts:', e) } },
  refreshGroups: async () => { try { set({groups: (await api.listGroups()) ?? []}) } catch (e) { console.error('refreshGroups:', e) } },
  refreshSnippets: async () => { try { set({snippets: (await api.listSnippets()) ?? []}) } catch (e) { console.error('refreshSnippets:', e) } },
  refreshActiveForwards: async () => { try { set({activeForwards: (await api.listActiveForwards()) ?? []}) } catch (e) { console.error('refreshActiveForwards:', e) } },
  refreshSessionHealth: async () => { try { set({sessionHealth: (await api.listSessionHealth()) ?? {count: 0}}) } catch (e) { console.error('refreshSessionHealth:', e) } },

  setUnlocked: (v) => set({unlocked: v}),

  addTab: (tab) => set((s) => ({tabs: [...s.tabs, tab], activeTabId: tab.tabId})),
  updateTab: (tabId, patch) =>
    set((s) => ({tabs: s.tabs.map((t) => (t.tabId === tabId ? {...t, ...patch} : t))})),
  removeTab: async (tabId) => {
    const tab = get().tabs.find((t) => t.tabId === tabId)
    if (tab?.sessionId && tab.status === 'connected') {
      try { await api.closeSession(tab.sessionId) } catch { /* already gone */ }
    }
    set((s) => {
      const tabs = s.tabs.filter((t) => t.tabId !== tabId)
      let activeTabId = s.activeTabId
      if (activeTabId === tabId) {
        activeTabId = tabs.length ? tabs[tabs.length - 1].tabId : null
      }
      return {tabs, activeTabId}
    })
  },
  setActiveTab: (tabId) => set({activeTabId: tabId}),
  reorderTab: (fromIndex, toIndex) =>
    set((s) => {
      const tabs = [...s.tabs]
      const [moved] = tabs.splice(fromIndex, 1)
      tabs.splice(toIndex, 0, moved)
      return {tabs}
    }),

  setTrustPrompt: (p) => set({trustPrompt: p}),

  toggleSnippets: () => set((s) => ({snippetsOpen: !s.snippetsOpen, aiOpen: false})),

  // AI drawer.
  aiOpen: false,
  aiConfigured: false,
  aiMessages: [],
  toggleAI: () => set((s) => ({aiOpen: !s.aiOpen, snippetsOpen: false})),
  refreshAIConfig: async () => {
    try {
      const st = await aiApi.getAIConfigStatus()
      set({aiConfigured: st.configured})
    } catch {
      set({aiConfigured: false})
    }
  },
  addAIMessage: (msg) => set((s) => ({aiMessages: [...s.aiMessages, msg]})),
  clearAIMessages: () => set({aiMessages: []}),
}))
