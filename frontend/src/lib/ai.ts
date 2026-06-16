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

export interface AIConfig {
  provider: string       // "openai", "anthropic", "openai-compatible"
  model: string
  endpoint?: string      // custom endpoint for openai-compatible
  apiKey: string
  systemPrompt?: string  // custom system prompt; empty = use default
}

export interface AIConfigStatus {
  configured: boolean
}

export const generateCommand = (sessionId: string, prompt: string) =>
  App.GenerateCommand(sessionId, prompt) as Promise<AICommandResult>

export const explainError = (sessionId: string, output: string) =>
  App.ExplainError(sessionId, output) as Promise<AIErrorResult>

export const setAIConfig = (provider: string, model: string, endpoint: string, apiKey: string, systemPrompt?: string) =>
  App.SetAIConfig(provider, model, endpoint, apiKey, systemPrompt ?? '') as Promise<void>

export const getAIConfig = () =>
  App.GetAIConfig() as Promise<AIConfig>

export const getAIConfigStatus = () =>
  App.GetAIConfigStatus() as Promise<AIConfigStatus>

export const testAIConnection = () =>
  App.TestAIConnection() as Promise<void>
