import type { SSEEvent } from "../types";

export type SSEHandler = (event: SSEEvent) => void;
export type StatusChangeHandler = (connected: boolean, retryCount?: number) => void;
export type DisconnectFn = () => void;

/**
 * Connect to the SSE endpoint and invoke handler for each event.
 * Auto-reconnects with exponential backoff on disconnect.
 * Distinguishes initial connection failure from mid-session disconnect.
 */
export function connectSSE(
  url: string,
  onEvent: SSEHandler,
  onStatusChange?: StatusChangeHandler
): DisconnectFn {
  let es: EventSource | null = null;
  let retryDelay = 1000;
  let retryCount = 0;
  let stopped = false;

  function connect() {
    if (stopped) return;
    es = new EventSource(url);

    es.onopen = () => {
      retryDelay = 1000; // reset on successful connection
      retryCount = 0;
      onStatusChange?.(true, 0);
    };

    es.onmessage = (e) => {
      try {
        const evt: SSEEvent = JSON.parse(e.data);
        onEvent(evt);
      } catch {
        console.warn("SSE parse error:", e.data);
      }
    };

    es.onerror = () => {
      retryCount += 1;
      // EventSource readyState: 0=CONNECTING, 1=OPEN, 2=CLOSED
      const wasConnected = es?.readyState === EventSource.CLOSED;
      es?.close();

      if (wasConnected) {
        // Connection dropped after being established
        console.warn(`SSE disconnected (retry #${retryCount}), reconnecting in ${retryDelay}ms...`);
      } else {
        // Initial connection failed (e.g. server not up yet)
        console.warn(`SSE connection failed (attempt #${retryCount}), retrying in ${retryDelay}ms...`);
      }

      onStatusChange?.(false, retryCount);

      if (!stopped) {
        setTimeout(connect, retryDelay);
        retryDelay = Math.min(retryDelay * 2, 10000);
      }
    };
  }

  connect();

  return () => {
    stopped = true;
    es?.close();
  };
}
