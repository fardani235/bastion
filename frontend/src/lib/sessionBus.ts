// sessionBus subscribes to the Go-side session:* events and fans them out to
// per-session listeners. Output arrives base64-encoded (raw PTY bytes); we
// decode to a Uint8Array so the terminal can write the exact bytes verbatim.
import {EventsOn} from '../../wailsjs/runtime/runtime'

type OutputHandler = (bytes: Uint8Array) => void
type ClosedHandler = (reason: string) => void

interface Listeners {
  output?: OutputHandler
  closed?: ClosedHandler
}

const listeners = new Map<string, Listeners>()
// Track which event names we've already wired so we don't double-subscribe.
const wired = new Set<string>()
const unsubscribers = new Map<string, () => void>()

function b64ToBytes(b64: string): Uint8Array {
  const bin = atob(b64)
  const out = new Uint8Array(bin.length)
  for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i)
  return out
}

function ensureWired(sessionId: string) {
  if (wired.has(sessionId)) return

  const offOut = EventsOn(`session:output:${sessionId}`, (payload: string) => {
    try {
      listeners.get(sessionId)?.output?.(b64ToBytes(payload))
    } catch (e) {
      console.error('sessionBus: invalid payload for session', sessionId, e)
    }
  })
  const offClosed = EventsOn(`session:closed:${sessionId}`, (reason: string) => {
    listeners.get(sessionId)?.closed?.(reason)
  })
  unsubscribers.set(sessionId, () => {
    offOut()
    offClosed()
  })
  wired.add(sessionId)
}

/** subscribe registers handlers for one session and returns an unsubscribe fn. */
export function subscribe(
  sessionId: string,
  handlers: {onOutput: OutputHandler; onClosed: ClosedHandler},
): () => void {
  listeners.set(sessionId, {output: handlers.onOutput, closed: handlers.onClosed})
  ensureWired(sessionId)

  return () => {
    listeners.delete(sessionId)
    unsubscribers.get(sessionId)?.()
    unsubscribers.delete(sessionId)
    wired.delete(sessionId)
  }
}
