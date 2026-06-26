import { useEffect, useRef, useCallback } from "preact/hooks";
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

function renderMessage(msg: ChatMessage) {
  switch (msg.role) {
    case "user":
      return <UserMessage key={msg._id} content={msg.content} />;
    case "assistant":
      return <AssistantMessage key={msg._id} content={msg.content} streaming={msg.streaming} />;
    case "reasoning":
      return <ReasoningBlock key={msg._id} content={msg.content} />;
    case "tool_call":
      return (
        <ToolCallCard
          key={msg._id}
          name={msg.name}
          args={msg.args}
          result={msg.result}
          isError={msg.isError}
        />
      );
    case "sub_agent":
      return (
        <div key={msg._id} class="message system-message">
          <span class="system-icon">🤖</span> Sub-agent: {msg.prompt}
        </div>
      );
    case "hook_blocked":
      return (
        <div key={msg._id} class="message system-message hook-blocked">
          ⚠️ Hook blocked: {msg.tool} ({msg.reason})
        </div>
      );
    case "system":
      return (
        <div key={msg._id} class="message system-message">
          {msg.content}
        </div>
      );
    default:
      return null;
  }
}

export function MessageList({ messages, agentPhase, toolName }: Props) {
  const endRef = useRef<HTMLDivElement>(null);
  const listRef = useRef<HTMLElement>(null);
  const userScrolledUp = useRef(false);

  // 判断用户是否在底部附近（距离底部 80px 以内视为"在底部"）
  const isNearBottom = useCallback(() => {
    const el = listRef.current;
    if (!el) return true;
    return el.scrollHeight - el.scrollTop - el.clientHeight < 80;
  }, []);

  // 监听用户手动滚动
  useEffect(() => {
    const el = listRef.current;
    if (!el) return;
    const handleScroll = () => {
      userScrolledUp.current = !isNearBottom();
    };
    el.addEventListener("scroll", handleScroll, { passive: true });
    return () => el.removeEventListener("scroll", handleScroll);
  }, [isNearBottom]);

  // 自动滚动：仅在用户处于底部附近时触发
  useEffect(() => {
    if (!userScrolledUp.current) {
      endRef.current?.scrollIntoView({ behavior: "smooth" });
    }
  }, [messages.length, agentPhase]);

  return (
    <main class="message-list" ref={listRef}>
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
      {messages.map((msg) => renderMessage(msg))}
      <AgentStatus phase={agentPhase} toolName={toolName} />
      <div ref={endRef} />
    </main>
  );
}
