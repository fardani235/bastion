import type {HostDTO} from '../lib/api'

// HostItem is one host row. A single click connects; the row also exposes
// edit/delete affordances on hover. Right-click opens a context menu.
export default function HostItem({
  host,
  onConnect,
  onEdit,
  onDelete,
  onPortForwards,
  onContextMenu,
}: {
  host: HostDTO
  onConnect: () => void
  onEdit: () => void
  onDelete: () => void
  onPortForwards: () => void
  onContextMenu: (e: React.MouseEvent, host: HostDTO) => void
}) {
  return (
    <div
      className="group flex cursor-pointer items-center gap-2 rounded-md px-2 py-1.5 text-sm hover:bg-surface-2"
      onDoubleClick={onConnect}
      onContextMenu={(e) => onContextMenu(e, host)}
    >
      <span className="h-1.5 w-1.5 rounded-full bg-accent-dim" />
      <button onClick={onConnect} className="min-w-0 flex-1 truncate text-left text-text" title={`${host.username}@${host.hostname}:${host.port}`}>
        {host.label}
      </button>
      <span className="hidden items-center gap-1 group-hover:flex">
        <button onClick={onPortForwards} className="text-muted hover:text-accent" title="Port Forwards">
          ↕
        </button>
        <button onClick={onEdit} className="text-muted hover:text-text" title="Edit">
          ✎
        </button>
        <button onClick={onDelete} className="text-muted hover:text-danger" title="Delete">
          ✕
        </button>
      </span>
    </div>
  )
}
