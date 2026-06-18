import {useEffect, useState} from 'react'
import Modal, {Field, inputClass, CustomSelect} from './Modal'
import * as aiApi from '../lib/ai'
import {useAppStore} from '../state/useAppStore'

// AISettingsModal lets the user configure the AI provider and API key.
export default function AISettingsModal({onClose}: {onClose: () => void}) {
  const refreshAIConfig = useAppStore((s) => s.refreshAIConfig)

  const [provider, setProvider] = useState('openai')
  const [model, setModel] = useState('gpt-4o')
  const [endpoint, setEndpoint] = useState('')
  const [apiKey, setApiKey] = useState('')
  const [hasKey, setHasKey] = useState(false)
  const [systemPrompt, setSystemPrompt] = useState('')
  const [autoExplainErrors, setAutoExplainErrors] = useState(false)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [testing, setTesting] = useState(false)
  const [testResult, setTestResult] = useState<'ok' | 'fail' | null>(null)
  const [error, setError] = useState('')

  // Load existing config on mount.
  useEffect(() => {
    const load = async () => {
      try {
        const cfg = await aiApi.getAIConfig()
        setAutoExplainErrors(cfg.autoExplainErrors)
        if (cfg.hasKey) {
          setProvider(cfg.provider)
          setModel(cfg.model)
          setEndpoint(cfg.endpoint || '')
          setHasKey(true)
          // The key itself is never sent to the renderer. Leave the field blank;
          // a blank key on save means "keep the stored key".
          setSystemPrompt(cfg.systemPrompt || '')
        }
      } catch {
        // Not configured yet — leave defaults.
      } finally {
        setLoading(false)
      }
    }
    void load()
  }, [])

  async function handleSave() {
    if (!apiKey.trim() && !hasKey) {
      setError('API key is required')
      return
    }
    setSaving(true)
    setError('')
    try {
      await aiApi.setAIConfig(provider, model, endpoint, apiKey, systemPrompt, autoExplainErrors)
      await refreshAIConfig()
      onClose()
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to save')
    } finally {
      setSaving(false)
    }
  }

  async function handleTest() {
    if (!apiKey.trim() && !hasKey) {
      setError('API key is required')
      return
    }
    setTesting(true)
    setTestResult(null)
    setError('')
    try {
      await aiApi.setAIConfig(provider, model, endpoint, apiKey, systemPrompt, autoExplainErrors)
      setError('Config saved, testing connection...')
    } catch (e) {
      setTestResult('fail')
      setError('Save failed: ' + (typeof e === 'string' ? e : e instanceof Error ? e.message : String(e)))
      setTesting(false)
      return
    }
    try {
      await aiApi.testAIConnection()
      setTestResult('ok')
      setError('')
    } catch (e) {
      console.error('test connection error:', e)
      setTestResult('fail')
      setError('API error: ' + (typeof e === 'string' ? e : e instanceof Error ? e.message : JSON.stringify(e)))
    } finally {
      setTesting(false)
    }
  }

  const showEndpoint = provider === 'openai-compatible'

  return (
    <Modal title="AI Settings" onClose={onClose} width={460}>
      {loading ? (
        <div className="py-4 text-center text-sm text-muted">Loading…</div>
      ) : (
        <div className="space-y-4">
          <Field label="Provider">
            <CustomSelect
              value={provider}
              onChange={setProvider}
              options={[
                {value: 'openai', label: 'OpenAI'},
                {value: 'anthropic', label: 'Anthropic'},
                {value: 'openai-compatible', label: 'OpenAI-compatible (Ollama, etc.)'},
              ]}
            />
          </Field>

          <Field label="Model">
            <input
              value={model}
              onChange={(e) => setModel(e.target.value)}
              placeholder={
                provider === 'openai' ? 'gpt-4o' :
                provider === 'anthropic' ? 'claude-sonnet-4-20250514' :
                'llama3.2'
              }
              className={inputClass}
            />
          </Field>

          {showEndpoint && (
            <Field label="API Endpoint">
              <input
                value={endpoint}
                onChange={(e) => setEndpoint(e.target.value)}
                placeholder="http://localhost:11434/v1"
                className={inputClass}
              />
            </Field>
          )}

          <Field label="API Key">
            <input
              type="password"
              value={apiKey}
              onChange={(e) => setApiKey(e.target.value)}
              placeholder={hasKey ? '•••••••• (leave blank to keep current key)' : 'sk-…'}
              className={inputClass}
            />
          </Field>

          <Field label="System Prompt (optional)">
            <div className="space-y-1">
              <textarea
                value={systemPrompt}
                onChange={(e) => setSystemPrompt(e.target.value)}
                placeholder="Leave empty to use the default prompt for command generation and error explanation."
                rows={5}
                className={`${inputClass} resize-y font-mono text-xs`}
              />
              <button
                type="button"
                onClick={() => setSystemPrompt('')}
                className="text-xs text-muted hover:text-text"
              >
                Reset to default
              </button>
            </div>
          </Field>

          <label className="flex cursor-pointer items-start gap-2">
            <input
              type="checkbox"
              checked={autoExplainErrors}
              onChange={(e) => setAutoExplainErrors(e.target.checked)}
              className="mt-0.5"
            />
            <span className="text-sm text-text">
              Auto-explain errors
              <span className="mt-0.5 block text-xs text-muted">
                When a command fails, automatically send the terminal output to your
                AI provider for an explanation. Off by default: this transmits screen
                contents (which may include hostnames, file contents, or secrets) to a
                third party without asking each time.
              </span>
            </span>
          </label>

          {testResult === 'ok' && (
            <p className="text-xs text-accent">Connection successful ✓</p>
          )}
          {testResult === 'fail' && (
            <p className="text-xs text-danger">{error || 'Connection failed — check your settings'}</p>
          )}
          {!testResult && error && <p className="text-xs text-danger">{error}</p>}

          <div className="flex gap-2 pt-2">
            <button
              onClick={handleTest}
              disabled={testing || saving}
              className="rounded-md border border-border bg-surface-2 px-3 py-2 text-sm text-text hover:bg-surface disabled:opacity-50"
            >
              {testing ? 'Testing…' : 'Test connection'}
            </button>
            <div className="flex-1" />
            <button
              onClick={onClose}
              className="rounded-md border border-border bg-surface-2 px-3 py-2 text-sm text-text hover:bg-surface"
            >
              Cancel
            </button>
            <button
              onClick={handleSave}
              disabled={saving || testing}
              className="rounded-md bg-accent px-3 py-2 text-sm text-white hover:bg-accent/80 disabled:opacity-50"
            >
              {saving ? 'Saving…' : 'Save'}
            </button>
          </div>
        </div>
      )}
    </Modal>
  )
}
