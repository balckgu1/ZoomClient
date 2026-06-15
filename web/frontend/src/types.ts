// SSE event from backend
export interface SSEEvent {
  ch: "agent" | "system" | "emotion";
  data: Record<string, unknown>;
}

// Chat message types displayed in the UI
export type ChatMessage =
  | { role: "user"; content: string }
  | { role: "assistant"; content: string }
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

// App state
export interface AppState {
  messages: ChatMessage[];
  model: string;
  connected: boolean;
  busy: boolean;
  turnCount: number;
  pendingPermission: PermissionAsk | null;
}
