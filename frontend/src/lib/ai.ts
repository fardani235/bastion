// Typed wrappers for the AI-related Go bindings. All methods require an
// unlocked vault and will fail if the vault is locked.
import * as App from '../../wailsjs/go/main/App'

export interface AICommandResult {
  command: string
  explanation: string
}

export interface AIErrorResult {
  explanation: string
  fixCommand?: string
}

// AIConfigView is the renderer-facing view of the AI config. It deliberately
// carries NO API key — the backend sends a hasKey flag instead (mirroring the
// host-credential boundary). The key travels renderer->Go only, via setAIConfig.
export interface AIConfigView {
  provider: string          // "openai", "anthropic", "openai-compatible"
  model: string
  endpoint?: string         // custom endpoint for openai-compatible
  systemPrompt?: string     // custom system prompt; empty = use default
  hasKey: boolean           // whether a key is stored (never the key itself)
  autoExplainErrors: boolean // auto-send terminal output to the AI on errors
}

export interface AIConfigStatus {
  configured: boolean
  autoExplainErrors: boolean
}

export const generateCommand = (sessionId: string, prompt: string) =>
  App.GenerateCommand(sessionId, prompt) as Promise<AICommandResult>

export const explainError = (sessionId: string, output: string) =>
  App.ExplainError(sessionId, output) as Promise<AIErrorResult>

export const setAIConfig = (provider: string, model: string, endpoint: string, apiKey: string, systemPrompt?: string, autoExplainErrors?: boolean) =>
  App.SetAIConfig(provider, model, endpoint, apiKey, systemPrompt ?? '', autoExplainErrors ?? false) as Promise<void>

export const getAIConfig = () =>
  App.GetAIConfig() as Promise<AIConfigView>

export const getAIConfigStatus = () =>
  App.GetAIConfigStatus() as Promise<AIConfigStatus>

export const testAIConnection = () =>
  App.TestAIConnection() as Promise<void>
