package main

import (
	"fmt"
	"log"
	"strings"

	appssh "bastion/internal/ssh"

	"github.com/google/uuid"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// UploadCandidate is the frontend-facing preview of one file, returned by
// PrepareUpload. Directories are expanded into their contents so the confirm
// dialog lists every file that will be transferred.
type UploadCandidate struct {
	Path   string `json:"path"`             // absolute local path
	Name   string `json:"name"`             // display name (base name for top-level; rel-path within dir)
	Size   int64  `json:"size"`             // bytes
	Upload bool   `json:"upload"`           // false = skipped (non-regular / unreadable)
	Reason string `json:"reason,omitempty"` // why it was skipped, when Upload is false
}

// PrepareUploadResult carries the resolved default dest dir, the expanded
// file list, and the original paths the user selected. DestDir is editable by
// the user before confirming. Paths are returned so the caller can forward
// them to UploadFiles, which re-expands them (preserving directory structure).
type PrepareUploadResult struct {
	DestDir    string            `json:"destDir"`
	Candidates []UploadCandidate `json:"candidates"`
	Paths      []string          `json:"paths"`
}

// PrepareUpload expands dropped paths (files + directories) into a flat list
// of uploadable files. Directories are walked recursively; non-regular files
// inside them are skipped individually. The caller (confirm dialog) displays
// the candidates and on confirmation sends the original paths to UploadFiles.
func (a *App) PrepareUpload(sessionID string, paths []string) (PrepareUploadResult, error) {
	if !a.IsUnlocked() {
		return PrepareUploadResult{}, errLocked
	}
	defer a.touchAutoLock()

	// Collect per-path so one bad path doesn't break the whole preview.
	var allFiles []appssh.CollectedFile
	for _, p := range paths {
		files, err := appssh.CollectFiles([]string{p})
		if err != nil {
			continue
		}
		allFiles = append(allFiles, files...)
	}

	candidates := make([]UploadCandidate, len(allFiles))
	for i, f := range allFiles {
		candidates[i] = UploadCandidate{
			Path:   f.LocalPath,
			Name:   f.RemoteName,
			Size:   f.Size,
			Upload: true,
		}
	}

	return PrepareUploadResult{
		DestDir:    a.ResolveUploadDir(sessionID),
		Candidates: candidates,
		Paths:      paths,
	}, nil
}

// PickFilesForUpload opens the OS file chooser, then classifies the selected
// files exactly as PrepareUpload does. It is the primary entry point for the
// upload feature: native drag-and-drop is unreliable on Linux/WebKit2GTK (the
// webview opens dropped files instead of yielding their paths), so file
// selection goes through this picker. Returns an empty candidate list (no
// error) when the user cancels the dialog.
func (a *App) PickFilesForUpload(sessionID string) (PrepareUploadResult, error) {
	if !a.IsUnlocked() {
		return PrepareUploadResult{}, errLocked
	}
	defer a.touchAutoLock()

	paths, err := runtime.OpenMultipleFilesDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Select files to upload",
	})
	if err != nil {
		return PrepareUploadResult{}, fmt.Errorf("bastion: file picker: %w", err)
	}
	// Cancel yields no paths; return an empty (non-error) result so the UI can
	// simply do nothing.
	return a.PrepareUpload(sessionID, paths)
}

// ResolveUploadDir returns the best-known upload destination for a session: the
// shell's CWD if it has advertised one via OSC 7, otherwise the remote home
// directory (read from a fresh SFTP session). Returns "~" only as a last resort
// if the connection cannot be reached — the editable confirm dialog lets the
// user correct it either way.
func (a *App) ResolveUploadDir(sessionID string) string {
	if a.sessions == nil {
		return "~"
	}
	if cwd := a.sessions.CWD(sessionID); cwd != "" {
		return cwd
	}
	client, ok := a.sessions.Client(sessionID)
	if !ok {
		return "~"
	}
	dir, err := appssh.NewUploader(client).ResolveDefaultDir()
	if err != nil || dir == "" {
		return "~"
	}
	return dir
}

// validateDestDir rejects an empty or control-character-bearing destination
// path at the trust boundary, mirroring validateHostname in hosts.go.
func validateDestDir(dir string) error {
	if strings.TrimSpace(dir) == "" {
		return fmt.Errorf("bastion: destination directory must not be empty")
	}
	for i := 0; i < len(dir); i++ {
		if dir[i] < ' ' || dir[i] == 0x7f {
			return fmt.Errorf("bastion: destination directory contains control characters")
		}
	}
	return nil
}

// UploadFiles uploads the given local paths (files and/or directories) into
// destDir on the session's remote host over SFTP, reusing the live SSH
// connection. Directories are walked recursively. It returns a transfer ID
// immediately and runs the transfer concurrently in the background, emitting
// upload:progress:<id> events during and an upload:done:<id> event on
// completion. Existing remote files are overwritten.
func (a *App) UploadFiles(sessionID, destDir string, paths []string) (string, error) {
	if !a.IsUnlocked() {
		return "", errLocked
	}
	defer a.touchAutoLock()

	if err := validateDestDir(destDir); err != nil {
		return "", err
	}
	if len(paths) == 0 {
		return "", fmt.Errorf("bastion: no files to upload")
	}

	client, ok := a.sessions.Client(sessionID)
	if !ok {
		return "", fmt.Errorf("bastion: unknown or closed session %q", sessionID)
	}

	files, err := appssh.CollectFiles(paths)
	if err != nil {
		return "", fmt.Errorf("bastion: collect files: %w", err)
	}
	if len(files) == 0 {
		return "", fmt.Errorf("bastion: no uploadable files (non-regular files are skipped)")
	}

	transferID := uuid.NewString()
	uploader := appssh.NewUploader(client)

	go func() {
		log.Printf("[upload] %s: %d file(s) -> %s on session %s", transferID, len(files), destDir, sessionID)
		progress := func(p appssh.UploadProgress) {
			if a.emitter != nil {
				a.emitter.EmitUploadProgress(p)
			}
		}
		results, err := uploader.Upload(transferID, destDir, files, progress)
		if err != nil {
			results = make([]appssh.UploadFileResult, 0, len(files))
			for _, f := range files {
				results = append(results, appssh.UploadFileResult{Name: f.RemoteName, Error: err.Error()})
			}
		}
		if a.emitter != nil {
			a.emitter.EmitUploadDone(transferID, results)
		}
	}()

	return transferID, nil
}
