// desktop/src/lib/EmotionFSM.ts
// 宠物情绪状态机：6 种状态 + 自动回落定时器
import { emotionState } from "./SidecarBridge";

type EmotionName = "idle" | "thinking" | "executing" | "talking" | "happy" | "error";

let fallbackTimer: ReturnType<typeof setTimeout> | null = null;

function clearFallback() {
  if (fallbackTimer) { clearTimeout(fallbackTimer); fallbackTimer = null; }
}

function setWithFallback(state: EmotionName, fallbackMs: number) {
  clearFallback();
  emotionState.set(state);
  fallbackTimer = setTimeout(() => emotionState.set("idle"), fallbackMs);
}

/** 由 SidecarBridge 的情绪事件驱动调用 */
export function transitionFromBackend(newState: string) {
  switch (newState) {
    case "thinking":  setWithFallback("thinking", 30_000); break;
    case "executing": setWithFallback("executing", 60_000); break;
    case "idle":      clearFallback(); emotionState.set("idle"); break;
    case "error":     setWithFallback("error", 5_000); break;
  }
}

/** 由前端推断：收到首个 token → talking */
export function transitionToTalking() {
  setWithFallback("talking", 60_000);
}

/** 由前端推断：done → happy → 3s → idle */
export function transitionToHappy() {
  setWithFallback("happy", 3_000);
}
