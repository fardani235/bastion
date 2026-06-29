import {useCallback, useEffect, useRef, useState} from 'react'
import {useAppStore} from '../state/useAppStore'
import type {AIMessage} from '../state/useAppStore'
import * as aiApi from '../lib/ai'
import * as api from '../lib/api'
import AISettingsModal from './AISettingsModal'

let msgCounter = 0
function nextId() {
  msgCounter++
  return `ai-${Date.now()}-${msgCounter}`
}

// AIDrawer slides in from the right with a WhatsApp-style chat interface for
// AI command generation and error suggestions. Conversations are stateful on
// the backend (LangChain ConversationBuffer memory) so the AI understands
// context across messages.
export default function AIDrawer() {
  const open = useAppStore((s) => s.aiOpen)
  const toggle = useAppStore((s) => s.toggleAI)
  const messages = useAppStore((s) => s.aiMessages)
  const addMessage = useAppStore((s) => s.addAIMessage)
  const clearMessages = useAppStore((s) => s.clearAIMessages)
  const aiConfigured = useAppStore((s) => s.aiConfigured)
  const refreshAIConfig = useAppStore((s) => s.refreshAIConfig)
  const aiChatId = useAppStore((s) => s.aiChatId)
  const setAIChatId = useAppStore((s) => s.setAIChatId)
  const tabs = useAppStore((s) => s.tabs)
  const activeTabId = useAppStore((s) => s.activeTabId)

  const [input, setInput] = useState('')
  const [busy, setBusy] = useState(false)
  const [settingsOpen, setSettingsOpen] = useState(false)
  const messagesEndRef = useRef<HTMLDivElement>(null)

  const activeTab = tabs.find((t) => t.tabId === activeTabId)

  // Refresh AI config and create a backend chat session when drawer opens.
  const initChat = useCallback(async () => {
    await refreshAIConfig()
    if (useAppStore.getState().aiConfigured) {
      try {
        const id = await aiApi.newChat()
        setAIChatId(id)
      } catch {
        // Not configured — leave defaults.
      }
    }
  }, [refreshAIConfig, setAIChatId])

  useEffect(() => {
    if (open) void initChat()
  }, [open, initChat])

  // Auto-scroll to bottom when new messages arrive.
  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({behavior: 'smooth'})
  }, [messages])

  if (!open) return null

  async function handleSend() {
    const text = input.trim()
    if (!text || busy) return
    setInput('')

    const userMsg: AIMessage = {
      id: nextId(),
      role: 'user',
      content: text,
      timestamp: Date.now(),
    }
    addMessage(userMsg)
    setBusy(true)

    try {
      const result = await aiApi.chat(aiChatId!, text)
      const reply: AIMessage = {
        id: nextId(),
        role: 'assistant',
        content: result.reply,
        command: result.command || undefined,
        timestamp: Date.now(),
      }
      addMessage(reply)
    } catch (e) {
      const errMsg: AIMessage = {
        id: nextId(),
        role: 'error',
        content: e instanceof Error ? e.message : 'Request failed',
        timestamp: Date.now(),
      }
      addMessage(errMsg)
    } finally {
      setBusy(false)
    }
  }

  function handleKeyDown(e: React.KeyboardEvent) {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      void handleSend()
    }
  }

  async function handleClear() {
    if (aiChatId) {
      try {
        await aiApi.clearChat(aiChatId)
      } catch {
        // Session may already be gone.
      }
    }
    clearMessages()
    try {
      const id = await aiApi.newChat()
      setAIChatId(id)
    } catch {
      setAIChatId(null)
    }
  }

  async function handleInject(command: string) {
    if (!activeTab?.sessionId) return
    try {
      await api.writeToSession(activeTab.sessionId, command + '\n')
    } catch (e) {
      console.error('inject command:', e)
    }
  }

  return (
    <>
      <div className="flex w-[360px] shrink-0 flex-col border-l border-border bg-surface">
        {/* Header */}
        <div className="flex items-center justify-between px-3 py-3">
          <span className="text-sm font-semibold text-text">AI</span>
          <div className="flex gap-2 text-xs">
            <button
              onClick={() => setSettingsOpen(true)}
              className="text-muted hover:text-text"
              title="AI settings"
            >
              Settings
            </button>
            <button onClick={handleClear} className="text-muted hover:text-text" title="Clear conversation">
              Clear
            </button>
            <button onClick={toggle} className="text-muted hover:text-text" title="Close">
              ✕
            </button>
          </div>
        </div>

        {/* Messages */}
        <div className="flex-1 overflow-y-auto px-3 py-2 space-y-3">
          {messages.length === 0 && (
            <div className="py-8 text-center text-xs text-muted/60">
              {aiConfigured
                ? 'Describe a command you want to run.\nE.g. "find all files larger than 100MB"'
                : 'Configure an AI provider in Settings\nto enable command generation.'}
            </div>
          )}
          {messages.map((m) => (
            <div key={m.id} className={`flex ${m.role === 'user' ? 'justify-end' : 'justify-start'}`}>
              <div
                className={`max-w-[85%] rounded-lg px-3 py-2 text-sm ${
                  m.role === 'user'
                    ? 'bg-accent text-bg'
                    : m.role === 'error'
                      ? 'bg-danger/10 text-danger'
                      : 'bg-surface-2 text-text'
                }`}
              >
                <div className="whitespace-pre-wrap break-words">{m.content}</div>
                {m.command && (
                  <button
                    onClick={() => handleInject(m.command!)}
                    className="mt-2 rounded border border-border bg-surface px-2 py-0.5 text-xs text-accent hover:bg-surface-2"
                    title="Inject into active session"
                  >
                    Inject
                  </button>
                )}
              </div>
            </div>
          ))}
          {busy && (
            <div className="flex justify-start">
              <div className="rounded-lg bg-surface-2 px-3 py-2 text-sm text-muted">
                <span className="inline-flex gap-1">
                  <span className="animate-pulse">·</span>
                  <span className="animate-pulse delay-150">·</span>
                  <span className="animate-pulse delay-300">·</span>
                </span>
              </div>
            </div>
          )}
          <div ref={messagesEndRef} />
        </div>

        {/* Input */}
        <div className="border-t border-border px-3 py-2">
          <div className="flex gap-2">
            <input
              value={input}
              onChange={(e) => setInput(e.target.value)}
              onKeyDown={handleKeyDown}
              placeholder={aiConfigured ? 'Describe a command…' : 'Configure AI first…'}
              disabled={busy || !aiConfigured}
              className="min-w-0 flex-1 rounded-md border border-border bg-surface-2 px-3 py-2 text-sm text-text outline-none placeholder:text-muted/50 focus:border-accent disabled:opacity-50"
            />
            <button
              onClick={() => void handleSend()}
              disabled={busy || !input.trim() || !aiConfigured}
              className="rounded-md bg-accent px-3 py-2 text-sm text-white hover:bg-accent/80 disabled:opacity-50"
            >
              Send
            </button>
          </div>
        </div>
      </div>

      {settingsOpen && <AISettingsModal onClose={() => setSettingsOpen(false)} />}
    </>
  )
}
