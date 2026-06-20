# Bastion

A local-first, cross-platform **SSH connection manager** with an encrypted
credential vault. Bastion keeps your hosts, passwords, keys, snippets, and
port-forward rules in a single SQLite database, encrypted under a master
password — and gives you a fast multi-tab terminal to connect with.

Built with [Wails v2](https://wails.io) (Go backend + React/TypeScript
frontend) and [xterm.js](https://xtermjs.org).

---

## Features

- **Encrypted vault** — secrets are sealed with AES-256-GCM under a key derived
  from your master password (Argon2id). The password is never stored; there is
  no recovery.
- **Host management** — organize SSH hosts into groups, with password or
  private-key authentication (encrypted passphrases supported).
- **Multi-tab terminal** — full PTY sessions over `golang.org/x/crypto/ssh`,
  rendered with xterm.js, with per-host font size and live session health.
- **Strict host-key trust** — unknown keys prompt once and pin to a
  `known_hosts` file; a *changed* key is a hard failure, never a silent accept.
- **Local port forwarding** — define `local → remote` rules per host that start
  and stop with the session, bound to `127.0.0.1` only.
- **File upload** — upload files and folders to the remote host over SFTP via
  the right-click context menu on any connected terminal. Directories are
  uploaded recursively with their structure preserved. Reuses the session's
  existing authenticated connection — no extra password prompt.
- **Snippets** — save and paste frequently used commands.
- **AI command generation** — describe what you want and get a shell command,
  powered by OpenAI, Anthropic, or any OpenAI-compatible provider (OpenRouter,
  Ollama, etc.). Bring your own API key — stored encrypted in the vault.
- **AI error explanation** — when a command fails (stderr patterns like
  `command not found`, `Permission denied`, etc.), Bastion can explain the error
  and suggest a fix.
- **Per-host font size** — each session remembers its own font size, adjustable
  from the host list context menu.
- **Right-click context menu** — right-click any connected terminal to Copy,
  Paste, or upload files and folders (no keyboard interception, so vim and
  other TUI apps work).
- **SSH config import** — scan and import hosts from `~/.ssh/config`.
- **Auto-lock** — the vault locks on idle timeout and OS screen lock, tearing
  down every live session.

## Demos
### Context menu
![Right-click context menu — Copy, Paste, Upload](vid2.gif)

### Snippets drawer
![Snippets drawer — save and paste frequently used commands](vid1.gif)

### AI drawer
![AI command generation and error explanation](vid0.gif)

---

## Security model

| Concern | Approach |
|---|---|
| Key derivation | Argon2id (`time=3`, `memory=64 MiB`, `threads=4`); legacy PBKDF2-600k vaults still unlock |
| Secret encryption | AES-256-GCM, random nonce per blob |
| Password check | Verify-blob scheme — confirms the master password without decrypting any credential |
| Credential boundary | Plaintext secrets travel renderer→Go only; the UI receives `hasPassword`-style flags, never the secret |
| Master password | Minimum 8 characters, enforced at the backend (not just the UI) |
| In-memory key | Held only while unlocked; zeroed on lock, screensaver, and shutdown |
| Host keys | Strict `known_hosts`; changed keys rejected as a possible MITM |

See [Operational notes](#operational-notes) below for vault location, session
logging, and auto-lock specifics.

## Requirements

- **Go** ≥ 1.25
- **Node** ≥ 18
- **[Wails CLI](https://wails.io/docs/gettingstarted/installation)** v2.12+
- **Linux:** GTK3 + WebKit2GTK dev packages. On distros shipping only
  `webkit2gtk-4.1` (Ubuntu 24.04+, Fedora 38+), the build tag `webkit2_41` is
  required — the `Makefile` already passes it.

## Development

The `Makefile` wraps the common tasks (all with the `webkit2_41` tag):

```bash
make run      # hot-reloading dev app (React + Go rebuild on change)
make build    # production binary -> build/bin/
make deb      # build + package a Debian .deb
make clean    # remove build/bin
go test ./... # run the Go test suite
```

To run without the Makefile on a WebKit-4.1 system:

```bash
wails dev   -tags webkit2_41
wails build -tags webkit2_41
```

## Repository layout

```
main.go            — Wails bootstrap
app.go             — App struct: owns the store, in-memory vault key, auto-lock
auth.go            — Setup/Unlock/Lock, KDF parameter handling
hosts.go           — host CRUD IPC + credential encrypt/decrypt boundary
sessions.go        — OpenSession, host-key trust, terminal I/O IPC
crud.go            — groups & snippets IPC
portforwards.go    — port-forward config IPC
hosts_import.go    — ~/.ssh/config scan & import
transfer.go        — file-upload IPC: PrepareUpload, UploadFiles, ResolveUploadDir
ai.go              — AI IPC: GenerateCommand, ExplainError, Get/SetAIConfig, TestAIConnection
emitter.go         — Wails event emitter + optional session logging
session_health.go  — live session info

internal/vault/    — Argon2id KDF, AES-256-GCM, verify-blob (crypto core)
internal/store/    — SQLite persistence (hosts, groups, snippets, port_forwards)
internal/ssh/      — known-hosts trust, PTY session, port-forward & SFTP upload managers
internal/ai/       — LLM client (OpenAI, Anthropic, OpenAI-compatible formats)

frontend/          — React + TypeScript + Vite + Tailwind + xterm.js
```

The three `internal/` packages are deliberately decoupled: `vault` and `store`
know nothing of each other (ciphertext is opaque to the store), and `internal/ssh`
is vault-agnostic — callers pass resolved credentials and receive output through
an `Emitter` interface, so it's fully testable without Wails or a live server.

## Operational notes

### Vault location

On Linux the vault lives at `~/.config/bastion/vault.db`. The directory is
created on first launch with mode `0700`.

### Auto-lock

The vault locks automatically after an idle timeout (default 5 minutes,
configurable via `SetAutoLockSeconds`, minimum 60s) and when the OS screensaver
activates. **Locking closes every live SSH session** and rejects further
terminal input — a locked vault never leaves a writable terminal open behind it.

### File upload

Right-click a connected terminal to open the context menu with **Upload Files…**
and **Upload Folder…** options. Native drag-and-drop (files → connected terminal)
also works on platforms where WebKit2GTK supports it.

Uploads run over **SFTP on the session's existing SSH connection** — no
re-authentication, no second password prompt, and file bytes are read by the Go
backend directly (they never pass through the renderer).

- **Directories** — folders are uploaded recursively. Their structure is
  preserved on the remote side: uploading `project/` with `src/main.go` and
  `README.md` lands as `destDir/project/src/main.go` and
  `destDir/project/README.md`.
- **Destination** — files land in the shell's current working directory when it
  can be determined, otherwise the remote home directory. The CWD is tracked
  from the OSC 7 escape sequence that well-configured shells emit on each prompt;
  shells that don't emit it fall back to home. Either way, the confirmation
  dialog shows the resolved path as an **editable** field, so you always see and
  can correct where files will go before sending.
- **Confirmation** — every operation opens a dialog listing every file that will
  be transferred, the target host, and the destination. Nothing is sent until
  you confirm.
- **Concurrent transfers** — up to 4 files upload in parallel, each using its
  own SFTP channel over the same SSH connection.
- **File permissions** — the original Unix file mode is preserved (e.g.,
  executable scripts stay executable after upload).
- **Overwrite** — an existing remote file with the same name is overwritten,
  matching `scp`'s default behavior.

Uploads require the remote server's SFTP subsystem (enabled by default on
essentially all OpenSSH installations). If it is disabled, the upload fails with
a clear error and the interactive session is unaffected.

### AI configuration

AI is **Bring Your Own Key** — Bastion never ships with bundled API credentials.

Supported providers:
- **OpenAI** — uses `https://api.openai.com/v1` by default
- **Anthropic** — uses the Anthropic Messages API
- **OpenAI-compatible** — any service with an OpenAI-compatible `/chat/completions` endpoint (OpenRouter, Ollama, Groq, etc.)

Your API key is encrypted under the vault master key and stored in `vault_meta`,
alongside the provider and model selection. Only the AI drawer's frontend
components talk to the Go IPC — the key never reaches the renderer.

Models use the provider's native naming convention:
- OpenAI: `gpt-4o`, `gpt-4o-mini`, etc.
- Anthropic: `claude-sonnet-4-20250514`, `claude-haiku-3-5`, etc.
- OpenRouter: `provider/model` format, e.g. `openai/gpt-4o`, `nvidia/nemotron-3-ultra-550b-a55b:free`

### Session logging

Session logging is **off by default**. When enabled (via `SetSessionLogging`),
Bastion writes the full terminal stream of each session to
`~/.config/bastion/logs/<label>-<timestamp>.log` with mode `0600`.

These logs are **plaintext** and can capture anything shown on the terminal,
including passwords typed at `sudo`/`mysql` prompts and other secrets. They are
**not** encrypted under the vault key. Leave logging disabled unless you
specifically need it, and treat the log directory as sensitive.

## ⚠️ No password recovery

The master password is never stored, transmitted, or recoverable. If you forget
it, the vault is permanently inaccessible. This is by design.
