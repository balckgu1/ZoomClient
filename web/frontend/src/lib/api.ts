const BASE = "";

async function post(path: string, body?: Record<string, unknown>): Promise<Response> {
  const res = await fetch(`${BASE}${path}`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: body ? JSON.stringify(body) : "{}",
  });
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: res.statusText }));
    throw new Error(err.error || res.statusText);
  }
  return res;
}

async function del(path: string): Promise<Response> {
  const res = await fetch(`${BASE}${path}`, { method: "DELETE" });
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: res.statusText }));
    throw new Error(err.error || res.statusText);
  }
  return res;
}

async function patch(path: string, body: Record<string, unknown>): Promise<Response> {
  const res = await fetch(`${BASE}${path}`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: res.statusText }));
    throw new Error(err.error || res.statusText);
  }
  return res;
}

export async function sendChat(message: string): Promise<void> {
  await post("/api/chat", { message });
}

export async function sendClear(): Promise<void> {
  await post("/api/clear");
}

export async function sendCompact(): Promise<void> {
  await post("/api/compact");
}

export async function sendExit(): Promise<void> {
  await post("/api/exit");
}

export async function sendStop(): Promise<void> {
  await post("/api/stop");
}

export async function sendPermission(
  id: string,
  allow: boolean,
  reason: string = ""
): Promise<void> {
  await post("/api/permission", { id, allow, reason });
}

export interface StatusResponse {
  model: string;
  turn_count: number;
  busy: boolean;
  session_id: string;
  workdir: string;
  permission_mode: string;
}

export async function fetchStatus(): Promise<StatusResponse> {
  const res = await fetch(`${BASE}/api/status`);
  return res.json();
}

// ─── 上下文占用 API ───

import type { ContextUsage } from "../types";

// fetchContextUsage 查询最近一次上下文占用快照（GET /api/context-usage）
export async function fetchContextUsage(): Promise<ContextUsage> {
  const res = await fetch(`${BASE}/api/context-usage`);
  if (!res.ok) throw new Error(res.statusText);
  return res.json();
}

// ─── Session API ───

import type { SessionMeta, SessionRecord } from "../types";

export async function fetchSessions(): Promise<SessionMeta[]> {
  const res = await fetch(`${BASE}/api/sessions`);
  if (!res.ok) throw new Error(res.statusText);
  return res.json();
}

export async function createSession(): Promise<SessionMeta> {
  const res = await post("/api/sessions");
  return res.json();
}

export async function loadSession(id: string): Promise<SessionRecord> {
  const res = await fetch(`${BASE}/api/sessions/${id}`);
  if (!res.ok) throw new Error(res.statusText);
  return res.json();
}

export async function deleteSession(id: string): Promise<void> {
  await del(`/api/sessions/${id}`);
}

export async function renameSession(id: string, title: string): Promise<void> {
  await patch(`/api/sessions/${id}`, { title });
}

// ─── Model API ───

import type { ModelPreset, ModelsResponse } from "../types";

export async function fetchModels(): Promise<ModelsResponse> {
  const res = await fetch(`${BASE}/api/models`);
  if (!res.ok) throw new Error(res.statusText);
  return res.json();
}

export async function addModel(preset: ModelPreset): Promise<void> {
  await post("/api/models", preset as unknown as Record<string, unknown>);
}

export async function selectModel(name: string): Promise<void> {
  await post("/api/model/select", { name });
}

export async function deleteModel(name: string): Promise<void> {
  await del(`/api/models/${name}`);
}

export async function testModel(preset: ModelPreset): Promise<{ status: string; message: string }> {
  const res = await post("/api/models/test", preset as unknown as Record<string, unknown>);
  return res.json();
}

async function put(path: string, body: Record<string, unknown>): Promise<Response> {
  const res = await fetch(`${BASE}${path}`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: res.statusText }));
    throw new Error(err.error || res.statusText);
  }
  return res;
}

export async function updateModel(name: string, preset: ModelPreset): Promise<void> {
  await put(`/api/models/${name}`, preset as unknown as Record<string, unknown>);
}

// ─── 工作目录 API ───

import type { PermissionConfig, PermissionMode, PermissionRule } from "../types";

// fetchWorkDir 查询当前工作目录（GET /api/workdir）
export async function fetchWorkDir(): Promise<string> {
  const res = await fetch(`${BASE}/api/workdir`);
  if (!res.ok) throw new Error(res.statusText);
  const data = await res.json();
  return (data.path as string) || "";
}

// setWorkDir 请求切换工作目录（POST /api/workdir）。
// 注意：后端将切换动作投递给 REPL 循环串行执行，本请求仅返回"已接受"，
// 真正的成功/失败结果通过 SSE 的 info/error 事件反馈，前端应据此刷新显示。
export async function setWorkDir(path: string): Promise<void> {
  await post("/api/workdir", { path });
}

// ─── 权限配置 API ───

// fetchPermissionConfig 查询当前权限配置（GET /api/permission/config）
export async function fetchPermissionConfig(): Promise<PermissionConfig> {
  const res = await fetch(`${BASE}/api/permission/config`);
  if (!res.ok) throw new Error(res.statusText);
  return res.json();
}

// updatePermissionConfig 运行时更新权限模式与规则（PUT /api/permission/config），
// 后端同步生效并返回最新快照。
export async function updatePermissionConfig(cfg: {
  mode: PermissionMode;
  deny_rules: PermissionRule[];
  allow_rules: PermissionRule[];
}): Promise<PermissionConfig> {
  const res = await put("/api/permission/config", cfg as unknown as Record<string, unknown>);
  return res.json();
}

// ─── 技能目录 API ───

import type { SkillMeta } from "../types";

// fetchSkills 拉取全部已加载 skill 的元信息（GET /api/skills），供输入框 "/" 扩展框使用。
//
// 后端约定 skills 恒为数组，这里仍兜底一次 null：技能目录属于可选能力，
// 拉取失败或旧版二进制缺字段时退化为空列表，不应让整个输入框报错。
export async function fetchSkills(): Promise<SkillMeta[]> {
  const res = await fetch(`${BASE}/api/skills`);
  if (!res.ok) throw new Error(res.statusText);
  const data = await res.json();
  return (data.skills as SkillMeta[]) || [];
}
