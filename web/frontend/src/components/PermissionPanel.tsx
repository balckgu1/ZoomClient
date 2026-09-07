import { useState, useEffect } from "preact/hooks";
import type { PermissionConfig, PermissionMode, PermissionRule } from "../types";
import { PERMISSION_MODES, permissionModeMeta } from "../lib/labels";

// PermissionPanel 的 props
interface Props {
  // config 为后端返回的当前权限快照
  config: PermissionConfig;
  // onSave 提交完整配置（模式 + 拒绝/允许规则），后端同步生效
  onSave: (cfg: {
    mode: PermissionMode;
    deny_rules: PermissionRule[];
    allow_rules: PermissionRule[];
  }) => Promise<void> | void;
  onClose: () => void;
}

// 空规则模板，用于"添加规则"表单初始化
const EMPTY_RULE: PermissionRule = { tool: "", behavior: "", path: "", content: "" };

// PermissionPanel —— 权限管理面板（模态，完整规则编辑器）。
// 支持：切换权限模式、对拒绝规则与允许规则进行增删改，保存后运行时生效。
export function PermissionPanel({ config, onSave, onClose }: Props) {
  const [mode, setMode] = useState<PermissionMode>(config.mode);
  const [denyRules, setDenyRules] = useState<PermissionRule[]>(config.deny_rules || []);
  const [allowRules, setAllowRules] = useState<PermissionRule[]>(config.allow_rules || []);
  // editing 记录正在编辑的规则所属分区与下标；adding 记录正在新增的分区
  const [adding, setAdding] = useState<"deny" | "allow" | null>(null);
  const [draft, setDraft] = useState<PermissionRule>(EMPTY_RULE);
  const [saving, setSaving] = useState(false);

  // 外部 config 变化时重置本地编辑状态，避免展示陈旧数据
  useEffect(() => {
    setMode(config.mode);
    setDenyRules(config.deny_rules || []);
    setAllowRules(config.allow_rules || []);
  }, [config]);

  // beginAdd 打开指定分区的新增表单
  const beginAdd = (section: "deny" | "allow") => {
    setAdding(section);
    setDraft({ ...EMPTY_RULE });
  };

  // commitAdd 将草稿写入对应分区列表
  const commitAdd = () => {
    if (!adding) return;
    const rule: PermissionRule = {
      tool: draft.tool.trim(),
      behavior: adding === "deny" ? "deny" : "allow",
      path: draft.path.trim(),
      content: draft.content.trim(),
    };
    if (adding === "deny") setDenyRules((prev) => [...prev, rule]);
    else setAllowRules((prev) => [...prev, rule]);
    setAdding(null);
    setDraft(EMPTY_RULE);
  };

  // removeRule 删除指定分区的某条规则
  const removeRule = (section: "deny" | "allow", index: number) => {
    if (section === "deny") setDenyRules((prev) => prev.filter((_, i) => i !== index));
    else setAllowRules((prev) => prev.filter((_, i) => i !== index));
  };

  // handleSave 提交完整配置，成功后关闭面板
  const handleSave = async () => {
    setSaving(true);
    try {
      await onSave({ mode, deny_rules: denyRules, allow_rules: allowRules });
      onClose();
    } finally {
      setSaving(false);
    }
  };

  const meta = permissionModeMeta(mode);

  return (
    <div class="modal-overlay" onClick={onClose}>
      <div class="panel-dialog panel-dialog--wide" onClick={(e) => e.stopPropagation()}>
        <div class="panel-dialog__header">
          <h3>权限管理</h3>
          <button class="panel-dialog__close" onClick={onClose} title="关闭">&times;</button>
        </div>

        <div class="panel-dialog__body">
          {/* ── 权限模式切换 ── */}
          <div class="field">
            <span class="field__label">权限模式</span>
            <div class="mode-segmented">
              {PERMISSION_MODES.map((m) => (
                <button
                  key={m.value}
                  class={`mode-btn ${mode === m.value ? `mode-btn--active tone-${m.tone}` : ""}`}
                  onClick={() => setMode(m.value)}
                  title={m.hint}
                >
                  {m.label}
                </button>
              ))}
            </div>
            <p class="field__hint">{meta.hint}</p>
          </div>

          {/* ── 拒绝规则 ── */}
          <RuleSection
            title="拒绝规则"
            tone="danger"
            desc="命中后直接拒绝，优先级高于允许规则"
            rules={denyRules}
            onRemove={(i) => removeRule("deny", i)}
            onAdd={() => beginAdd("deny")}
            adding={adding === "deny"}
            draft={draft}
            setDraft={setDraft}
            onCommit={commitAdd}
            onCancel={() => setAdding(null)}
          />

          {/* ── 允许规则 ── */}
          <RuleSection
            title="允许规则"
            tone="safe"
            desc="命中后自动放行，无需询问"
            rules={allowRules}
            onRemove={(i) => removeRule("allow", i)}
            onAdd={() => beginAdd("allow")}
            adding={adding === "allow"}
            draft={draft}
            setDraft={setDraft}
            onCommit={commitAdd}
            onCancel={() => setAdding(null)}
          />
        </div>

        <div class="panel-dialog__footer">
          <button class="btn btn--ghost" onClick={onClose} disabled={saving}>取消</button>
          <button class="btn btn--primary" onClick={handleSave} disabled={saving}>
            {saving ? "保存中…" : "保存并生效"}
          </button>
        </div>
      </div>
    </div>
  );
}

