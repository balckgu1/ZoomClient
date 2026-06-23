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
}

export async function fetchStatus(): Promise<StatusResponse> {
  const res = await fetch(`${BASE}/api/status`);
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
