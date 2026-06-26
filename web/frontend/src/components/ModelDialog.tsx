import { useState, useEffect } from "preact/hooks";
import type { ModelPreset } from "../types";
import { testModel } from "../lib/api";

interface Props {
  mode: "add" | "edit";
  initial?: ModelPreset;
  onSave: (preset: ModelPreset) => void;
  onClose: () => void;
}

export function ModelDialog({ mode, initial, onSave, onClose }: Props) {
  const [name, setName] = useState("");
  const [type, setType] = useState("openai");
  const [baseURL, setBaseURL] = useState("");
  const [apiKey, setApiKey] = useState("");
  const [modelName, setModelName] = useState("");
  const [testing, setTesting] = useState(false);
  const [testResult, setTestResult] = useState<{ ok: boolean; msg: string } | null>(null);

  useEffect(() => {
    if (initial) {
      setName(initial.name);
      setType(initial.type);
      setBaseURL(initial.base_url || "");
      setApiKey(initial.api_key || "");
      setModelName(initial.model_name);
    }
  }, [initial]);

  const handleSave = () => {
    if (!name.trim()) return;
    onSave({
      name: name.trim(),
      type,
      base_url: baseURL.trim() || undefined,
      api_key: apiKey.trim() || undefined,
      model_name: modelName.trim() || name.trim(),
    });
  };

  const handleTest = async () => {
    setTesting(true);
    setTestResult(null);
    try {
      const result = await testModel({
        name: "_test_",
        type,
        base_url: baseURL.trim() || undefined,
        api_key: apiKey.trim() || undefined,
        model_name: modelName.trim() || "_test_",
      });
      setTestResult({
        ok: result.status === "ok",
        msg: result.message,
      });
    } catch (err) {
      setTestResult({ ok: false, msg: String(err) });
    } finally {
      setTesting(false);
    }
  };

  const title = mode === "add" ? "Add Model" : "Edit Model";

  return (
    <div class="modal-overlay" onClick={onClose}>
      <div class="model-dialog" onClick={(e) => e.stopPropagation()}>
        <div class="model-dialog__header">
          <h3>{title}</h3>
          <button class="model-dialog__close" onClick={onClose}>&times;</button>
        </div>
        <div class="model-dialog__body">
          <label class="model-dialog__field">
            <span class="model-dialog__label">Name</span>
            <input
              value={name}
              onInput={(e) => setName((e.target as HTMLInputElement).value)}
              placeholder="Display name"
              disabled={mode === "edit"}
            />
          </label>
          <label class="model-dialog__field">
            <span class="model-dialog__label">Type</span>
            <select value={type} onChange={(e) => setType((e.target as HTMLSelectElement).value)}>
              <option value="openai">OpenAI</option>
              <option value="ollama">Ollama</option>
              <option value="anthropic">Anthropic</option>
              <option value="gemini">Gemini</option>
            </select>
          </label>
          {type !== "ollama" && (
            <label class="model-dialog__field">
              <span class="model-dialog__label">Base URL</span>
              <input
                value={baseURL}
                onInput={(e) => setBaseURL((e.target as HTMLInputElement).value)}
                placeholder={type === "openai" ? "https://api.openai.com" : ""}
              />
            </label>
          )}
          {(type === "anthropic" || type === "gemini" || type === "openai") && (
            <label class="model-dialog__field">
              <span class="model-dialog__label">API Key</span>
              <input
                type="password"
                value={apiKey}
                onInput={(e) => setApiKey((e.target as HTMLInputElement).value)}
                placeholder="sk-..."
              />
            </label>
          )}
          <label class="model-dialog__field">
            <span class="model-dialog__label">Model name</span>
            <input
              value={modelName}
              onInput={(e) => setModelName((e.target as HTMLInputElement).value)}
              placeholder={name || "e.g. gpt-4o"}
            />
          </label>

          {testResult && (
            <div class={`model-dialog__test-result ${testResult.ok ? "test-ok" : "test-fail"}`}>
              {testResult.ok ? "✅ " : "❌ "}
              {testResult.msg}
            </div>
          )}
        </div>
        <div class="model-dialog__footer">
          <button class="model-dialog__btn btn-test" onClick={handleTest} disabled={testing}>
            {testing ? "Testing..." : "Test"}
          </button>
          <div class="model-dialog__footer-right">
            <button class="model-dialog__btn btn-cancel" onClick={onClose}>Cancel</button>
            <button class="model-dialog__btn btn-save" onClick={handleSave} disabled={!name.trim()}>
              Save
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}
