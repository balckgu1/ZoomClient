export type AgentPhase = "idle" | "thinking" | "handling" | "streaming";

interface Props {
  phase: AgentPhase;
  toolName?: string;
}

export function AgentStatus({ phase, toolName }: Props) {
  if (phase === "idle") return null;

  return (
    <div class={`agent-status agent-status--${phase}`}>
      <div class="agent-status__avatar">
        {phase === "thinking" && (
          <span class="agent-status__dots">
            <span class="dot dot-1" />
            <span class="dot dot-2" />
            <span class="dot dot-3" />
          </span>
        )}
        {phase === "handling" && <span class="agent-status__spinner" />}
        {phase === "streaming" && <span class="agent-status__cursor">|</span>}
      </div>
      <span class="agent-status__label">
        {phase === "thinking" && "Thinking..."}
        {phase === "handling" && (
          <>Handling{toolName ? <><code class="agent-status__tool-name">{toolName}</code></> : null}...</>
        )}
        {phase === "streaming" && "Streaming..."}
      </span>
    </div>
  );
}
