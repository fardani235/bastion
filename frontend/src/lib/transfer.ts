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

// ---------------------------------------------------------------------------
// Download
// ---------------------------------------------------------------------------

export interface RemoteEntry {
  name: string
  size: number
  isDir: boolean
  mode: number
  modTime: number
}

export interface DownloadCandidate {
  path: string
  name: string
  size: number
}

export interface PrepareDownloadResult {
  candidates: DownloadCandidate[]
  paths: string[]
}

export interface DownloadProgress {
  transferId: string
  fileIndex: number
  fileCount: number
  name: string
  bytes: number
  total: number
}

export interface DownloadFileResult {
  name: string
  bytes: number
  ok: boolean
  error?: string
}

export const listRemoteDir = (sessionId: string, path: string) =>
  App.ListRemoteDir(sessionId, path) as Promise<RemoteEntry[]>

export const prepareDownload = (sessionId: string, paths: string[]) =>
  App.PrepareDownload(sessionId, paths) as Promise<PrepareDownloadResult>

export const pickDownloadDestination = (sessionId: string) =>
  App.PickDownloadDestination(sessionId) as Promise<string>

export const downloadFiles = (sessionId: string, localDir: string, paths: string[]) =>
  App.DownloadFiles(sessionId, localDir, paths) as Promise<string>

export function subscribeDownload(
  transferId: string,
  handlers: {
    onProgress: (p: DownloadProgress) => void
    onDone: (results: DownloadFileResult[]) => void
  },
): () => void {
  const offProgress = EventsOn(`download:progress:${transferId}`, (p: DownloadProgress) => {
    handlers.onProgress(p)
  })
  const offDone = EventsOn(`download:done:${transferId}`, (results: DownloadFileResult[]) => {
    handlers.onDone(results ?? [])
  })
  return () => {
    offProgress()
    offDone()
  }
}
