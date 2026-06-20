import Modal from './Modal'

export default function AboutModal({onClose}: {onClose: () => void}) {
  return (
    <Modal title="About Bastion" onClose={onClose} width={400}>
      <div className="space-y-3 text-sm text-muted">
        <p>
          Bastion is an SSH client and session manager with a local-first, encrypted vault.
        </p>
        <div>
          <p className="text-text mb-1">Features:</p>
          <ul className="list-disc pl-5 space-y-1">
            <li>SSH sessions with PTY support and xterm.js terminal</li>
            <li>Encrypted credential vault (master password + per-host secrets)</li>
            <li>Host and group management</li>
            <li>Snippet manager for quick command insertion</li>
            <li>Local port forwarding with auto-start on connect</li>
            <li>File and folder upload to the remote host over SFTP (recursive, from the terminal context menu)</li>
            <li>Auto-lock on inactivity and screen lock</li>
            <li>SSH config import (~/.ssh/config)</li>
            <li>Session logging with auto-rotated log files</li>
            <li>Drag-and-drop tab reordering</li>
            <li>Connection health monitoring (uptime, keepalive)</li>
            <li>AI command generation (OpenAI / Anthropic / OpenRouter / Ollama)</li>
            <li>AI error explanation for failed commands</li>
            <li>Per-host font size adjustment</li>
            <li>Copy/paste toolbar buttons on each terminal pane</li>
          </ul>
        </div>
        <p>Author: <span className="text-text">fardani235</span></p>
      </div>
    </Modal>
  )
}
