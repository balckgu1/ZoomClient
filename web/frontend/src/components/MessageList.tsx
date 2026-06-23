import { useEffect, useRef } from "preact/hooks";
import type { ChatMessage } from "../types";
import type { AgentPhase } from "./AgentStatus";
import { UserMessage } from "./UserMessage";
import { AssistantMessage } from "./AssistantMessage";
import { ReasoningBlock } from "./ReasoningBlock";
import { ToolCallCard } from "./ToolCallCard";
import { AgentStatus } from "./AgentStatus";

interface Props {
  messages: ChatMessage[];
  agentPhase: AgentPhase;
  toolName?: string;
}

function renderMessage(msg: ChatMessage, i: number) {
  switch (msg.role) {
    case "user":
      return <UserMessage key={i} content={msg.content} />;
    case "assistant":
      return <AssistantMessage key={i} content={msg.content} streaming={msg.streaming} />;
    case "reasoning":
      return <ReasoningBlock key={i} content={msg.content} />;
    case "tool_call":
      return (
        <ToolCallCard
          key={i}
          name={msg.name}
          args={msg.args}
          result={msg.result}
          isError={msg.isError}
        />
      );
    case "sub_agent":
      return (
        <div key={i} class="message system-message">
          <span class="system-icon">🤖</span> Sub-agent: {msg.prompt}
        </div>
      );
    case "hook_blocked":
      return (
        <div key={i} class="message system-message hook-blocked">
          ⚠️ Hook blocked: {msg.tool} ({msg.reason})
        </div>
      );
    case "system":
      return (
        <div key={i} class="message system-message">
          {msg.content}
        </div>
      );
    default:
      return null;
  }
}

export function MessageList({ messages, agentPhase, toolName }: Props) {
  const endRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    endRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [messages.length, agentPhase]);

  return (
    <main class="message-list">
      {messages.length === 0 && agentPhase === "idle" && (
        <div class="empty-state">
          <div class="empty-state__logo">⚡</div>
          <h2 class="empty-state__title">ZoomClient</h2>
          <p class="empty-state__desc">Send a message to start the conversation.</p>
          <div class="empty-state__hints">
            <span class="hint-tag"><code>/clear</code> Clear history</span>
            <span class="hint-tag"><code>/compact</code> Compact context</span>
            <span class="hint-tag"><code>/exit</code> Exit session</span>
          </div>
        </div>
      )}
      {messages.map((msg, i) => renderMessage(msg, i))}
      <AgentStatus phase={agentPhase} toolName={toolName} />
      <div ref={endRef} />
    </main>
  );
}
