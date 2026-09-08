import { useState } from "preact/hooks";
import type { SessionMeta } from "../types";
import { workDirBaseName } from "../lib/labels";
import {
  IconBolt, IconFolder, IconPlus, IconPencil, IconTrash,
} from "../lib/icons";

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

// groupByWorkspace 按工作目录把会话分成若干组：同一工作区的会话归为一组，
// 类似 Codex 按工作区组织对话。组顺序依据后端返回的 updated_at 倒序遍历首次出现的顺序
// （即最近更新的工作区在前），组内保持后端顺序（每条会话仍按更新时间倒序）。
function groupByWorkspace(sessions: SessionMeta[]) {
  const groups: { label: string; title: string; items: SessionMeta[] }[] = [];
  const byWorkDir = new Map<string, { label: string; title: string; items: SessionMeta[] }>();
  for (const sess of sessions) {
    const wd = sess.workdir || "";
    let g = byWorkDir.get(wd);
    if (!g) {
      g = {
        label: wd ? workDirBaseName(wd) : "未设置工作区",
        title: wd,
        items: [],
      };
      byWorkDir.set(wd, g);
      groups.push(g);
    }
    g.items.push(sess);
  }
  return groups;
}

// Sidebar —— 左侧导航栏：品牌区 + 新建会话 + 按工作区分组的会话列表 + 底部状态。
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

  const groups = groupByWorkspace(sessions);

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
        <span class="sidebar__mark"><IconBolt size={15} /></span>
        <span class="sidebar__name">ZoomClient</span>
        <span class="sidebar__mode">web</span>
      </div>

      {/* 主操作：新建会话 */}
      <button class="sidebar__new" onClick={onNew}>
        <IconPlus size={14} />
        新对话
        <kbd class="kbd">Ctrl N</kbd>
      </button>

      {/* 会话列表 */}
      <div class="sidebar__scroll">
        {groups.length === 0 && (
          <div class="sidebar__empty">暂无会话，点击上方"新对话"开始</div>
        )}
        {groups.map((group) => (
          <div key={group.label} class="sidebar__group">
            <div class="sidebar__group-label" title={group.title}>
              <IconFolder size={11} />
              <span class="sidebar__group-name">{group.label}</span>
              <span class="sidebar__group-count">{group.items.length}</span>
            </div>
            {group.items.map((s) => (
              <div
                key={s.id}
                class={`sidebar__item ${s.id === currentId ? "is-active" : ""}`}
                onClick={() => onSelect(s.id)}
              >
                {editingId === s.id ? (
                  <input
                    class="sidebar__rename"
                    value={editTitle}
                    onInput={(e) => setEditTitle((e.target as HTMLInputElement).value)}
                    onKeyDown={handleKeyDown}
                    onBlur={confirmRename}
                    onClick={(e) => e.stopPropagation()}
                    autoFocus
                  />
                ) : (
                  <>
                    <span class="sidebar__title">{s.title}</span>
                    <div class="sidebar__tools">
                      <button
                        class="sidebar__tool"
                        title="重命名"
                        onClick={(e) => {
                          e.stopPropagation();
                          startRename(s.id, s.title);
                        }}
                      >
                        <IconPencil size={12} />
                      </button>
                      <button
                        class="sidebar__tool sidebar__tool--danger"
                        title="删除"
                        onClick={(e) => {
                          e.stopPropagation();
                          onDelete(s.id);
                        }}
                      >
                        <IconTrash size={12} />
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
      <div class="sidebar__foot">
        <div class="sidebar__foot-row" title={workDir || "未设置工作目录"}>
          <IconFolder size={12} />
          <span class="sidebar__foot-text">{workDirBaseName(workDir)}</span>
        </div>
        <div class="sidebar__foot-row">
          <span class={`led ${connected ? "led--on" : "led--off"}`} />
          <span class="sidebar__foot-text">{connected ? "已连接" : "未连接"}</span>
        </div>
      </div>
    </aside>
  );
}
