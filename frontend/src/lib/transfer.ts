// Typed wrappers for the file-upload (SFTP) IPC, plus subscription helpers for
// the upload:progress / upload:done events. File bytes never cross this
// boundary — Go reads the local files directly; only paths, sizes, and progress
// counters travel over IPC.
import * as App from '../../wailsjs/go/main/App'
import {EventsOn} from '../../wailsjs/runtime/runtime'

export interface UploadCandidate {
  path: string
  name: string
  size: number
  upload: boolean
  reason?: string
}

export interface PrepareUploadResult {
  destDir: string
  candidates: UploadCandidate[]
  paths: string[]
}

export interface UploadProgress {
  transferId: string
  fileIndex: number
  fileCount: number
  name: string
  bytes: number
  total: number
}

export interface UploadFileResult {
  name: string
  bytes: number
  ok: boolean
  error?: string
}

export const prepareUpload = (sessionId: string, paths: string[]) =>
  App.PrepareUpload(sessionId, paths) as Promise<PrepareUploadResult>

export const pickFilesForUpload = (sessionId: string) =>
  App.PickFilesForUpload(sessionId) as Promise<PrepareUploadResult>

export const pickFolderForUpload = (sessionId: string) =>
  App.PickFolderForUpload(sessionId) as Promise<PrepareUploadResult>

export const uploadFiles = (sessionId: string, destDir: string, paths: string[]) =>
  App.UploadFiles(sessionId, destDir, paths) as Promise<string>

/**
 * subscribeUpload wires progress + done handlers for one transfer and returns
 * an unsubscribe function. The done handler fires once; callers typically
 * unsubscribe from within it.
 */
export function subscribeUpload(
  transferId: string,
  handlers: {
    onProgress: (p: UploadProgress) => void
    onDone: (results: UploadFileResult[]) => void
  },
): () => void {
  const offProgress = EventsOn(`upload:progress:${transferId}`, (p: UploadProgress) => {
    handlers.onProgress(p)
  })
  const offDone = EventsOn(`upload:done:${transferId}`, (results: UploadFileResult[]) => {
    handlers.onDone(results ?? [])
  })
  return () => {
    offProgress()
    offDone()
  }
}
