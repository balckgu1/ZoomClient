import { useState, useRef, useEffect } from "preact/hooks";
import type { ModelPreset } from "../types";

interface Props {
  models: ModelPreset[];
  active: string;
  onSelect: (name: string) => void;
  onAdd: (preset: ModelPreset) => void;
  disabled?: boolean;
}

export function ModelSelector({ models, active, onSelect, onAdd, disabled }: Props) {
  const [open, setOpen] = useState(false);
  const [showForm, setShowForm] = useState(false);
  const [formName, setFormName] = useState("");
  const [formType, setFormType] = useState("openai");
  const [formURL, setFormURL] = useState("");
  const [formKey, setFormKey] = useState("");
  const [formModel, setFormModel] = useState("");
  const ref = useRef<HTMLDivElement>(null);

  // Close dropdown when clicking outside
  useEffect(() => {
    const handler = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) {
        setOpen(false);
        setShowForm(false);
      }
    };
    document.addEventListener("mousedown", handler);
    return () => document.removeEventListener("mousedown", handler);
  }, []);

  const handleSelect = (name: string) => {
    onSelect(name);
    setOpen(false);
  };

  const handleAdd = () => {
    if (!formName.trim()) return;
    onAdd({
      name: formName.trim(),
      type: formType,
      base_url: formURL.trim() || undefined,
      api_key: formKey.trim() || undefined,
      model_name: formModel.trim() || formName.trim(),
    });
    setFormName("");
    setFormType("openai");
    setFormURL("");
    setFormKey("");
    setFormModel("");
    setShowForm(false);
    setOpen(false);
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

          {!showForm ? (
            <div
              class="model-selector-item model-selector-add"
              onClick={() => setShowForm(true)}
            >
              + Add model
            </div>
          ) : (
            <div class="model-selector-form">
              <input
                placeholder="Name"
                value={formName}
                onInput={(e) => setFormName((e.target as HTMLInputElement).value)}
              />
              <select value={formType} onChange={(e) => setFormType((e.target as HTMLSelectElement).value)}>
                <option value="openai">OpenAI</option>
                <option value="ollama">Ollama</option>
                <option value="anthropic">Anthropic</option>
                <option value="gemini">Gemini</option>
              </select>
              <input
                placeholder="Base URL"
                value={formURL}
                onInput={(e) => setFormURL((e.target as HTMLInputElement).value)}
              />
              <input
                placeholder="API Key"
                type="password"
                value={formKey}
                onInput={(e) => setFormKey((e.target as HTMLInputElement).value)}
              />
              <input
                placeholder="Model name (optional)"
                value={formModel}
                onInput={(e) => setFormModel((e.target as HTMLInputElement).value)}
              />
              <button class="model-selector-form-btn" onClick={handleAdd}>
                Save
              </button>
            </div>
          )}
        </div>
      )}
    </div>
  );
}
