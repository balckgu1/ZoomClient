import { useState } from "preact/hooks";
import type { SessionMeta } from "../types";
import { workDirBaseName } from "../lib/labels";

interface SidebarProps {
  sessions: SessionMeta[];
  currentId: string;
  // connected 用于底部展示连接状态
  connected: boolean;
  // workDir 用于底部展示当前工作目录
  workDir: string;
  onSelect: (id: string) => void;
  onNew: () => void;
  onDelete: (id: string) => void;
  onRename: (id: string, title: string) => void;
}

// groupByDate 按更新时间把会话分为"今天 / 昨天 / 更早"三组，并过滤空组。
function groupByDate(sessions: SessionMeta[]) {
  const now = new Date();
  const today = new Date(now.getFullYear(), now.getMonth(), now.getDate());
  const yesterday = new Date(today.getTime() - 86400000);

  const groups: { label: string; items: SessionMeta[] }[] = [
    { label: "今天", items: [] },
    { label: "昨天", items: [] },
    { label: "更早", items: [] },
  ];

  for (const s of sessions) {
    const d = new Date(s.updated_at);
    if (d >= today) groups[0].items.push(s);
    else if (d >= yesterday) groups[1].items.push(s);
    else groups[2].items.push(s);
  }

  return groups.filter((g) => g.items.length > 0);
}

// Sidebar —— 左侧导航栏：品牌区 + 新建会话 + 分组会话列表 + 底部状态。
export function Sidebar({
  sessions,
  currentId,
  connected,
  workDir,
  onSelect,
  onNew,
  onDelete,
  onRename,
}: SidebarProps) {
  const [editingId, setEditingId] = useState<string | null>(null);
  const [editTitle, setEditTitle] = useState("");

  const groups = groupByDate(sessions);

  // startRename 进入某条会话的重命名编辑态
  const startRename = (id: string, title: string) => {
    setEditingId(id);
    setEditTitle(title);
  };

  // confirmRename 提交重命名（空标题忽略）
  const confirmRename = () => {
    if (editingId && editTitle.trim()) {
      onRename(editingId, editTitle.trim());
    }
    setEditingId(null);
  };

  // 回车确认、Esc 取消
  const handleKeyDown = (e: KeyboardEvent) => {
    if (e.key === "Enter") confirmRename();
    if (e.key === "Escape") setEditingId(null);
  };

  return (
    <aside class="sidebar">
      {/* 品牌区 */}
      <div class="sidebar__brand">
        <span class="sidebar__logo">⚡</span>
        <span class="sidebar__name">ZoomClient</span>
      </div>

      {/* 主操作：新建会话 */}
      <button class="sidebar-new" onClick={onNew}>
        ＋ 新对话
      </button>

      {/* 会话列表 */}
      <div class="sidebar-list">
        {groups.length === 0 && (
          <div class="sidebar-empty">暂无会话，点击上方"新对话"开始</div>
        )}
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
                        title="重命名"
                        onClick={(e) => {
                          e.stopPropagation();
                          startRename(s.id, s.title);
                        }}
                      >
                        ✏️
                      </button>
                      <button
                        class="sidebar-action-btn sidebar-action-delete"
                        title="删除"
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

      {/* 底部状态区 */}
      <div class="sidebar__footer">
        <div class="sidebar__footer-row" title={workDir || "未设置工作目录"}>
          <span class="sidebar__footer-icon">📁</span>
          <span class="sidebar__footer-text">{workDirBaseName(workDir)}</span>
        </div>
        <div class="sidebar__footer-row">
          <span class={`conn-dot ${connected ? "connected" : "disconnected"}`} />
          <span class="sidebar__footer-text">{connected ? "已连接" : "未连接"}</span>
        </div>
      </div>
    </aside>
  );
}
