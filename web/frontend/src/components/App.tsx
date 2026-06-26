import { useState, useEffect, useCallback, useRef } from "preact/hooks";
import type { ChatMessage, PermissionAsk, SSEEvent, SessionMeta, ModelPreset } from "../types";
import type { AgentPhase } from "./AgentStatus";
import { connectSSE } from "../lib/sse";
import {
  sendChat, sendClear, sendCompact, sendExit, sendStop, sendPermission,
  fetchSessions, createSession, loadSession, deleteSession, renameSession,
  fetchModels, addModel, selectModel, updateModel,
} from "../lib/api";
import { StatusBar } from "./StatusBar";
import { MessageList } from "./MessageList";
import { InputBar } from "./InputBar";
import { PermissionDialog } from "./PermissionDialog";
import { Sidebar } from "./Sidebar";
import { ModelSelector } from "./ModelSelector";

export function App() {
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [connected, setConnected] = useState(false);
  const [busy, setBusy] = useState(false);
  const [model, setModel] = useState("");
  const [turnCount, setTurnCount] = useState(0);
  const [permission, setPermission] = useState<PermissionAsk | null>(null);
  // Toast queue: multiple toasts are queued and shown sequentially
  const [toastQueue, setToastQueue] = useState<string[]>([]);
  const [currentToast, setCurrentToast] = useState<string | null>(null);

  // Session state
  const [sessions, setSessions] = useState<SessionMeta[]>([]);
  const [currentSessionId, setCurrentSessionId] = useState("");

  // Model state
  const [models, setModels] = useState<ModelPreset[]>([]);
  const [activeModel, setActiveModel] = useState("");

  // Agent phase state (for status indicator)
  const [agentPhase, setAgentPhase] = useState<AgentPhase>("idle");
  const [currentToolName, setCurrentToolName] = useState<string>("");

  // Typewriter effect refs
  const typewriterTimer = useRef<ReturnType<typeof setInterval> | null>(null);
  const pendingText = useRef<string>("");
  const currentIndex = useRef<number>(0);
  const streamingMsgIdx = useRef<number>(-1);

  // Message ID counter for stable React keys
  const nextMsgId = useRef(0);
  const genId = useCallback(() => { nextMsgId.current += 1; return nextMsgId.current; }, []);

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
      setMessages((prev) => {
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
  }, []);

  // Show a toast message briefly. Multiple toasts are queued and shown sequentially.
  const showToast = useCallback((msg: string) => {
    setToastQueue((prev) => [...prev, msg]);
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
        setMessages((prev) => {
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
      setMessages((prev) => {
        const updated = [...prev];
        if (updated[msgIdx] && updated[msgIdx].role === "assistant") {
          updated[msgIdx] = { ...updated[msgIdx], content: text.slice(0, ci), streaming: true } as ChatMessage;
        }
        return updated;
      });
    }, speed);
  }, []);

  // Handle incoming SSE events
  const handleSSEEvent = useCallback((evt: SSEEvent) => {
    const d = evt.data as Record<string, string>;

    if (evt.ch === "system") {
      const event = d.event;
      if (event === "ready") {
        setModel(d.model || "");
        showToast("Session started");
      } else if (event === "info") {
        showToast(d.message || "");
      } else if (event === "error") {
        showToast(`Error [${d.scope}]: ${d.message}`);
      } else if (event === "compact") {
        const before = d.before_bytes || "?";
        const after = d.after_bytes || "?";
        showToast(`Compacted: ${before} → ${after} bytes`);
      } else if (event === "permission_ask") {
        setPermission({
          id: d.id,
          tool: d.tool,
          args: d.args,
          reason: d.reason,
        });
      } else if (event === "session_end") {
        showToast("Session ended");
      } else if (event === "session_renamed") {
        const id = d.id;
        const title = d.title;
        setSessions((prev) =>
          prev.map((s) => (s.id === id ? { ...s, title } : s))
        );
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
        setMessages((prev) => {
          const updated = [...prev, { _id: genId(), role: "assistant", content: "", streaming: true } as ChatMessage];
          const newIdx = updated.length - 1;
          // Schedule typewriter after state update
          setTimeout(() => startTypewriter(fullText, newIdx), 0);
          return updated;
        });
      } else if (type === "reasoning") {
        setMessages((prev) => [...prev, { _id: genId(), role: "reasoning", content: d.content }]);
        setAgentPhase("thinking");
      } else if (type === "tool_call") {
        finishTypewriter();
        setMessages((prev) => [
          ...prev,
          { _id: genId(), role: "tool_call", name: d.name, args: d.args },
        ]);
        setAgentPhase("handling");
        setCurrentToolName(d.name || "");
      } else if (type === "tool_result") {
        setMessages((prev) => {
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
        setMessages((prev) => [...prev, { _id: genId(), role: "sub_agent", prompt: d.prompt }]);
      } else if (type === "hook_blocked") {
        setMessages((prev) => [
          ...prev,
          { _id: genId(), role: "hook_blocked", tool: d.tool, reason: d.reason },
        ]);
      } else if (type === "todo_panel") {
        setMessages((prev) => [
          ...prev,
          { _id: genId(), role: "system", content: `📋 Plan\n${d.content}` },
        ]);
      } else if (type === "done") {
        finishTypewriter();
        setBusy(false);
        setAgentPhase("idle");
        setCurrentToolName("");
        setTurnCount((c) => c + 1);
        refreshSessions();
      }
    }
  }, [showToast, refreshSessions, finishTypewriter, startTypewriter]);

  // Connect SSE on mount + load sessions + load models
  useEffect(() => {
    const disconnect = connectSSE("/api/events", handleSSEEvent, setConnected);
    refreshSessions();
    refreshModels();
    return disconnect;
  }, [handleSSEEvent, refreshSessions, refreshModels]);

  // Send a chat message
  const handleSend = useCallback(
    async (message: string) => {
      setMessages((prev) => [...prev, { _id: genId(), role: "user", content: message }]);
      setBusy(true);
      setAgentPhase("thinking");
      try {
        await sendChat(message);
      } catch (err) {
        setBusy(false);
        setAgentPhase("idle");
        showToast(`Send failed: ${err}`);
      }
    },
    [showToast]
  );

  // Handle slash commands
  const handleSlashCommand = useCallback(
    async (cmd: string) => {
      const lower = cmd.toLowerCase().trim();
      try {
        if (lower === "/clear") {
          await sendClear();
          setMessages([]);
          setTurnCount(0);
          showToast("History cleared");
        } else if (lower === "/compact") {
          await sendCompact();
        } else if (lower === "/exit") {
          await sendExit();
          showToast("Session ending...");
        } else {
          showToast(`Unknown command: ${cmd}`);
        }
      } catch (err) {
        showToast(`Command failed: ${err}`);
      }
    },
    [showToast]
  );

  // Handle permission dialog response
  const handlePermissionResolve = useCallback(
    async (allow: boolean, reason: string) => {
      if (!permission) return;
      try {
        await sendPermission(permission.id, allow, reason);
      } catch (err) {
        showToast(`Permission reply failed: ${err}`);
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
      setMessages([]);
      setTurnCount(0);
      await refreshSessions();
    } catch (err) {
      showToast(`Create session failed: ${err}`);
    }
  }, [refreshSessions, showToast]);

  const handleSelectSession = useCallback(async (id: string) => {
    if (id === currentSessionId) return;
    try {
      const record = await loadSession(id);
      setCurrentSessionId(id);
      // Convert backend messages (fsm.Message format) to ChatMessage format
      const converted: ChatMessage[] = [];
      if (record.messages) {
        for (const msg of record.messages) {
          const m = msg as Record<string, unknown>;
          if (m.role === "user") {
            converted.push({ _id: genId(), role: "user", content: String(m.content || "") });
          } else if (m.role === "assistant") {
            if (m.reasoning_content) {
              converted.push({ _id: genId(), role: "reasoning", content: String(m.reasoning_content) });
            }
            converted.push({ _id: genId(), role: "assistant", content: String(m.content || "") });
          } else if (m.role === "tool" && m.tool_call_id) {
            // Skip tool messages for cleaner display
          }
        }
      }
      setMessages(converted);
      setTurnCount(record.turn_count || 0);
    } catch (err) {
      showToast(`Load session failed: ${err}`);
    }
  }, [currentSessionId, showToast]);

  const handleDeleteSession = useCallback(async (id: string) => {
    if (!confirm("Are you sure to delete this session?")) return;
    try {
      await deleteSession(id);
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
      showToast(`Delete failed: ${err}`);
    }
  }, [currentSessionId, refreshSessions, handleSelectSession, handleNewSession, showToast]);

  const handleRenameSession = useCallback(async (id: string, title: string) => {
    try {
      await renameSession(id, title);
      setSessions((prev) =>
        prev.map((s) => (s.id === id ? { ...s, title } : s))
      );
    } catch (err) {
      showToast(`Rename failed: ${err}`);
    }
  }, [showToast]);

  // Auto-select first session on initial load
  useEffect(() => {
    if (!currentSessionId && sessions.length > 0) {
      setCurrentSessionId(sessions[0].id);
    }
  }, [sessions, currentSessionId]);

  // ─── Model actions ───

  const handleModelSelect = useCallback(async (name: string) => {
    try {
      await selectModel(name);
      setActiveModel(name);
      setModel(name);
      showToast(`Switching to model "${name}"...`);
    } catch (err) {
      showToast(`Model switch failed: ${err}`);
    }
  }, [showToast]);

  const handleModelAdd = useCallback(async (preset: ModelPreset) => {
    try {
      await addModel(preset);
      await refreshModels();
      showToast(`Model "${preset.name}" added`);
    } catch (err) {
      showToast(`Add model failed: ${err}`);
    }
  }, [refreshModels, showToast]);

  const handleModelEdit = useCallback(async (name: string, preset: ModelPreset) => {
    try {
      await updateModel(name, preset);
      await refreshModels();
      showToast(`Model "${name}" updated`);
    } catch (err) {
      showToast(`Edit model failed: ${err}`);
    }
  }, [refreshModels, showToast]);

  // Handle stop button
  const handleStop = useCallback(async () => {
    try {
      await sendStop();
      finishTypewriter();
      setBusy(false);
      setAgentPhase("idle");
      setCurrentToolName("");
      showToast("Generation stopped");
    } catch (err) {
      showToast(`Stop failed: ${err}`);
    }
  }, [finishTypewriter, showToast]);

  return (
    <div class="app-layout">
      <Sidebar
        sessions={sessions}
        currentId={currentSessionId}
        onSelect={handleSelectSession}
        onNew={handleNewSession}
        onDelete={handleDeleteSession}
        onRename={handleRenameSession}
      />
      <div class="app-main">
        <StatusBar
          status={{ messages, model, connected, busy, turnCount, pendingPermission: permission }}
        />
        <div class="model-bar">
          <ModelSelector
            models={models}
            active={activeModel}
            onSelect={handleModelSelect}
            onAdd={handleModelAdd}
            onEdit={handleModelEdit}
            disabled={busy}
          />
        </div>
        <MessageList messages={messages} agentPhase={agentPhase} toolName={currentToolName} />
        {currentToast && <div class="toast">{currentToast}</div>}
        {permission && (
          <PermissionDialog permission={permission} onResolve={handlePermissionResolve} />
        )}
        <InputBar disabled={busy} busy={busy} onSend={handleSend} onSlashCommand={handleSlashCommand} onStop={handleStop} />
      </div>
    </div>
  );
}
