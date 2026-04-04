"use client";

import { useCallback, useEffect } from "react";
import { useWebSocket } from "@/hooks/useWebSocket";
import { useChat } from "@/hooks/useChat";
import ChatPanel from "@/components/ChatPanel";
import InputBar from "@/components/InputBar";
import Sidebar from "@/components/Sidebar";
import UsageBar from "@/components/UsageBar";

export default function Home() {
  const wsUrl =
    typeof window !== "undefined"
      ? `${window.location.protocol === "https:" ? "wss:" : "ws:"}//${
          window.location.host
        }/ws`
      : "";

  const { connected, send, on } = useWebSocket(wsUrl);
  const chat = useChat();

  // Wire WebSocket messages to chat state
  useEffect(() => {
    const unsub = on("*", (msg) => {
      chat.handleWSMessage(msg as { type: string; data?: unknown });
    });
    return unsub;
  }, [on, chat]);

  // Request session list on connect
  useEffect(() => {
    if (connected) {
      send({ type: "list_sessions" });
    }
  }, [connected, send]);

  const handleSend = useCallback(
    (content: string) => {
      chat.addUserMessage(content);
      send({
        type: "chat",
        data: {
          session_id: chat.sessionId || undefined,
          content,
        },
      });
    },
    [chat, send]
  );

  const handleCommand = useCallback(
    (cmd: string) => {
      send({
        type: "command",
        data: {
          session_id: chat.sessionId || undefined,
          command: cmd.replace(/^\//, ""),
        },
      });
      if (cmd === "/clear") {
        chat.clearMessages();
      }
    },
    [chat, send]
  );

  const handleNewSession = useCallback(() => {
    send({ type: "create_session" });
    chat.clearMessages();
  }, [send, chat]);

  const handleSelectSession = useCallback(
    (id: string) => {
      chat.clearMessages();
      chat.setSessionId(id);
      send({
        type: "switch_session",
        data: { session_id: id },
      });
    },
    [chat, send]
  );

  return (
    <div className="flex h-screen bg-zinc-900 text-zinc-200">
      {/* Sidebar */}
      <Sidebar
        sessions={chat.sessions}
        currentId={chat.sessionId}
        onSelect={handleSelectSession}
        onNew={handleNewSession}
      />

      {/* Main chat area */}
      <div className="flex-1 flex flex-col min-w-0">
        {/* Connection status bar */}
        <div className="h-8 flex items-center px-4 border-b border-zinc-800 bg-zinc-950/50 shrink-0">
          <div className="flex items-center gap-2">
            <div
              className={`w-2 h-2 rounded-full ${
                connected ? "bg-emerald-500" : "bg-red-500 animate-pulse"
              }`}
            />
            <span className="text-xs text-zinc-500">
              {connected ? "Connected" : "Connecting..."}
            </span>
          </div>
          {chat.sessionId && (
            <span className="text-xs text-zinc-600 ml-4 font-mono">
              {chat.sessionId}
            </span>
          )}
        </div>

        {/* Messages */}
        <ChatPanel messages={chat.messages} loading={chat.loading} />

        {/* Usage */}
        <UsageBar usage={chat.usage} />

        {/* Input */}
        <InputBar
          onSend={handleSend}
          onCommand={handleCommand}
          disabled={chat.loading}
        />
      </div>
    </div>
  );
}
