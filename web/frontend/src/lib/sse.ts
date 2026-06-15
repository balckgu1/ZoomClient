import type { SSEEvent } from "../types";

export type SSEHandler = (event: SSEEvent) => void;
export type DisconnectFn = () => void;

/**
 * Connect to the SSE endpoint and invoke handler for each event.
 * Auto-reconnects with exponential backoff on disconnect.
 */
export function connectSSE(
  url: string,
  onEvent: SSEHandler,
  onStatusChange?: (connected: boolean) => void
): DisconnectFn {
  let es: EventSource | null = null;
  let retryDelay = 1000;
  let stopped = false;

  function connect() {
    if (stopped) return;
    es = new EventSource(url);

    es.onopen = () => {
      retryDelay = 1000; // reset on successful connection
      onStatusChange?.(true);
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
      onStatusChange?.(false);
      es?.close();
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
