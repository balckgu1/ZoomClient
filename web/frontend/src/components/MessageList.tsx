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
  // onSuggestion 点击空状态推荐卡片时触发，直接把起始提示词发送给智能体
  onSuggestion?: (prompt: string) => void;
}

// 空状态推荐卡片：图标、标题、说明与点击后发送的起始提示词。
// 参考设计图的四张彩色卡片，帮助用户快速上手。
const SUGGESTIONS: { icon: string; tone: string; title: string; desc: string; prompt: string }[] = [
  {
    icon: "🔍",
    tone: "blue",
    title: "探索并理解代码",
    desc: "梳理项目结构与关键模块",
    prompt: "请帮我梳理这个项目的整体结构和关键模块。",
  },
  {
    icon: "🛠",
    tone: "green",
    title: "构建新功能",
    desc: "从零实现一个功能、应用或工具",
    prompt: "我想新增一个功能，请先帮我确认需求，再给出实现方案。",
  },
  {
    icon: "📝",
    tone: "amber",
    title: "审查代码",
    desc: "审查改动并提出优化建议",
    prompt: "请审查当前代码，指出潜在问题并给出修改建议。",
  },
  {
    icon: "🐞",
    tone: "red",
    title: "修复问题",
    desc: "定位并修复 Bug 或失败用例",
    prompt: "请帮我定位并修复当前存在的问题或失败的测试。",
  },
];

// renderMessage 按消息角色分发到对应的展示组件。
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
          <span class="system-icon">🤖</span> 子智能体：{msg.prompt}
        </div>
      );
    case "hook_blocked":
      return (
        <div key={msg._id} class="message system-message hook-blocked">
          ⚠️ 钩子拦截：{msg.tool}（{msg.reason}）
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

export function MessageList({ messages, agentPhase, toolName, onSuggestion }: Props) {
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
          <h2 class="empty-state__title">今天想做点什么？</h2>
          <p class="empty-state__desc">选择一个起点，或直接在下方输入你的需求。</p>

          <div class="empty-state__cards">
            {SUGGESTIONS.map((s) => (
              <button
                key={s.title}
                class={`suggestion-card suggestion-card--${s.tone}`}
                onClick={() => onSuggestion?.(s.prompt)}
              >
                <span class="suggestion-card__icon">{s.icon}</span>
                <span class="suggestion-card__body">
                  <span class="suggestion-card__title">{s.title}</span>
                  <span class="suggestion-card__desc">{s.desc}</span>
                </span>
              </button>
            ))}
          </div>

          <div class="empty-state__hints">
            <span class="hint-tag"><code>/clear</code> 清空历史</span>
            <span class="hint-tag"><code>/compact</code> 压缩上下文</span>
            <span class="hint-tag"><code>/exit</code> 退出会话</span>
          </div>
        </div>
      )}
      {messages.map((msg) => renderMessage(msg))}
      <AgentStatus phase={agentPhase} toolName={toolName} />
      <div ref={endRef} />
    </main>
  );
}
