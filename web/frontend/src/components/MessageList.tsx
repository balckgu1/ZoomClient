import { useEffect, useRef, useCallback } from "preact/hooks";
import type { ChatMessage } from "../types";
import type { AgentPhase } from "./AgentStatus";
import { UserMessage } from "./UserMessage";
import { AssistantMessage } from "./AssistantMessage";
import { ReasoningBlock } from "./ReasoningBlock";
import { ToolCallCard } from "./ToolCallCard";
import { AgentStatus } from "./AgentStatus";
import {
  IconBook, IconBug, IconHammer, IconPulse, IconAlert, IconBot,
} from "../lib/icons";
import type { ComponentChildren } from "preact";

interface Props {
  messages: ChatMessage[];
  agentPhase: AgentPhase;
  toolName?: string;
  // onSuggestion 点击空状态推荐卡片时触发，直接把起始提示词发送给智能体
  onSuggestion?: (prompt: string) => void;
}

// 空状态起点卡：四张 2×2 网格，仅图标着色，靠发丝线而非彩色描边分区。
// 每张卡点下去就是一句初始提示词，让"完全空白"变成"从哪开始"。
const STARTS: { icon: ComponentChildren; tone: string; title: string; desc: string; prompt: string }[] = [
  {
    icon: <IconPulse size={16} />,
    tone: "explore",
    title: "探索并理解代码",
    desc: "梳理项目结构与关键模块",
    prompt: "请帮我梳理这个项目的整体结构和关键模块。",
  },
  {
    icon: <IconHammer size={16} />,
    tone: "build",
    title: "构建新功能",
    desc: "从零实现一个功能、应用或工具",
    prompt: "我想新增一个功能，请先帮我确认需求，再给出实现方案。",
  },
  {
    icon: <IconBook size={16} />,
    tone: "review",
    title: "审查代码",
    desc: "审查改动并提出优化建议",
    prompt: "请审查当前代码，指出潜在问题并给出修改建议。",
  },
  {
    icon: <IconBug size={16} />,
    tone: "fix",
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
        <div key={msg._id} class="msg-note msg-note--sub">
          <IconBot size={13} />
          <span class="msg-note__body">子智能体：{msg.prompt}</span>
        </div>
      );
    case "hook_blocked":
      return (
        <div key={msg._id} class="msg-note msg-note--block">
          <IconAlert size={13} />
          <span class="msg-note__body">钩子拦截：{msg.tool}（{msg.reason}）</span>
        </div>
      );
    case "system":
      return (
        <div key={msg._id} class="msg-note">
          <IconPulse size={13} />
          <span class="msg-note__body">{msg.content}</span>
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
    <main class="thread" ref={listRef}>
      <div class="thread__inner">
        {/* 空状态：起点卡 2×2 + 一句 "/" 提示 */}
        {messages.length === 0 && agentPhase === "idle" && (
          <div class="empty">
            <span class="empty__mark"><IconPulse size={20} /></span>
            <h2 class="empty__title">今天想做点什么？</h2>
            <p class="empty__desc">选一个起点，或直接在下方输入你的需求。</p>

            <div class="empty__grid">
              {STARTS.map((s) => (
                <button
                  key={s.title}
                  class={`start start--${s.tone}`}
                  onClick={() => onSuggestion?.(s.prompt)}
                >
                  <span class="start__icon">{s.icon}</span>
                  <span class="start__body">
                    <span class="start__title">{s.title}</span>
                    <span class="start__desc">{s.desc}</span>
                  </span>
                </button>
              ))}
            </div>

            <p class="empty__hint">
              键入 <code>/</code> 调出命令与技能
            </p>
          </div>
        )}

        {messages.map((msg) => renderMessage(msg))}
        <AgentStatus phase={agentPhase} toolName={toolName} />
        <div ref={endRef} />
      </div>
    </main>
  );
}
