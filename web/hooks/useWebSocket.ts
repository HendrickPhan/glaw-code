"use client";

import { useEffect, useRef, useState, useCallback } from "react";
import { WSMessage } from "@/lib/types";

export function useWebSocket(url: string) {
  const ws = useRef<WebSocket | null>(null);
  const [connected, setConnected] = useState(false);
  const handlers = useRef<Map<string, Set<(data: unknown) => void>>>(
    new Map()
  );

  const connect = useCallback(() => {
    if (ws.current?.readyState === WebSocket.OPEN) return;

    const socket = new WebSocket(url);

    socket.onopen = () => setConnected(true);
    socket.onclose = () => {
      setConnected(false);
      setTimeout(connect, 2000);
    };
    socket.onerror = () => socket.close();

    socket.onmessage = (event) => {
      try {
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        const msg: any = JSON.parse(event.data);
        const type = msg.type as string;

        const set = handlers.current.get(type);
        if (set) {
          set.forEach((handler) => handler(msg.data ?? msg));
        }
        const wildcards = handlers.current.get("*");
        if (wildcards) {
          wildcards.forEach((handler) => handler(msg));
        }
      } catch {
        // ignore parse errors
      }
    };

    ws.current = socket;
  }, [url]);

  useEffect(() => {
    connect();
    return () => ws.current?.close();
  }, [connect]);

  const send = useCallback((msg: WSMessage) => {
    if (ws.current?.readyState === WebSocket.OPEN) {
      ws.current.send(JSON.stringify(msg));
    }
  }, []);

  const on = useCallback(
    (type: string, handler: (data: unknown) => void) => {
      if (!handlers.current.has(type)) {
        handlers.current.set(type, new Set());
      }
      handlers.current.get(type)!.add(handler);
      return () => {
        handlers.current.get(type)?.delete(handler);
      };
    },
    []
  );

  return { connected, send, on };
}
