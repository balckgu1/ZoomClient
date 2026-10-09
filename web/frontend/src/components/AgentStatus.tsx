export type AgentPhase = "idle" | "thinking" | "handling" | "streaming" | "memory";

interface Props {
  phase: AgentPhase;
  toolName?: string;
}

// AgentStatus —— 对话流底部的实时状态行，色轨与推理/工具保持一致。
// 各相位使用同一套运动语言：思考=三点呼吸、处理=括号旋转、流式=光标闪烁、记忆=旋转。
export function AgentStatus({ phase, toolName }: Props) {
  if (phase === "idle") return null;

  return (
    <div class={`agent-status agent-status--${phase}`}>
      {phase === "thinking" && (
        <span class="agent-status__dots" aria-hidden="true">
          <i /><i /><i />
        </span>
      )}
      {phase === "handling" && <span class="agent-status__spin" aria-hidden="true" />}
      {phase === "streaming" && <span class="caret" aria-hidden="true" />}
      {phase === "memory" && <span class="agent-status__spin" aria-hidden="true" />}
      <span class="agent-status__label">
        {phase === "thinking" && "思考中…"}
        {phase === "handling" && (
          <>正在处理{toolName ? <code class="agent-status__tool">{toolName}</code> : null}…</>
        )}
        {phase === "streaming" && "输出中…"}
        {phase === "memory" && "记忆更新中…"}
      </span>
    </div>
  );
}