// RuleSection 的 props —— 单个规则分区（拒绝 / 允许）的展示与编辑
interface RuleSectionProps {
  title: string;
  tone: "danger" | "safe";
  desc: string;
  rules: PermissionRule[];
  onRemove: (index: number) => void;
  onAdd: () => void;
  adding: boolean;
  draft: PermissionRule;
  setDraft: (r: PermissionRule) => void;
  onCommit: () => void;
  onCancel: () => void;
}

// RuleSection —— 渲染规则列表、删除按钮，以及内联的"添加规则"表单。
// 抽成独立组件以便拒绝/允许两个分区复用同一套逻辑与样式。
function RuleSection({
  title, tone, desc, rules, onRemove, onAdd, adding, draft, setDraft, onCommit, onCancel,
}: RuleSectionProps) {
  return (
    <div class="rule-section">
      <div class="rule-section__head">
        <div>
          <span class={`rule-section__title tone-text-${tone}`}>{title}</span>
          <span class="rule-section__desc">{desc}</span>
        </div>
        <button class="btn btn--small btn--ghost" onClick={onAdd}>+ 添加</button>
      </div>

      {rules.length === 0 && !adding && (
        <div class="rule-empty">暂无规则</div>
      )}

      {rules.map((r, i) => (
        <div key={i} class="rule-row">
          <div class="rule-row__main">
            <code class="rule-tool">{r.tool || "*"}</code>
            {r.path && <span class="rule-tag">path: {r.path}</span>}
            {r.content && <span class="rule-tag">content: {r.content}</span>}
          </div>
          <button class="rule-row__del" title="删除" onClick={() => onRemove(i)}>🗑</button>
        </div>
      ))}

      {adding && (
        <div class="rule-form">
          <input
            class="field__input"
            value={draft.tool}
            onInput={(e) => setDraft({ ...draft, tool: (e.target as HTMLInputElement).value })}
            placeholder="工具名（留空或 * 表示任意工具）"
            autoFocus
          />
          <input
            class="field__input"
            value={draft.path}
            onInput={(e) => setDraft({ ...draft, path: (e.target as HTMLInputElement).value })}
            placeholder="路径匹配（可选，支持 re: 前缀正则）"
          />
          <input
            class="field__input"
            value={draft.content}
            onInput={(e) => setDraft({ ...draft, content: (e.target as HTMLInputElement).value })}
            placeholder="内容匹配（可选，如命令片段，支持 re: 前缀）"
          />
          <div class="rule-form__actions">
            <button class="btn btn--small btn--ghost" onClick={onCancel}>取消</button>
            <button class="btn btn--small btn--primary" onClick={onCommit}>确定</button>
          </div>
        </div>
      )}
    </div>
  );
}
