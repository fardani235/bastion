package ssh

import (
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/pkg/sftp"
	gossh "golang.org/x/crypto/ssh"
)

const progressInterval = 64 * 1024

var defaultConcurrency = clamp(runtime.NumCPU(), 1, 4)

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

type UploadProgress struct {
	TransferID string `json:"transferId"`
	FileIndex  int    `json:"fileIndex"`
	FileCount  int    `json:"fileCount"`
	Name       string `json:"name"`
	Bytes      int64  `json:"bytes"`
	Total      int64  `json:"total"`
}

type UploadFileResult struct {
	Name  string `json:"name"`
	Bytes int64  `json:"bytes"`
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

type ProgressFunc func(UploadProgress)

type CollectedFile struct {
	LocalPath  string
	RemoteName string
	Size       int64
	Mode       os.FileMode
}

type Uploader struct {
	client *gossh.Client
}

func NewUploader(client *gossh.Client) *Uploader {
	return &Uploader{client: client}
}

func CollectFiles(paths []string) ([]CollectedFile, error) {
	var files []CollectedFile
	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			return nil, fmt.Errorf("collect: %s: %w", p, err)
		}
		if !info.IsDir() {
			if info.Mode().IsRegular() {
				files = append(files, CollectedFile{
					LocalPath:  p,
					RemoteName: filepath.Base(p),
					Size:       info.Size(),
					Mode:       info.Mode().Perm(),
				})
			}
			continue
		}
		err = filepath.WalkDir(p, func(walkPath string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			info, err := d.Info()
			if err != nil {
				return err
			}
			if !info.Mode().IsRegular() {
				return nil
			}
			rel, err := filepath.Rel(filepath.Dir(p), walkPath)
			if err != nil {
				return err
			}
			files = append(files, CollectedFile{
				LocalPath:  walkPath,
				RemoteName: filepath.ToSlash(rel),
				Size:       info.Size(),
				Mode:       info.Mode().Perm(),
			})
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("collect: walk %s: %w", p, err)
		}
	}
	return files, nil
}

func (u *Uploader) ResolveDefaultDir() (string, error) {
	sc, err := sftp.NewClient(u.client)
	if err != nil {
		return "", fmt.Errorf("ssh: open sftp: %w", err)
	}
	defer sc.Close()
	wd, err := sc.Getwd()
	if err != nil {
		return "", fmt.Errorf("ssh: sftp getwd: %w", err)
	}
	return wd, nil
}

func (u *Uploader) Upload(transferID, destDir string, files []CollectedFile, progress ProgressFunc) ([]UploadFileResult, error) {
	if len(files) == 0 {
		return nil, nil
	}

	results := make([]UploadFileResult, len(files))
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, defaultConcurrency)

	for i, f := range files {
		wg.Add(1)
		go func(idx int, file CollectedFile) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			sc, err := sftp.NewClient(u.client)
			if err != nil {
				mu.Lock()
				results[idx] = UploadFileResult{Name: file.RemoteName, Error: fmt.Sprintf("open sftp: %v", err)}
				mu.Unlock()
				return
			}
			defer sc.Close()

			n, err := u.uploadOne(sc, destDir, file, transferID, idx, len(files), progress)
			res := UploadFileResult{Name: file.RemoteName, Bytes: n}
			if err != nil {
				res.Error = err.Error()
			} else {
				res.OK = true
			}
			mu.Lock()
			results[idx] = res
			mu.Unlock()
		}(i, f)
	}
	wg.Wait()

	return results, nil
}

