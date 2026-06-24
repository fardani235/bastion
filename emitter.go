package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	appssh "bastion/internal/ssh"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// wailsEmitter implements ssh.Emitter by publishing Wails events. PTY output is
// base64-encoded so arbitrary bytes survive the JSON event channel; the
// frontend decodes and writes them verbatim to xterm.js.
//
// Event names match spec §4:
//   - session:output:<id>  payload: base64 string
//   - session:closed:<id>  payload: reason string
//
// It also optionally logs all session output to ~/.config/bastion/logs/.
type wailsEmitter struct {
	ctx    context.Context
	logDir string

	mu          sync.Mutex
	sessionLogs map[string]*os.File
}

func newWailsEmitter(ctx context.Context) *wailsEmitter {
	return &wailsEmitter{
		ctx:         ctx,
		sessionLogs: make(map[string]*os.File),
	}
}

func (e *wailsEmitter) EmitOutput(sessionID string, chunk []byte) {
	runtime.EventsEmit(e.ctx, "session:output:"+sessionID,
		base64.StdEncoding.EncodeToString(chunk))
	e.writeLog(sessionID, chunk)
}

func (e *wailsEmitter) EmitClosed(sessionID string, reason string) {
	runtime.EventsEmit(e.ctx, fmt.Sprintf("session:closed:%s", sessionID), reason)
	e.closeLog(sessionID)
}

// EmitUploadProgress publishes a single file-upload progress update. The
// payload is the appssh.UploadProgress struct, JSON-serialized by Wails.
func (e *wailsEmitter) EmitUploadProgress(p appssh.UploadProgress) {
	runtime.EventsEmit(e.ctx, "upload:progress:"+p.TransferID, p)
}

// EmitUploadDone publishes the final per-file result list for a transfer.
func (e *wailsEmitter) EmitUploadDone(transferID string, results []appssh.UploadFileResult) {
	runtime.EventsEmit(e.ctx, "upload:done:"+transferID, results)
}

func (e *wailsEmitter) EmitDownloadProgress(p appssh.DownloadProgress) {
	runtime.EventsEmit(e.ctx, "download:progress:"+p.TransferID, p)
}

func (e *wailsEmitter) EmitDownloadDone(transferID string, results []appssh.DownloadFileResult) {
	runtime.EventsEmit(e.ctx, "download:done:"+transferID, results)
}

// OpenLog creates a log file for the given session and starts writing output.
func (e *wailsEmitter) OpenLog(sessionID, label string) error {
	if e.logDir == "" {
		return nil
	}
	if err := os.MkdirAll(e.logDir, 0700); err != nil {
		return fmt.Errorf("emitter: mkdir log dir: %w", err)
	}
	ts := time.Now().Format("20060102-150405")
	name := fmt.Sprintf("%s-%s.log", sanitize(label), ts)
	path := filepath.Join(e.logDir, name)
	// 0600: session logs may contain secrets typed at the terminal, so they must
	// not be world- or group-readable. os.Create would use 0666&umask (~0644).
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("emitter: create log: %w", err)
	}
	e.mu.Lock()
	e.sessionLogs[sessionID] = f
	e.mu.Unlock()
	return nil
}

func (e *wailsEmitter) writeLog(sessionID string, chunk []byte) {
	e.mu.Lock()
	f, ok := e.sessionLogs[sessionID]
	e.mu.Unlock()
	if !ok {
		return
	}
	_, _ = f.Write(chunk)
}

func (e *wailsEmitter) closeLog(sessionID string) {
	e.mu.Lock()
	f, ok := e.sessionLogs[sessionID]
	delete(e.sessionLogs, sessionID)
	e.mu.Unlock()
	if ok {
		_, _ = f.Write([]byte("\n--- session closed ---\n"))
		_ = f.Close()
	}
}

func sanitize(name string) string {
	// Whitelist: only allow alphanumeric, dot, hyphen, underscore.
	// Strip everything else to prevent path traversal and filesystem abuse.
	b := make([]byte, 0, len(name))
	for i := 0; i < len(name); i++ {
		c := name[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '.' || c == '-' || c == '_' {
			b = append(b, c)
		} else {
			b = append(b, '_')
		}
	}
	if len(b) == 0 {
		return "session"
	}
	return string(b)
}
