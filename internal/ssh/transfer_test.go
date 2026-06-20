package ssh

import (
	"crypto/ed25519"
	"crypto/rand"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	glssh "github.com/gliderlabs/ssh"
	"github.com/pkg/sftp"
	gossh "golang.org/x/crypto/ssh"
)

func startSFTPServer(t *testing.T) *gossh.Client {
	t.Helper()

	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	signer, err := gossh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatalf("NewSignerFromKey: %v", err)
	}

	srv := &glssh.Server{
		PasswordHandler: func(glssh.Context, string) bool { return true },
		SubsystemHandlers: map[string]glssh.SubsystemHandler{
			"sftp": func(s glssh.Session) {
				server, err := sftp.NewServer(s)
				if err != nil {
					return
				}
				_ = server.Serve()
				_ = server.Close()
			},
		},
	}
	srv.AddHostKey(signer)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })

	addr := ln.Addr().String()
	client, err := gossh.Dial("tcp", addr, &gossh.ClientConfig{
		User:            "tester",
		Auth:            []gossh.AuthMethod{gossh.Password("any")},
		HostKeyCallback: gossh.InsecureIgnoreHostKey(),
		Timeout:         5 * time.Second,
	})
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func TestCollectFiles_SingleFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "hello.txt")
	if err := os.WriteFile(src, []byte("content"), 0o644); err != nil {
		t.Fatal(err)
	}

	files, err := CollectFiles([]string{src})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}
	if files[0].LocalPath != src {
		t.Fatalf("expected LocalPath %q, got %q", src, files[0].LocalPath)
	}
	if files[0].RemoteName != "hello.txt" {
		t.Fatalf("expected RemoteName %q, got %q", "hello.txt", files[0].RemoteName)
	}
}