func (u *Uploader) uploadOne(sc *sftp.Client, destDir string, file CollectedFile, transferID string, idx, count int, progress ProgressFunc) (int64, error) {
	src, err := os.Open(file.LocalPath)
	if err != nil {
		return 0, fmt.Errorf("open local file: %w", err)
	}
	defer src.Close()

	remotePath := path.Join(destDir, file.RemoteName)
	remoteDir := path.Dir(remotePath)

	if err := sc.MkdirAll(remoteDir); err != nil {
		return 0, fmt.Errorf("mkdir remote dir: %w", err)
	}

	dst, err := sc.Create(remotePath)
	if err != nil {
		return 0, fmt.Errorf("create remote file: %w", err)
	}

	pw := &progressWriter{
		dst:        dst,
		transferID: transferID,
		idx:        idx,
		count:      count,
		name:       file.RemoteName,
		total:      file.Size,
		progress:   progress,
	}

	n, copyErr := io.Copy(pw, src)
	if closeErr := dst.Close(); closeErr != nil && copyErr == nil {
		return n, fmt.Errorf("close remote file: %w", closeErr)
	}
	if copyErr != nil {
		return n, fmt.Errorf("transfer: %w", copyErr)
	}

	if file.Mode != 0 {
		_ = sc.Chmod(remotePath, file.Mode)
	}

	if progress != nil {
		progress(UploadProgress{
			TransferID: transferID, FileIndex: idx, FileCount: count,
			Name: file.RemoteName, Bytes: n, Total: file.Size,
		})
	}
	return n, nil
}

type progressWriter struct {
	dst        io.Writer
	transferID string
	idx        int
	count      int
	name       string
	total      int64
	progress   ProgressFunc

	written  int64
	lastEmit int64
}

func (w *progressWriter) Write(p []byte) (int, error) {
	n, err := w.dst.Write(p)
	w.written += int64(n)
	if w.progress != nil && w.written-w.lastEmit >= progressInterval {
		w.lastEmit = w.written
		w.progress(UploadProgress{
			TransferID: w.transferID, FileIndex: w.idx, FileCount: w.count,
			Name: w.name, Bytes: w.written, Total: w.total,
		})
	}
	return n, err
}

// ---------------------------------------------------------------------------
// Download
// ---------------------------------------------------------------------------

type DownloadProgress struct {
	TransferID string `json:"transferId"`
	FileIndex  int    `json:"fileIndex"`
	FileCount  int    `json:"fileCount"`
	Name       string `json:"name"`
	Bytes      int64  `json:"bytes"`
	Total      int64  `json:"total"`
}

type DownloadFileResult struct {
	Name  string `json:"name"`
	Bytes int64  `json:"bytes"`
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

type DownloadProgressFunc func(DownloadProgress)

type RemoteEntry struct {
	Name    string `json:"name"`
	Size    int64  `json:"size"`
	IsDir   bool   `json:"isDir"`
	Mode    uint32 `json:"mode"`
	ModTime int64  `json:"modTime"`
}

type Downloader struct {
	client *gossh.Client
}

func NewDownloader(client *gossh.Client) *Downloader {
	return &Downloader{client: client}
}

func (d *Downloader) ReadDir(path string) ([]RemoteEntry, error) {
	sc, err := sftp.NewClient(d.client)
	if err != nil {
		return nil, fmt.Errorf("ssh: open sftp: %w", err)
	}
	defer sc.Close()

	entries, err := sc.ReadDir(path)
	if err != nil {
		return nil, fmt.Errorf("ssh: read dir %s: %w", path, err)
	}

	result := make([]RemoteEntry, len(entries))
	for i, e := range entries {
		info := e
		result[i] = RemoteEntry{
			Name:    info.Name(),
			Size:    info.Size(),
			IsDir:   info.IsDir(),
			Mode:    uint32(info.Mode().Perm()),
			ModTime: info.ModTime().UnixMilli(),
		}
	}
	return result, nil
}

func (d *Downloader) CollectRemotePaths(paths []string) ([]CollectedFile, error) {
	sc, err := sftp.NewClient(d.client)
	if err != nil {
		return nil, fmt.Errorf("ssh: open sftp: %w", err)
	}
	defer sc.Close()

	var files []CollectedFile
	for _, p := range paths {
		info, err := sc.Stat(p)
		if err != nil {
			return nil, fmt.Errorf("collect remote: %s: %w", p, err)
		}
		if !info.IsDir() {
			if info.Mode().IsRegular() {
				files = append(files, CollectedFile{
					LocalPath:  p,
					RemoteName: filepath.Base(p),
					Size:       info.Size(),
					Mode:       info.Mode().Perm(),
				})
			}
			continue
		}
		walker := sc.Walk(p)
		for walker.Step() {
			if walker.Err() != nil {
				continue
			}
			if walker.Stat().IsDir() {
				continue
			}
			if !walker.Stat().Mode().IsRegular() {
				continue
			}
			rel, err := filepath.Rel(filepath.Dir(p), walker.Path())
			if err != nil {
				continue
			}
			files = append(files, CollectedFile{
				LocalPath:  walker.Path(),
				RemoteName: filepath.ToSlash(rel),
				Size:       walker.Stat().Size(),
				Mode:       walker.Stat().Mode().Perm(),
			})
		}
	}
	return files, nil
}

func (d *Downloader) Download(transferID, localDir string, files []CollectedFile, progress DownloadProgressFunc) ([]DownloadFileResult, error) {
	if len(files) == 0 {
		return nil, nil
	}

	results := make([]DownloadFileResult, len(files))
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, defaultConcurrency)

	for i, f := range files {
		wg.Add(1)
		go func(idx int, file CollectedFile) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			sc, err := sftp.NewClient(d.client)
			if err != nil {
				mu.Lock()
				results[idx] = DownloadFileResult{Name: file.RemoteName, Error: fmt.Sprintf("open sftp: %v", err)}
				mu.Unlock()
				return
			}
			defer sc.Close()

			n, err := d.downloadOne(sc, localDir, file, transferID, idx, len(files), progress)
			res := DownloadFileResult{Name: file.RemoteName, Bytes: n}
			if err != nil {
				res.Error = err.Error()
			} else {
				res.OK = true
			}
			mu.Lock()
			results[idx] = res
			mu.Unlock()
		}(i, f)
	}
	wg.Wait()

	return results, nil
}

