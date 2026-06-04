// desktop/src/lib/SidecarBridge.ts
// Tauri ↔ Go Sidecar 通信桥。监听 sidecar-event，按 ch 字段分流到不同 store。
import { listen } from "@tauri-apps/api/event";
import { invoke } from "@tauri-apps/api/core";
import { writable, type Writable } from "svelte/store";

// ── 类型 ──
export interface AgentMessage {
  type: string;
  content?: string;
  name?: string;
  args?: string;
  is_error?: boolean;
}

export interface ChatMessage {
  role: "user" | "assistant" | "tool" | "system";
  content: string;
  isError?: boolean;
}

export interface PermissionRequest {
  id: string;
  tool: string;
  reason: string;
  args: string;
}

// ── Stores ──
export const agentMessages: Writable<AgentMessage[]> = writable([]);
export const chatHistory: Writable<ChatMessage[]> = writable([]);
export const emotionState: Writable<string> = writable("idle");
export const isProcessing: Writable<boolean> = writable(false);
export const sidecarAlive: Writable<boolean> = writable(true);
export const permissionRequests: Writable<PermissionRequest[]> = writable([]);

const pendingAcks = new Map<string, ReturnType<typeof setTimeout>>();
let msgCounter = 0;

// ── 心跳检测 ──
const HEARTBEAT_TIMEOUT = 10_000;
let lastHeartbeat = Date.now();
setInterval(() => {
  if (Date.now() - lastHeartbeat > HEARTBEAT_TIMEOUT) sidecarAlive.set(false);
}, 5_000);

// ── 主事件监听 ──
listen<string>("sidecar-event", (evt) => {
  let msg: { ch: string; data: Record<string, unknown> };
  try { msg = JSON.parse(evt.payload); } catch { return; }

  switch (msg.ch) {
    case "agent":  onAgent(msg.data as unknown as AgentMessage); break;
    case "emotion": emotionState.set((msg.data as Record<string, string>).state ?? "idle"); break;
    case "system":  onSystem(msg.data as Record<string, string>); break;
    case "permission": onPermission(msg.data as unknown as PermissionRequest); break;
  }
});

function onAgent(d: AgentMessage) {
  agentMessages.update((m) => [...m, d]);
  chatHistory.update((h) => {
    const last = h[h.length - 1];
    switch (d.type) {
      case "token":
        if (last?.role === "assistant") { last.content += d.content ?? ""; return [...h]; }
        return [...h, { role: "assistant", content: d.content ?? "" }];
      case "reasoning":
        return [...h, { role: "system", content: `💭 ${d.content ?? ""}` }];
      case "tool_call":
        return [...h, { role: "tool", content: `▶ ${d.name}(${d.args ?? ""})` }];
      case "tool_result":
        if (last?.role === "tool") {
          last.content += d.is_error ? "\n✗ error" : "\n✓ ok";
          if (d.content) last.content += `\n${d.content.slice(0, 300)}`;
          last.isError = d.is_error;
          return [...h];
        }
        return [...h, { role: "tool", content: d.content ?? "", isError: d.is_error }];
      case "assistant":
        if (last?.role === "assistant") { last.content += d.content ?? ""; return [...h]; }
        return [...h, { role: "assistant", content: d.content ?? "" }];
      case "done":
        isProcessing.set(false);
        return h;
      default: return h;
    }
  });
}

function onSystem(d: Record<string, string>) {
  if (d.event === "heartbeat") { lastHeartbeat = Date.now(); sidecarAlive.set(true); return; }
  if (d.event === "ack" && d.id) {
    const t = pendingAcks.get(d.id);
    if (t) { clearTimeout(t); pendingAcks.delete(d.id); }
    if (d.status === "rejected") {
      chatHistory.update((h) => [...h, { role: "system", content: `⚠ 被拒绝: ${d.reason ?? ""}` }]);
    }
    return;
  }
  if (d.event === "error") {
    chatHistory.update((h) => [...h, { role: "system", content: `✗ [${d.scope ?? ""}] ${d.message ?? ""}` }]);
    isProcessing.set(false);
    return;
  }
  if (d.event === "info") {
    chatHistory.update((h) => [...h, { role: "system", content: `· ${d.message ?? ""}` }]);
  }
}

function onPermission(d: PermissionRequest) {
  permissionRequests.update((reqs) => [...reqs, d]);
  chatHistory.update((h) => [...h, {
    role: "system",
    content: `🔐 权限请求: ${d.tool} — ${d.reason}`
  }]);
}

// ── 发送 ──
export async function sendChat(message: string): Promise<void> {
  const id = `msg_${++msgCounter}`;
  const json = JSON.stringify({ ch: "cmd", id, action: "chat", payload: { message } });
  chatHistory.update((h) => [...h, { role: "user", content: message }]);
  isProcessing.set(true);
  const timer = setTimeout(() => {
    pendingAcks.delete(id);
    chatHistory.update((h) => [...h, { role: "system", content: "⚠ 发送超时，请重试" }]);
    isProcessing.set(false);
  }, 5_000);
  pendingAcks.set(id, timer);
  try { await invoke("send_to_sidecar", { message: json }); }
  catch (err) { clearTimeout(timer); pendingAcks.delete(id); isProcessing.set(false);
    chatHistory.update((h) => [...h, { role: "system", content: `✗ 发送失败: ${err}` }]);
  }
}

export async function sendCmd(action: string, payload?: Record<string, unknown>): Promise<void> {
  const id = `cmd_${++msgCounter}`;
  const json = JSON.stringify({ ch: "cmd", id, action, payload: payload ?? {} });
  try { await invoke("send_to_sidecar", { message: json }); } catch { /* silent */ }
}

export async function sendPermissionReply(id: string, allow: boolean, reason?: string): Promise<void> {
  const cmdId = `perm_reply_${++msgCounter}`;
  const json = JSON.stringify({
    ch: "cmd",
    id: cmdId,
    action: "permission_reply",
    payload: { id, allow, reason: reason ?? "" }
  });
  permissionRequests.update((reqs) => reqs.filter((r) => r.id !== id));
  try { await invoke("send_to_sidecar", { message: json }); } catch { /* silent */ }
}
