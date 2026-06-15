import { useState, useEffect, useCallback } from "preact/hooks";
import type { ChatMessage, PermissionAsk, SSEEvent } from "../types";
import { connectSSE } from "../lib/sse";
import { sendChat, sendClear, sendCompact, sendExit, sendPermission } from "../lib/api";
import { StatusBar } from "./StatusBar";
import { MessageList } from "./MessageList";
import { InputBar } from "./InputBar";
import { PermissionDialog } from "./PermissionDialog";

export function App() {
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [connected, setConnected] = useState(false);
  const [busy, setBusy] = useState(false);
  const [model, setModel] = useState("");
  const [turnCount, setTurnCount] = useState(0);
  const [permission, setPermission] = useState<PermissionAsk | null>(null);
  const [toast, setToast] = useState<string | null>(null);

  // Show a toast message briefly
  const showToast = useCallback((msg: string) => {
    setToast(msg);
    setTimeout(() => setToast(null), 3000);
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
          // Find the last tool_call without result and update it
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
      }
    }
  }, [showToast]);

  // Connect SSE on mount
  useEffect(() => {
    const disconnect = connectSSE("/api/events", handleSSEEvent, setConnected);
    return disconnect;
  }, [handleSSEEvent]);

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

  return (
    <div class="app">
      <StatusBar
        status={{ messages, model, connected, busy, turnCount, pendingPermission: permission }}
      />
      <MessageList messages={messages} />
      {toast && <div class="toast">{toast}</div>}
      {permission && (
        <PermissionDialog permission={permission} onResolve={handlePermissionResolve} />
      )}
      <InputBar disabled={busy} onSend={handleSend} onSlashCommand={handleSlashCommand} />
    </div>
  );
}
