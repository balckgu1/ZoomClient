import { useState, useEffect, useCallback, useRef } from "preact/hooks";
import type {
  ChatMessage, PermissionAsk, SSEEvent, SessionMeta, ModelPreset,
  PermissionConfig, PermissionMode, ContextUsage, SkillMeta,
} from "../types";
import type { AgentPhase } from "./AgentStatus";
import { connectSSE } from "../lib/sse";
import { reconstructMessages } from "../lib/history";
import { findSkill, parseSlashInput, skillPrompt } from "../lib/slash";
import {
  sendChat, sendClear, sendCompact, sendExit, sendStop, sendPermission,
  fetchSessions, createSession, loadSession, deleteSession, renameSession,
  fetchModels, addModel, selectModel, updateModel,
  fetchWorkDir, setWorkDir, fetchPermissionConfig, updatePermissionConfig,
  fetchContextUsage, fetchSkills,
} from "../lib/api";
import { StatusBar } from "./StatusBar";
import { MessageList } from "./MessageList";
import { Composer } from "./Composer";
import { PermissionDialog } from "./PermissionDialog";
import { WorkDirPanel } from "./WorkDirPanel";
import { PermissionPanel } from "./PermissionPanel";
import { Sidebar } from "./Sidebar";
import { IconAlert, IconCheck, IconSpark } from "../lib/icons";

// ToastTone 是轻提示的三种语气：普通消息、成功、失败。
type ToastTone = "info" | "ok" | "error";

// ToastItem 队列条目：文本 + 语气。
interface ToastItem {
  text: string;
  tone: ToastTone;
}

