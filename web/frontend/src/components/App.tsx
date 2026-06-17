import { useState, useEffect, useCallback } from "preact/hooks";
import type { ChatMessage, PermissionAsk, SSEEvent, SessionMeta, ModelPreset } from "../types";
import { connectSSE } from "../lib/sse";
import {
  sendChat, sendClear, sendCompact, sendExit, sendPermission,
  fetchSessions, createSession, loadSession, deleteSession, renameSession,
  fetchModels, addModel, selectModel,
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
  const [toast, setToast] = useState<string | null>(null);

  // Session state
  const [sessions, setSessions] = useState<SessionMeta[]>([]);
  const [currentSessionId, setCurrentSessionId] = useState("");

  // Model state
  const [models, setModels] = useState<ModelPreset[]>([]);
  const [activeModel, setActiveModel] = useState("");

  // Show a toast message briefly
  const showToast = useCallback((msg: string) => {
    setToast(msg);
    setTimeout(() => setToast(null), 3000);
  }, []);

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
        // Update session title in sidebar
        const id = d.id;
        const title = d.title;
        setSessions((prev) =>
          prev.map((s) => (s.id === id ? { ...s, title } : s))
        );
      }
      return;
    }

    if (evt.ch === "agent") {
      const type = d.type;
      if (type === "assistant") {
        setMessages((prev) => [...prev, { role: "assistant", content: d.content }]);
      } else if (type === "reasoning") {
        setMessages((prev) => [...prev, { role: "reasoning", content: d.content }]);
      } else if (type === "tool_call") {
        setMessages((prev) => [
          ...prev,
          { role: "tool_call", name: d.name, args: d.args },
        ]);
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
      } else if (type === "sub_agent") {
        setMessages((prev) => [...prev, { role: "sub_agent", prompt: d.prompt }]);
      } else if (type === "hook_blocked") {
        setMessages((prev) => [
          ...prev,
          { role: "hook_blocked", tool: d.tool, reason: d.reason },
        ]);
      } else if (type === "todo_panel") {
        setMessages((prev) => [
          ...prev,
          { role: "system", content: `📋 Plan\n${d.content}` },
        ]);
      } else if (type === "done") {
        setBusy(false);
        setTurnCount((c) => c + 1);
        // Refresh session list after turn completes
        refreshSessions();
      }
    }
  }, [showToast, refreshSessions]);

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
      setMessages((prev) => [...prev, { role: "user", content: message }]);
      setBusy(true);
      try {
        await sendChat(message);
      } catch (err) {
        setBusy(false);
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
            converted.push({ role: "user", content: String(m.content || "") });
          } else if (m.role === "assistant") {
            if (m.reasoning_content) {
              converted.push({ role: "reasoning", content: String(m.reasoning_content) });
            }
            converted.push({ role: "assistant", content: String(m.content || "") });
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
            disabled={busy}
          />
        </div>
        <MessageList messages={messages} />
        {toast && <div class="toast">{toast}</div>}
        {permission && (
          <PermissionDialog permission={permission} onResolve={handlePermissionResolve} />
        )}
        <InputBar disabled={busy} onSend={handleSend} onSlashCommand={handleSlashCommand} />
      </div>
    </div>
  );
}