func TestCollectFiles_Directory(t *testing.T) {
	dir := t.TempDir()

	if err := os.MkdirAll(filepath.Join(dir, "sub", "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "root.txt"), []byte("root"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sub", "a.txt"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sub", "nested", "b.txt"), []byte("b"), 0o644); err != nil {
		t.Fatal(err)
	}

	files, err := CollectFiles([]string{dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 3 {
		t.Fatalf("expected 3 files, got %d", len(files))
	}

	prefix := filepath.Base(dir) + "/"
	for _, f := range files {
		if len(f.RemoteName) < len(prefix) || f.RemoteName[:len(prefix)] != prefix {
			t.Fatalf("expected remote name %q to start with %q", f.RemoteName, prefix)
		}
	}

	names := map[string]bool{}
	for _, f := range files {
		names[f.RemoteName] = true
	}
	for _, want := range []string{prefix + "root.txt", prefix + "sub/a.txt", prefix + "sub/nested/b.txt"} {
		if !names[want] {
			t.Fatalf("expected %q in results", want)
		}
	}
}

func TestUploader_SingleFile(t *testing.T) {
	client := startSFTPServer(t)
	dir := t.TempDir()
	src := filepath.Join(dir, "hello.txt")
	want := []byte("hello bastion upload\n")
	if err := os.WriteFile(src, want, 0o644); err != nil {
		t.Fatalf("write src: %v", err)
	}

	destDir := filepath.Join(dir, "remote")
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		t.Fatalf("mkdir dest: %v", err)
	}

	up := NewUploader(client)
	files, err := CollectFiles([]string{src})
	if err != nil {
		t.Fatal(err)
	}

	var progressCalls int
	results, err := up.Upload("t1", destDir, files, func(UploadProgress) { progressCalls++ })
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if len(results) != 1 || !results[0].OK {
		t.Fatalf("expected 1 ok result, got %+v", results)
	}

	got, err := os.ReadFile(filepath.Join(destDir, "hello.txt"))
	if err != nil {
		t.Fatalf("read uploaded: %v", err)
	}
	if string(got) != string(want) {
		t.Fatalf("content mismatch: got %q want %q", got, want)
	}
	if progressCalls == 0 {
		t.Fatal("expected at least one progress callback (final 100%)")
	}
}

func TestUploader_Directory(t *testing.T) {
	client := startSFTPServer(t)
	localDir := t.TempDir()
	remoteRoot := filepath.Join(localDir, "remote")

	if err := os.MkdirAll(filepath.Join(localDir, "src", "util"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(localDir, "README.md"), []byte("# project"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(localDir, "src", "main.go"), []byte("package main"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(localDir, "src", "util", "helper.go"), []byte("package util"), 0o644); err != nil {
		t.Fatal(err)
	}

	up := NewUploader(client)
	files, err := CollectFiles([]string{localDir})
	if err != nil {
		t.Fatal(err)
	}

	results, err := up.Upload("t-dir", remoteRoot, files, nil)
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	for _, r := range results {
		if !r.OK {
			t.Fatalf("unexpected failure: %s: %s", r.Name, r.Error)
		}
	}

	dirBase := filepath.Base(localDir)
	check := func(path string, want string) {
		got, err := os.ReadFile(filepath.Join(remoteRoot, path))
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if string(got) != want {
			t.Fatalf("%s: got %q want %q", path, got, want)
		}
	}
	check(filepath.Join(dirBase, "README.md"), "# project")
	check(filepath.Join(dirBase, "src", "main.go"), "package main")
	check(filepath.Join(dirBase, "src", "util", "helper.go"), "package util")
}

func TestUploader_Overwrite(t *testing.T) {
	client := startSFTPServer(t)
	dir := t.TempDir()
	src := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(src, []byte("new"), 0o644); err != nil {
		t.Fatalf("write src: %v", err)
	}
	destDir := filepath.Join(dir, "remote")
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(destDir, "f.txt"), []byte("OLD LONGER CONTENT"), 0o644); err != nil {
		t.Fatalf("write existing: %v", err)
	}

	up := NewUploader(client)
	files, err := CollectFiles([]string{src})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := up.Upload("t2", destDir, files, nil); err != nil {
		t.Fatalf("Upload: %v", err)
	}
	got, _ := os.ReadFile(filepath.Join(destDir, "f.txt"))
	if string(got) != "new" {
		t.Fatalf("overwrite failed: got %q", got)
	}
}

func TestUploader_MultipleAndPartialFailure(t *testing.T) {
	client := startSFTPServer(t)
	dir := t.TempDir()
	good := filepath.Join(dir, "good.txt")
	if err := os.WriteFile(good, []byte("ok"), 0o644); err != nil {
		t.Fatalf("write good: %v", err)
	}
	missing := filepath.Join(dir, "does-not-exist.txt")

	destDir := filepath.Join(dir, "remote")
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	up := NewUploader(client)
	// CollectFiles will skip the non-existent path (fails with error),
	// so test per-file failure by passing a non-existent file directly.
	files := []CollectedFile{
		{LocalPath: missing, RemoteName: "does-not-exist.txt", Size: 0},
		{LocalPath: good, RemoteName: "good.txt", Size: 2},
	}
	results, err := up.Upload("t3", destDir, files, nil)
	if err != nil {
		t.Fatalf("Upload should not return a whole-transfer error for a per-file issue: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].OK {
		t.Fatalf("expected first (missing) file to fail, got %+v", results[0])
	}
	if !results[1].OK {
		t.Fatalf("expected second (good) file to succeed, got %+v", results[1])
	}
	if _, err := os.Stat(filepath.Join(destDir, "good.txt")); err != nil {
		t.Fatalf("good file should have uploaded despite the earlier failure: %v", err)
	}
}

func TestUploader_RemoteNameIsBaseOnly(t *testing.T) {
	client := startSFTPServer(t)
	dir := t.TempDir()
	nested := filepath.Join(dir, "a", "b", "c")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("mkdir nested: %v", err)
	}
	src := filepath.Join(nested, "payload.bin")
	if err := os.WriteFile(src, []byte("x"), 0o644); err != nil {
		t.Fatalf("write src: %v", err)
	}
	destDir := filepath.Join(dir, "remote")
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		t.Fatalf("mkdir dest: %v", err)
	}

	up := NewUploader(client)
	files, err := CollectFiles([]string{src})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := up.Upload("t4", destDir, files, nil); err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if _, err := os.Stat(filepath.Join(destDir, "payload.bin")); err != nil {
		t.Fatalf("expected file at destDir/payload.bin: %v", err)
	}
}

func TestUploader_FileModePreservation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file mode preservation is Unix-specific")
	}

	client := startSFTPServer(t)
	dir := t.TempDir()
	src := filepath.Join(dir, "script.sh")
	if err := os.WriteFile(src, []byte("#!/bin/sh\necho hi\n"), 0o755); err != nil {
		t.Fatalf("write src: %v", err)
	}

	destDir := filepath.Join(dir, "remote")
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		t.Fatalf("mkdir dest: %v", err)
	}

	up := NewUploader(client)
	files, err := CollectFiles([]string{src})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := up.Upload("t-mode", destDir, files, nil); err != nil {
		t.Fatalf("Upload: %v", err)
	}

	info, err := os.Stat(filepath.Join(destDir, "script.sh"))
	if err != nil {
		t.Fatalf("stat uploaded: %v", err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("expected mode 0755, got %04o", info.Mode().Perm())
	}
}

func TestUploader_ResolveDefaultDir(t *testing.T) {
	client := startSFTPServer(t)
	up := NewUploader(client)
	dir, err := up.ResolveDefaultDir()
	if err != nil {
		t.Fatalf("ResolveDefaultDir: %v", err)
	}
	if dir == "" {
		t.Fatal("expected a non-empty default dir")
	}
}