export function App() {
  // 按会话维护独立的消息缓冲区：切换会话只改变"显示哪个缓冲"，不丢弃任何会话的实时状态。
  // 这是修复"切走再切回后工具调用/思考态丢失"的核心——旧实现用单一全局 messages 数组，
  // 每次切换都从磁盘整体替换，既丢在途状态又对有损历史无法还原工具卡片。
  const [buffers, setBuffers] = useState<Record<string, ChatMessage[]>>({});
  const [turnCounts, setTurnCounts] = useState<Record<string, number>>({});
  const [connected, setConnected] = useState(false);
  const [busy, setBusy] = useState(false);
  const [model, setModel] = useState("");
  const [permission, setPermission] = useState<PermissionAsk | null>(null);

  // runningSessionId：当前正在跑 agentLoop 的会话 id。SSE 事件与打字机都写入该会话缓冲，
  // 即使用户此刻正在查看别的会话，运行会话的流式内容也能持续累积、切回即恢复。
  const runningSessionId = useRef<string>("");
  // busyRef：busy 的 ref 镜像，供 useCallback（如 handleSelectSession）同步读取，
  // 避免把 busy 放进依赖导致回调频繁重建。
  const busyRef = useRef(false);
  // buffersRef：buffers 的 ref 镜像，供 handleSelectSession 判断目标会话是否已有缓存。
  const buffersRef = useRef<Record<string, ChatMessage[]>>({});
  // Toast 队列：多条提示排队依次展示。
  // tone 决定图标与描边色——成功、失败、普通消息不该长成同一个样子。
  const [toastQueue, setToastQueue] = useState<ToastItem[]>([]);
  const [currentToast, setCurrentToast] = useState<ToastItem | null>(null);

  // skills 为后端已加载的技能目录，驱动输入框 "/" 扩展框的技能分区
  const [skills, setSkills] = useState<SkillMeta[]>([]);

  // Session state
  const [sessions, setSessions] = useState<SessionMeta[]>([]);
  const [currentSessionId, setCurrentSessionId] = useState("");

  // Model state
  const [models, setModels] = useState<ModelPreset[]>([]);
  const [activeModel, setActiveModel] = useState("");

  // 工作目录与权限配置状态（来自后端，可运行时修改）
  const [workDir, setWorkDirState] = useState("");
  const [permissionConfig, setPermissionConfig] = useState<PermissionConfig>({
    mode: "default" as PermissionMode,
    interactive: false,
    deny_rules: [],
    allow_rules: [],
  });
  // 控制面板模态框的显示
  const [showWorkDir, setShowWorkDir] = useState(false);
  const [showPermission, setShowPermission] = useState(false);

  // Agent phase state (for status indicator)
  const [agentPhase, setAgentPhase] = useState<AgentPhase>("idle");
  const [currentToolName, setCurrentToolName] = useState<string>("");

  // 上下文窗口占用快照（后端 SSE 推送 + 首屏拉取）
  const [contextUsage, setContextUsage] = useState<ContextUsage | null>(null);

  // Typewriter effect refs
  const typewriterTimer = useRef<ReturnType<typeof setInterval> | null>(null);
  const pendingText = useRef<string>("");
  const currentIndex = useRef<number>(0);
  const streamingMsgIdx = useRef<number>(-1);

  // Message ID counter for stable React keys
  const nextMsgId = useRef(0);
  const genId = useCallback(() => { nextMsgId.current += 1; return nextMsgId.current; }, []);

  // 同步 buffersRef 镜像，供回调中读取最新缓冲而无需加入依赖
  useEffect(() => { buffersRef.current = buffers; }, [buffers]);

  // updateRunning 把一次消息变更应用到"运行会话"的缓冲区。
  // SSE 事件与打字机都通过它写入，保证运行会话的内容不会串到当前正在查看的其它会话。
  const updateRunning = useCallback((fn: (msgs: ChatMessage[]) => ChatMessage[]) => {
    const sid = runningSessionId.current;
    if (!sid) return;
    setBuffers((prev) => ({ ...prev, [sid]: fn(prev[sid] ?? []) }));
  }, []);

  // ─── 显示态派生：始终展示"当前选中会话"的缓冲与运行状态 ───
  const messages = currentSessionId ? (buffers[currentSessionId] ?? []) : [];
  const turnCount = currentSessionId ? (turnCounts[currentSessionId] ?? 0) : 0;
  // 仅当查看的正是运行中的会话时，才展示"思考中/流式"等实时相位与当前工具名；
  // 查看其它会话时应显示为 idle，避免把别的会话的运行态误显示到这里。
  const viewingRunning = !!currentSessionId && currentSessionId === runningSessionId.current;
  const displayPhase: AgentPhase = viewingRunning ? agentPhase : "idle";
  const displayTool = viewingRunning ? currentToolName : "";

  // Finish typewriter: flush remaining text immediately
  const finishTypewriter = useCallback(() => {
    if (typewriterTimer.current) {
      clearInterval(typewriterTimer.current);
      typewriterTimer.current = null;
    }
    const remaining = pendingText.current;
    const idx = currentIndex.current;
    const msgIdx = streamingMsgIdx.current;
    if (msgIdx >= 0 && idx < remaining.length) {
      updateRunning((prev) => {
        const updated = [...prev];
        if (updated[msgIdx] && updated[msgIdx].role === "assistant") {
          updated[msgIdx] = { ...updated[msgIdx], content: remaining, streaming: false } as ChatMessage;
        }
        return updated;
      });
    }
    currentIndex.current = 0;
    pendingText.current = "";
    streamingMsgIdx.current = -1;
  }, [updateRunning]);

  // showToast 入队一条轻提示。tone 缺省为 info，只有明确的成败才需要指定。
  const showToast = useCallback((text: string, tone: ToastTone = "info") => {
    setToastQueue((prev) => [...prev, { text, tone }]);
  }, []);

  // Process toast queue: show next when current disappears
  useEffect(() => {
    if (currentToast === null && toastQueue.length > 0) {
      const [next, ...rest] = toastQueue;
      setCurrentToast(next);
      setToastQueue(rest);
      const timer = setTimeout(() => setCurrentToast(null), 3000);
      return () => clearTimeout(timer);
    }
  }, [currentToast, toastQueue]);

  // Refresh session list from backend
  const refreshSessions = useCallback(async () => {
    try {
      const list = await fetchSessions();
      setSessions(list || []);
    } catch {
      // ignore
    }
  }, []);

  // Refresh model list from backend
  const refreshModels = useCallback(async () => {
    try {
      const resp = await fetchModels();
      setModels(resp.models || []);
      setActiveModel(resp.active || "");
    } catch {
      // ignore
    }
  }, []);

  // refreshWorkDir 拉取当前工作目录
  const refreshWorkDir = useCallback(async () => {
    try {
      const dir = await fetchWorkDir();
      setWorkDirState(dir);
    } catch {
      // ignore
    }
  }, []);

  // refreshPermission 拉取当前权限配置快照
  const refreshPermission = useCallback(async () => {
    try {
      const cfg = await fetchPermissionConfig();
      setPermissionConfig(cfg);
    } catch {
      // ignore
    }
  }, []);

  // refreshContextUsage 拉取上下文占用快照缓存（首屏与 SSE 重连后兜底）
  const refreshContextUsage = useCallback(async () => {
    try {
      const usage = await fetchContextUsage();
      setContextUsage(usage);
    } catch {
      // ignore
    }
  }, []);

  // refreshSkills 拉取已加载的技能目录（GET /api/skills），驱动 "/" 扩展框。
  // skill 属于可选能力：目录拉取失败时保持空列表即可，不打扰用户。
  const refreshSkills = useCallback(async () => {
    try {
      const list = await fetchSkills();
      setSkills(list || []);
    } catch {
      // ignore
    }
  }, []);

  // Start typewriter effect for a given assistant message
  const startTypewriter = useCallback((fullText: string, msgIdx: number) => {
    // Clear any existing timer
    if (typewriterTimer.current) clearInterval(typewriterTimer.current);
    pendingText.current = fullText;
    currentIndex.current = 0;
    streamingMsgIdx.current = msgIdx;
    setAgentPhase("streaming");

    // Speed: ~15ms per char for short text, faster for long text
    const speed = fullText.length > 1000 ? 5 : fullText.length > 500 ? 8 : 15;

    typewriterTimer.current = setInterval(() => {
      currentIndex.current += 1;
      const ci = currentIndex.current;
      const text = pendingText.current;
      if (ci >= text.length) {
        // Done streaming
        if (typewriterTimer.current) clearInterval(typewriterTimer.current);
        typewriterTimer.current = null;
        updateRunning((prev) => {
          const updated = [...prev];
          if (updated[msgIdx] && updated[msgIdx].role === "assistant") {
            updated[msgIdx] = { ...updated[msgIdx], content: text, streaming: false } as ChatMessage;
          }
          return updated;
        });
        streamingMsgIdx.current = -1;
        setAgentPhase("idle");
        return;
      }
      updateRunning((prev) => {
        const updated = [...prev];
        if (updated[msgIdx] && updated[msgIdx].role === "assistant") {
          updated[msgIdx] = { ...updated[msgIdx], content: text.slice(0, ci), streaming: true } as ChatMessage;
        }
        return updated;
      });
    }, speed);
  }, [updateRunning]);

  // Handle incoming SSE events
  const handleSSEEvent = useCallback((evt: SSEEvent) => {
    const d = evt.data as Record<string, string>;

    if (evt.ch === "system") {
      const event = d.event;
      if (event === "ready") {
        setModel(d.model || "");
        showToast("会话已开始");
      } else if (event === "info") {
        showToast(d.message || "");
      } else if (event === "error") {
        showToast(`错误 [${d.scope}]：${d.message}`, "error");
      } else if (event === "compact") {
        const before = d.before_bytes || "?";
        const after = d.after_bytes || "?";
        showToast(`已压缩上下文：${before} → ${after} 字节`, "ok");
      } else if (event === "permission_ask") {
        setPermission({
          id: d.id,
          tool: d.tool,
          args: d.args,
          reason: d.reason,
        });
      } else if (event === "session_end") {
        showToast("会话已结束");
      } else if (event === "session_renamed") {
        const id = d.id;
        const title = d.title;
        setSessions((prev) =>
          prev.map((s) => (s.id === id ? { ...s, title } : s))
        );
      } else if (event === "workdir_changed") {
        // 工作目录切换成功后由后端回推，更新本地显示
        setWorkDirState(d.path || "");
        showToast(`工作目录已切换：${d.path || ""}`, "ok");
      } else if (event === "context_usage") {
        // 上下文占用快照：后端每轮/压缩/清空后推送，驱动右下角指示器
        const usage = d.usage as unknown as ContextUsage | undefined;
        if (usage) setContextUsage(usage);
      }
      return;
    }

    if (evt.ch === "emotion") {
      if (d.state === "thinking") {
        setAgentPhase("thinking");
        setCurrentToolName("");
      }
      return;
    }

    if (evt.ch === "agent") {
      const type = d.type;
      if (type === "assistant") {
        // Finish any ongoing typewriter first
        finishTypewriter();
        const fullText = d.content;
        // Add empty assistant message, then start typewriter
        updateRunning((prev) => {
          const updated = [...prev, { _id: genId(), role: "assistant", content: "", streaming: true } as ChatMessage];
          const newIdx = updated.length - 1;
          // Schedule typewriter after state update
          setTimeout(() => startTypewriter(fullText, newIdx), 0);
          return updated;
        });
      } else if (type === "reasoning") {
        updateRunning((prev) => [...prev, { _id: genId(), role: "reasoning", content: d.content }]);
        setAgentPhase("thinking");
      } else if (type === "tool_call") {
        finishTypewriter();
        updateRunning((prev) => [
          ...prev,
          { _id: genId(), role: "tool_call", name: d.name, args: d.args },
        ]);
        setAgentPhase("handling");
        setCurrentToolName(d.name || "");
      } else if (type === "tool_result") {
        updateRunning((prev) => {
          const updated = [...prev];
          for (let i = updated.length - 1; i >= 0; i--) {
            const m = updated[i];
            if (m.role === "tool_call" && m.name === d.name && m.result === undefined) {
              updated[i] = {
                ...m,
                result: d.content,
                isError: d.is_error === "true" || d.is_error === true as unknown as string,
              };
              break;
            }
          }
          return updated;
        });
        // After tool result, back to thinking if still busy
        setAgentPhase("thinking");
        setCurrentToolName("");
      } else if (type === "sub_agent") {
        updateRunning((prev) => [...prev, { _id: genId(), role: "sub_agent", prompt: d.prompt }]);
      } else if (type === "hook_blocked") {
        updateRunning((prev) => [
          ...prev,
          { _id: genId(), role: "hook_blocked", tool: d.tool, reason: d.reason },
        ]);
      } else if (type === "todo_panel") {
        updateRunning((prev) => [
          ...prev,
          { _id: genId(), role: "system", content: `📋 Plan\n${d.content}` },
        ]);
      } else if (type === "done") {
        finishTypewriter();
        busyRef.current = false;
        setBusy(false);
        setAgentPhase("idle");
        setCurrentToolName("");
        // 轮次计数归属到运行会话
        const sid = runningSessionId.current;
        if (sid) {
          setTurnCounts((prev) => ({ ...prev, [sid]: (prev[sid] ?? 0) + 1 }));
        }
        refreshSessions();
      }
    }
  }, [showToast, refreshSessions, finishTypewriter, startTypewriter, updateRunning]);

  // Connect SSE on mount + load sessions + load models + workdir + permission
  useEffect(() => {
    const disconnect = connectSSE("/api/events", handleSSEEvent, setConnected);
    refreshSessions();
    refreshModels();
    refreshWorkDir();
    refreshPermission();
    refreshContextUsage();
    refreshSkills();
    return disconnect;
  }, [handleSSEEvent, refreshSessions, refreshModels, refreshWorkDir, refreshPermission, refreshContextUsage, refreshSkills]);

  // Send a chat message
  const handleSend = useCallback(
    async (message: string) => {
      // 标记运行会话：本轮所有 SSE 事件与打字机都写入该会话缓冲
      runningSessionId.current = currentSessionId;
      busyRef.current = true;
      updateRunning((prev) => [...prev, { _id: genId(), role: "user", content: message }]);
      setBusy(true);
      setAgentPhase("thinking");
      try {
        await sendChat(message);
      } catch (err) {
        busyRef.current = false;
        setBusy(false);
        setAgentPhase("idle");
        showToast(`发送失败：${err}`, "error");
      }
    },
    [showToast, updateRunning, currentSessionId]
  );

  // handleSlashCommand 处理输入框提交上来的斜杠行。
  //
  // 首个 token 决定意图：
  //   · /clear、/compact、/exit 是 Web 模式真正接了后端分支的内置命令，就地执行；
  //   · 命中已加载技能名时，翻译成一条"先 load_skill 再干活"的普通对话消息发出去——
  //     技能正文不在上下文里，必须由模型主动调工具载入；
  //   · 其余一律提示未知命令，不静默丢弃用户输入。
  const handleSlashCommand = useCallback(
    async (cmd: string) => {
      const { head, name, rest } = parseSlashInput(cmd);
      try {
        if (head === "/clear") {
          await sendClear();
          const sid = currentSessionId;
          if (sid) {
            setBuffers((prev) => ({ ...prev, [sid]: [] }));
            setTurnCounts((prev) => ({ ...prev, [sid]: 0 }));
          }
          showToast("已清空历史", "ok");
          return;
        }
        if (head === "/compact") {
          await sendCompact();
          return;
        }
        if (head === "/exit") {
          await sendExit();
          showToast("会话正在结束…");
          return;
        }
        const skill = findSkill(skills, name);
        if (skill) {
          await handleSend(skillPrompt(skill, rest));
          return;
        }
        showToast(`未知命令：${head}`, "error");
      } catch (err) {
        showToast(`命令执行失败：${err}`, "error");
      }
    },
    [showToast, currentSessionId, skills, handleSend]
  );

  // Handle permission dialog response
  const handlePermissionResolve = useCallback(
    async (allow: boolean, reason: string) => {
      if (!permission) return;
      try {
        await sendPermission(permission.id, allow, reason);
      } catch (err) {
        showToast(`权限响应失败：${err}`, "error");
      }
      setPermission(null);
    },
    [permission, showToast]
  );

  // ─── Session actions ───

  const handleNewSession = useCallback(async () => {
    try {
      const meta = await createSession();
      setCurrentSessionId(meta.id);
      setBuffers((prev) => ({ ...prev, [meta.id]: [] }));
      setTurnCounts((prev) => ({ ...prev, [meta.id]: 0 }));
      await refreshSessions();
    } catch (err) {
      showToast(`创建会话失败：${err}`, "error");
    }
  }, [refreshSessions, showToast]);

  const handleSelectSession = useCallback(async (id: string) => {
    if (id === currentSessionId) return;
    setCurrentSessionId(id);

    // 已有缓存（含正在运行的会话）：保留实时视图，切回即恢复思考/流式与工具卡片。
    if (buffersRef.current[id]) {
      // 仅在空闲时回同步后端活跃会话（GET 会 swap 后端 State）；
      // busy 时跳过 swap，避免打断正在运行的轮次上下文。
      if (!busyRef.current) {
        try { await loadSession(id); } catch { /* 忽略：仅用于同步后端活跃会话 */ }
      }
      return;
    }

    // 无缓存：从后端加载（swap 活跃会话）并无损重建历史
    try {
      const record = await loadSession(id);
      setBuffers((prev) => ({ ...prev, [id]: reconstructMessages(record.messages, genId) }));
      setTurnCounts((prev) => ({ ...prev, [id]: record.turn_count || 0 }));
    } catch (err) {
      showToast(`加载会话失败：${err}`, "error");
    }
  }, [currentSessionId, showToast]);

  const handleDeleteSession = useCallback(async (id: string) => {
    if (!confirm("确定要删除这个会话吗？")) return;
    try {
      await deleteSession(id);
      // 清除该会话的前端缓存，避免残留
      setBuffers((prev) => { const n = { ...prev }; delete n[id]; return n; });
      setTurnCounts((prev) => { const n = { ...prev }; delete n[id]; return n; });
      if (id === currentSessionId) {
        // Refresh and select latest
        const list = await fetchSessions();
        setSessions(list || []);
        if (list.length > 0) {
          handleSelectSession(list[0].id);
        } else {
          handleNewSession();
        }
      } else {
        await refreshSessions();
      }
    } catch (err) {
      showToast(`删除失败：${err}`, "error");
    }
  }, [currentSessionId, refreshSessions, handleSelectSession, handleNewSession, showToast]);

  const handleRenameSession = useCallback(async (id: string, title: string) => {
    try {
      await renameSession(id, title);
      setSessions((prev) =>
        prev.map((s) => (s.id === id ? { ...s, title } : s))
      );
    } catch (err) {
      showToast(`重命名失败：${err}`, "error");
    }
  }, [showToast]);

  // Auto-select first session on initial load
  useEffect(() => {
    if (!currentSessionId && sessions.length > 0) {
      const id = sessions[0].id;
      setCurrentSessionId(id);
      // 首屏加载该会话历史并无损重建，避免首个会话显示空白
      loadSession(id)
        .then((record) => {
          setBuffers((prev) => (prev[id] ? prev : { ...prev, [id]: reconstructMessages(record.messages, genId) }));
          setTurnCounts((prev) => ({ ...prev, [id]: record.turn_count || 0 }));
        })
        .catch(() => { /* 忽略：首屏加载失败时保持空缓冲 */ });
    }
  }, [sessions, currentSessionId]);

  // ─── Model actions ───

  const handleModelSelect = useCallback(async (name: string) => {
    try {
      await selectModel(name);
      setActiveModel(name);
      setModel(name);
      showToast(`正在切换到模型"${name}"…`);
    } catch (err) {
      showToast(`切换模型失败：${err}`, "error");
    }
  }, [showToast]);

  const handleModelAdd = useCallback(async (preset: ModelPreset) => {
    try {
      await addModel(preset);
      await refreshModels();
      showToast(`已添加模型"${preset.name}"`, "ok");
    } catch (err) {
      showToast(`添加模型失败：${err}`, "error");
    }
  }, [refreshModels, showToast]);

  const handleModelEdit = useCallback(async (name: string, preset: ModelPreset) => {
    try {
      await updateModel(name, preset);
      await refreshModels();
      showToast(`已更新模型"${name}"`, "ok");
    } catch (err) {
      showToast(`编辑模型失败：${err}`, "error");
    }
  }, [refreshModels, showToast]);

  // handleWorkDirSave 提交工作目录切换请求。
  // 后端串行处理，成功/失败通过 SSE 的 workdir_changed / error 事件反馈。
  const handleWorkDirSave = useCallback(async (path: string) => {
    try {
      await setWorkDir(path);
      showToast("已提交工作目录切换请求…");
    } catch (err) {
      showToast(`切换工作目录失败：${err}`, "error");
    }
  }, [showToast]);

  // handlePermissionSave 运行时更新权限模式与规则，后端同步生效并返回最新快照。
  const handlePermissionSave = useCallback(async (cfg: {
    mode: PermissionMode;
    deny_rules: PermissionConfig["deny_rules"];
    allow_rules: PermissionConfig["allow_rules"];
  }) => {
    try {
      const updated = await updatePermissionConfig(cfg);
      setPermissionConfig(updated);
      showToast("权限配置已更新", "ok");
    } catch (err) {
      showToast(`更新权限配置失败：${err}`, "error");
    }
  }, [showToast]);

  // Handle stop button
  const handleStop = useCallback(async () => {
    try {
      await sendStop();
      finishTypewriter();
      busyRef.current = false;
      setBusy(false);
      setAgentPhase("idle");
      setCurrentToolName("");
      showToast("已停止生成");
    } catch (err) {
      showToast(`停止失败：${err}`, "error");
    }
  }, [finishTypewriter, showToast]);

  // handleCompact 主动触发一次完整上下文压缩；
  // 压缩结果与最新占用快照经 SSE 的 compact / context_usage 事件回推
  const handleCompact = useCallback(async () => {
    try {
      await sendCompact();
    } catch (err) {
      showToast(`压缩失败：${err}`, "error");
    }
  }, [showToast]);

  return (
    <div class="app-layout">
      <Sidebar
        sessions={sessions}
        currentId={currentSessionId}
        connected={connected}
        workDir={workDir}
        onSelect={handleSelectSession}
        onNew={handleNewSession}
        onDelete={handleDeleteSession}
        onRename={handleRenameSession}
      />
      <div class="app-main">
        <StatusBar
          status={{ messages, model, connected, busy, turnCount, pendingPermission: permission }}
          workDir={workDir}
        />
        <MessageList
          messages={messages}
          agentPhase={displayPhase}
          toolName={displayTool}
          onSuggestion={handleSend}
        />
        {currentToast && (
          <div class={`toast ${currentToast.tone === "ok" ? "toast--ok" : currentToast.tone === "error" ? "toast--error" : ""}`}>
            {currentToast.tone === "ok"
              ? <IconCheck size={14} />
              : currentToast.tone === "error"
                ? <IconAlert size={14} />
                : <IconSpark size={14} />}
            <span>{currentToast.text}</span>
          </div>
        )}
        {permission && (
          <PermissionDialog permission={permission} onResolve={handlePermissionResolve} />
        )}
        <Composer
          disabled={busy}
          busy={busy}
          workDir={workDir}
          permissionMode={permissionConfig.mode}
          models={models}
          activeModel={activeModel}
          contextUsage={contextUsage}
          skills={skills}
          onSend={handleSend}
          onSlashCommand={handleSlashCommand}
          onStop={handleStop}
          onOpenWorkDir={() => setShowWorkDir(true)}
          onOpenPermission={() => setShowPermission(true)}
          onSelectModel={handleModelSelect}
          onAddModel={handleModelAdd}
          onEditModel={handleModelEdit}
          onCompact={handleCompact}
        />
        {showWorkDir && (
          <WorkDirPanel
            current={workDir}
            onSave={handleWorkDirSave}
            onClose={() => setShowWorkDir(false)}
          />
        )}
        {showPermission && (
          <PermissionPanel
            config={permissionConfig}
            onSave={handlePermissionSave}
            onClose={() => setShowPermission(false)}
          />
        )}
      </div>
    </div>
  );
}
