import { useEffect, useRef } from "preact/hooks";
import type { ChatMessage } from "../types";
import { UserMessage } from "./UserMessage";
import { AssistantMessage } from "./AssistantMessage";
import { ReasoningBlock } from "./ReasoningBlock";
import { ToolCallCard } from "./ToolCallCard";

interface Props {
  messages: ChatMessage[];
}

function renderMessage(msg: ChatMessage, i: number) {
  switch (msg.role) {
    case "user":
      return <UserMessage key={i} content={msg.content} />;
    case "assistant":
      return <AssistantMessage key={i} content={msg.content} />;
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

export function MessageList({ messages }: Props) {
  const endRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    endRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [messages.length]);

  return (
    <main class="message-list">
      {messages.length === 0 && (
        <div class="empty-state">
          <p>Send a message to start the conversation.</p>
          <p class="hint">Use <code>/clear</code>, <code>/compact</code>, or <code>/exit</code> for commands.</p>
        </div>
      )}
      {messages.map((msg, i) => renderMessage(msg, i))}
      <div ref={endRef} />
    </main>
  );
}
