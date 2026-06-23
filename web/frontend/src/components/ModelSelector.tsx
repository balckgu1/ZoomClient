import { useState, useRef, useEffect } from "preact/hooks";
import type { ModelPreset } from "../types";
import { ModelDialog } from "./ModelDialog";

interface Props {
  models: ModelPreset[];
  active: string;
  onSelect: (name: string) => void;
  onAdd: (preset: ModelPreset) => void;
  onEdit: (name: string, preset: ModelPreset) => void;
  disabled?: boolean;
}

export function ModelSelector({ models, active, onSelect, onAdd, onEdit, disabled }: Props) {
  const [open, setOpen] = useState(false);
  const [dialogMode, setDialogMode] = useState<"add" | "edit" | null>(null);
  const [editTarget, setEditTarget] = useState<ModelPreset | undefined>(undefined);
  const [showEditSub, setShowEditSub] = useState(false);
  const ref = useRef<HTMLDivElement>(null);

  // Close dropdown when clicking outside
  useEffect(() => {
    const handler = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) {
        setOpen(false);
      }
    };
    document.addEventListener("mousedown", handler);
    return () => document.removeEventListener("mousedown", handler);
  }, []);

  const handleSelect = (name: string) => {
    onSelect(name);
    setOpen(false);
  };

  const handleOpenAdd = () => {
    setDialogMode("add");
    setEditTarget(undefined);
    setOpen(false);
  };

  const handleOpenEdit = (preset: ModelPreset) => {
    setDialogMode("edit");
    setEditTarget(preset);
    setOpen(false);
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

  const activeLabel = active || "No model";

  return (
    <div class="model-selector" ref={ref}>
      <button
        class="model-selector-trigger"
        onClick={() => !disabled && setOpen(!open)}
        disabled={disabled}
        title="Switch model"
      >
        <span class="model-selector-label">{activeLabel}</span>
        <span class="model-selector-arrow">{open ? "▲" : "▼"}</span>
      </button>

      {open && (
        <div class="model-selector-dropdown">
          {models.length === 0 && (
            <div class="model-selector-empty">No models configured</div>
          )}
          {models.map((m) => (
            <div
              key={m.name}
              class={`model-selector-item ${m.name === active ? "active" : ""}`}
              onClick={() => handleSelect(m.name)}
            >
              <span class="model-selector-item-name">{m.name}</span>
              <span class="model-selector-item-meta">{m.type} / {m.model_name}</span>
              {m.name === active && <span class="model-selector-check">✓</span>}
            </div>
          ))}

          <div class="model-selector-divider" />
          <div class="model-selector-item model-selector-add" onClick={handleOpenAdd}>
            + Add model
          </div>
          {models.length > 0 && (
            <div class="model-selector-item model-selector-edit" onClick={() => setShowEditSub(!showEditSub)}>
              <span class="model-selector-edit-label">Edit model</span>
              <span class="model-selector-sub-arrow">{showEditSub ? "▼" : "▶"}</span>
            </div>
          )}
          {showEditSub &&
            models.map((m) => (
              <div
                key={`edit-${m.name}`}
                class="model-selector-item model-selector-sub-item"
                onClick={() => handleOpenEdit(m)}
              >
                <span class="model-selector-sub-icon">✎</span>
                <span class="model-selector-item-name">{m.name}</span>
                <span class="model-selector-item-meta">{m.type}</span>
              </div>
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