func (d *Downloader) downloadOne(sc *sftp.Client, localDir string, file CollectedFile, transferID string, idx, count int, progress DownloadProgressFunc) (int64, error) {
	// Prevent path traversal: reject RemoteName components that would escape
	// the intended localDir when joined.
	if strings.Contains(file.RemoteName, "..") {
		return 0, fmt.Errorf("rejected path traversal: %s", file.RemoteName)
	}

	src, err := sc.Open(file.LocalPath)
	if err != nil {
		return 0, fmt.Errorf("open remote file: %w", err)
	}
	defer src.Close()

	localPath := filepath.Join(localDir, file.RemoteName)

	// Double-check that the resolved path is within localDir.
	cleanLocal := filepath.Clean(localDir)
	cleanPath := filepath.Clean(localPath)
	if !strings.HasPrefix(cleanPath, cleanLocal+string(filepath.Separator)) && cleanPath != cleanLocal {
		return 0, fmt.Errorf("path traversal detected: %s", file.RemoteName)
	}

	localDirPath := filepath.Dir(localPath)

	if err := os.MkdirAll(localDirPath, 0755); err != nil {
		return 0, fmt.Errorf("mkdir local dir: %w", err)
	}

	dst, err := os.Create(localPath)
	if err != nil {
		return 0, fmt.Errorf("create local file: %w", err)
	}

	pr := &progressReader{
		src:        src,
		transferID: transferID,
		idx:        idx,
		count:      count,
		name:       file.RemoteName,
		total:      file.Size,
		progress:   progress,
	}

	n, copyErr := io.Copy(dst, pr)
	if closeErr := dst.Close(); closeErr != nil && copyErr == nil {
		return n, fmt.Errorf("close local file: %w", closeErr)
	}
	if copyErr != nil {
		return n, fmt.Errorf("transfer: %w", copyErr)
	}

	if file.Mode != 0 {
		_ = os.Chmod(localPath, file.Mode)
	}

	if progress != nil {
		progress(DownloadProgress{
			TransferID: transferID, FileIndex: idx, FileCount: count,
			Name: file.RemoteName, Bytes: n, Total: file.Size,
		})
	}
	return n, nil
}

type progressReader struct {
	src        io.Reader
	transferID string
	idx        int
	count      int
	name       string
	total      int64
	progress   DownloadProgressFunc

	read     int64
	lastEmit int64
}

func (r *progressReader) Read(p []byte) (int, error) {
	n, err := r.src.Read(p)
	r.read += int64(n)
	if r.progress != nil && r.read-r.lastEmit >= progressInterval {
		r.lastEmit = r.read
		r.progress(DownloadProgress{
			TransferID: r.transferID, FileIndex: r.idx, FileCount: r.count,
			Name: r.name, Bytes: r.read, Total: r.total,
		})
	}
	return n, err
}
