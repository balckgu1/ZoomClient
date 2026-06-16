import { useState } from "preact/hooks";
import type { SessionMeta } from "../types";

interface SidebarProps {
  sessions: SessionMeta[];
  currentId: string;
  onSelect: (id: string) => void;
  onNew: () => void;
  onDelete: (id: string) => void;
  onRename: (id: string, title: string) => void;
}

function groupByDate(sessions: SessionMeta[]) {
  const now = new Date();
  const today = new Date(now.getFullYear(), now.getMonth(), now.getDate());
  const yesterday = new Date(today.getTime() - 86400000);

  const groups: { label: string; items: SessionMeta[] }[] = [
    { label: "today", items: [] },
    { label: "testerdat", items: [] },
    { label: "earlier", items: [] },
  ];

  for (const s of sessions) {
    const d = new Date(s.updated_at);
    if (d >= today) groups[0].items.push(s);
    else if (d >= yesterday) groups[1].items.push(s);
    else groups[2].items.push(s);
  }

  return groups.filter((g) => g.items.length > 0);
}

export function Sidebar({
  sessions,
  currentId,
  onSelect,
  onNew,
  onDelete,
  onRename,
}: SidebarProps) {
  const [editingId, setEditingId] = useState<string | null>(null);
  const [editTitle, setEditTitle] = useState("");

  const groups = groupByDate(sessions);

  const startRename = (id: string, title: string) => {
    setEditingId(id);
    setEditTitle(title);
  };

  const confirmRename = () => {
    if (editingId && editTitle.trim()) {
      onRename(editingId, editTitle.trim());
    }
    setEditingId(null);
  };

  const handleKeyDown = (e: KeyboardEvent) => {
    if (e.key === "Enter") confirmRename();
    if (e.key === "Escape") setEditingId(null);
  };

  return (
    <div class="sidebar">
      <button class="sidebar-new" onClick={onNew}>
        + New Chat
      </button>
      <div class="sidebar-list">
        {groups.map((group) => (
          <div key={group.label} class="sidebar-group">
            <div class="sidebar-group-label">{group.label}</div>
            {group.items.map((s) => (
              <div
                key={s.id}
                class={`sidebar-item ${s.id === currentId ? "active" : ""}`}
                onClick={() => onSelect(s.id)}
              >
                {editingId === s.id ? (
                  <input
                    class="sidebar-rename-input"
                    value={editTitle}
                    onInput={(e) => setEditTitle((e.target as HTMLInputElement).value)}
                    onKeyDown={handleKeyDown}
                    onBlur={confirmRename}
                    onClick={(e) => e.stopPropagation()}
                    autoFocus
                  />
                ) : (
                  <>
                    <span class="sidebar-title">{s.title}</span>
                    <div class="sidebar-actions">
                      <button
                        class="sidebar-action-btn"
                        title="Rename"
                        onClick={(e) => {
                          e.stopPropagation();
                          startRename(s.id, s.title);
                        }}
                      >
                        ✏️
                      </button>
                      <button
                        class="sidebar-action-btn sidebar-action-delete"
                        title="Delete"
                        onClick={(e) => {
                          e.stopPropagation();
                          onDelete(s.id);
                        }}
                      >
                        🗑️
                      </button>
                    </div>
                  </>
                )}
              </div>
            ))}
          </div>
        ))}
      </div>
    </div>
  );
}
