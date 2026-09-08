import { useState, useRef, useEffect } from "preact/hooks";
import type { ModelPreset } from "../types";
import { ModelDialog } from "./ModelDialog";
import { IconCheck, IconChevronDown, IconChevronRight, IconPencil, IconPlus, IconSearch, IconLayers } from "../lib/icons";

interface Props {
  models: ModelPreset[];
  active: string;
  onSelect: (name: string) => void;
  onAdd: (preset: ModelPreset) => void;
  onEdit: (name: string, preset: ModelPreset) => void;
  disabled?: boolean;
}

// ModelSelector —— 输入板上的模型切换 chip，展开一块菜单。
// 复写 main > 添加 / 编辑，编辑态另开子菜单列出可选模型。
export function ModelSelector({ models, active, onSelect, onAdd, onEdit, disabled }: Props) {
  const [open, setOpen] = useState(false);
  const [dialogMode, setDialogMode] = useState<"add" | "edit" | null>(null);
  const [editTarget, setEditTarget] = useState<ModelPreset | undefined>(undefined);
  const [showEditSub, setShowEditSub] = useState(false);
  const [search, setSearch] = useState("");
  const ref = useRef<HTMLDivElement>(null);

  // 点击外部收起菜单，复位所有子状态
  const closeDropdown = () => {
    setOpen(false);
    setShowEditSub(false);
    setSearch("");
  };

  useEffect(() => {
    const handler = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) {
        closeDropdown();
      }
    };
    document.addEventListener("mousedown", handler);
    return () => document.removeEventListener("mousedown", handler);
  }, []);

  // Filter models by search query
  const filtered = search
    ? models.filter((m) =>
        m.name.toLowerCase().includes(search.toLowerCase()) ||
        m.model_name.toLowerCase().includes(search.toLowerCase())
      )
    : models;

  const handleSelect = (name: string) => {
    onSelect(name);
    closeDropdown();
  };

  const handleOpenAdd = () => {
    setDialogMode("add");
    setEditTarget(undefined);
    closeDropdown();
  };

  const handleOpenEdit = (preset: ModelPreset) => {
    setDialogMode("edit");
    setEditTarget(preset);
    closeDropdown();
  };

  const handleDialogSave = (preset: ModelPreset) => {
    if (dialogMode === "add") {
      onAdd(preset);
    } else if (dialogMode === "edit" && editTarget) {
      onEdit(editTarget.name, preset);
    }
    setDialogMode(null);
    setEditTarget(undefined);
  };

  const handleDialogClose = () => {
    setDialogMode(null);
    setEditTarget(undefined);
  };

  const activeLabel = active || "未选择模型";

  return (
    <div class="model-selector" ref={ref}>
      <button
        class="chip model-selector__trigger"
        onClick={() => !disabled && (open ? closeDropdown() : setOpen(true))}
        disabled={disabled}
        title="切换模型"
      >
        <IconLayers size={13} />
        <span class="chip__label">{activeLabel}</span>
        <IconChevronDown size={11} />
      </button>

      {open && (
        <div class="menu">
          {models.length > 3 && (
            <div class="menu__search">
              <IconSearch size={13} />
              <input
                class="menu__search-input"
                type="text"
                placeholder="搜索模型…"
                value={search}
                onInput={(e) => setSearch((e.target as HTMLInputElement).value)}
                onClick={(e) => e.stopPropagation()}
              />
            </div>
          )}
          {filtered.length === 0 && (
            <div class="menu__empty">{search ? "无匹配模型" : "未配置模型"}</div>
          )}
          {filtered.map((m) => (
            <button
              key={m.name}
              class={`menu__item ${m.name === active ? "is-active" : ""}`}
              onClick={() => handleSelect(m.name)}
            >
              <span class="menu__item-name">{m.name}</span>
              <span class="menu__item-meta">{m.type} / {m.model_name}</span>
              {m.name === active && <span class="menu__check"><IconCheck size={13} /></span>}
            </button>
          ))}

          <div class="menu__sep" />
          <button class="menu__item menu__item--action" onClick={handleOpenAdd}>
            <IconPlus size={14} />
            <span class="menu__item-name">添加模型</span>
          </button>
          {models.length > 0 && (
            <button
              class="menu__item menu__item--action"
              onClick={() => setShowEditSub(!showEditSub)}
            >
              <IconPencil size={14} />
              <span class="menu__item-name">编辑模型</span>
              <span class="menu__sub-arrow">
                {showEditSub ? <IconChevronDown size={11} /> : <IconChevronRight size={11} />}
              </span>
            </button>
          )}
          {showEditSub &&
            models.map((m) => (
              <button
                key={`edit-${m.name}`}
                class="menu__item menu__item--action menu__sub"
                onClick={() => handleOpenEdit(m)}
              >
                <span class="menu__item-name">{m.name}</span>
                <span class="menu__item-meta">{m.type}</span>
              </button>
            ))}
        </div>
      )}

      {dialogMode && (
        <ModelDialog
          mode={dialogMode}
          initial={editTarget}
          onSave={handleDialogSave}
          onClose={handleDialogClose}
        />
      )}
    </div>
  );
}
