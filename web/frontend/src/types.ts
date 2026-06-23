// SSE event from backend
export interface SSEEvent {
  ch: "agent" | "system" | "emotion";
  data: Record<string, unknown>;
}

// Chat message types displayed in the UI
export type ChatMessage =
  | { role: "user"; content: string }
  | { role: "assistant"; content: string; streaming?: boolean }
  | { role: "reasoning"; content: string }
  | { role: "tool_call"; name: string; args: string; result?: string; isError?: boolean }
  | { role: "sub_agent"; prompt: string }
  | { role: "hook_blocked"; tool: string; reason: string }
  | { role: "system"; content: string };

// Permission ask event from backend
export interface PermissionAsk {
  id: string;
  tool: string;
  args: string;
  reason: string;
}

// Session metadata (for sidebar list)
export interface SessionMeta {
  id: string;
  title: string;
  created_at: string;
  updated_at: string;
  turn_count: number;
}

// Full session record (with messages)
export interface SessionRecord extends SessionMeta {
  messages: ChatMessage[];
  model: string;
}

// App state
export interface AppState {
  messages: ChatMessage[];
  model: string;
  connected: boolean;
  busy: boolean;
  turnCount: number;
  pendingPermission: PermissionAsk | null;
}

// Model preset (for model selector)
export interface ModelPreset {
  name: string;
  type: string;
  base_url?: string;
  api_key?: string;
  model_name: string;
}

// Model list response
export interface ModelsResponse {
  models: ModelPreset[];
  active: string;
}
