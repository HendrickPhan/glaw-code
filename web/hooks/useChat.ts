"use client";

import { useState, useCallback, useRef } from "react";
import { ChatMessage, SessionInfo, ToolResultInfo, ToolUseInfo, UsageInfo, WSIncoming } from "@/lib/types";

let msgCounter = 0;
function nextId() {
  return `msg_${++msgCounter}_${Date.now()}`;
}

export function useChat() {
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [loading, setLoading] = useState(false);
  const [sessionId, setSessionId] = useState<string>("");
  const [sessions, setSessions] = useState<SessionInfo[]>([]);
  const [usage, setUsage] = useState<UsageInfo | null>(null);

  const toolStartTimes = useRef<Map<string, number>>(new Map());

  const addUserMessage = useCallback((content: string) => {
    const msg: ChatMessage = {
      id: nextId(),
      role: "user",
      content,
      timestamp: Date.now(),
    };
    setMessages((prev) => [...prev, msg]);
  }, []);

  const addAssistantMessage = useCallback((content: string) => {
    if (!content.trim()) return;
    const msg: ChatMessage = {
      id: nextId(),
      role: "assistant",
      content,
      timestamp: Date.now(),
    };
    setMessages((prev) => [...prev, msg]);
  }, []);

  const addToolUse = useCallback(
    (id: string, name: string, input: string) => {
      const msg: ChatMessage = {
        id: nextId(),
        role: "tool",
        content: "",
        timestamp: Date.now(),
        toolUse: { id, name, input },
      };
      setMessages((prev) => [...prev, msg]);
      toolStartTimes.current.set(id, Date.now());
      setLoading(true);
    },
    []
  );

  const addToolResult = useCallback((id: string, content: string, isError: boolean) => {
    const elapsed = toolStartTimes.current.has(id)
      ? Date.now() - toolStartTimes.current.get(id)!
      : 0;
    toolStartTimes.current.delete(id);

    setMessages((prev) =>
      prev.map((msg) =>
        msg.toolUse?.id === id
          ? {
              ...msg,
              toolResult: { id, content, isError, elapsed },
            }
          : msg
      )
    );
  }, []);

  const clearMessages = useCallback(() => setMessages([]), []);

  const loadHistory = useCallback((historyItems: WSIncoming[]) => {
    const restored: ChatMessage[] = [];
    for (const item of historyItems) {
      const role = item.role as string;
      if (role === "user" || role === "assistant") {
        restored.push({
          id: nextId(),
          role: role as "user" | "assistant",
          content: (item.content as string) || "",
          timestamp: Date.now(),
        });
      } else if (role === "tool") {
        restored.push({
          id: nextId(),
          role: "tool",
          content: "",
          timestamp: Date.now(),
          toolUse: item.toolUse as ToolUseInfo | undefined,
          toolResult: item.toolResult as ToolResultInfo | undefined,
        });
      }
    }
    setMessages(restored);
  }, []);

  const handleWSMessage = useCallback(
    (msg: WSIncoming) => {
      const type = msg.type as string;
      const d = msg.data || msg;
      const sid = msg.session_id as string | undefined;

      switch (type) {
        case "session_list":
          if (Array.isArray(d)) setSessions(d);
          break;
        case "session_created":
          if (d?.session_id) {
            setSessionId(d.session_id as string);
          }
          break;
        case "session_switched":
        case "history":
          if (Array.isArray(d?.messages)) {
            loadHistory(d.messages as WSIncoming[]);
          }
          break;
        case "user_message":
          // Already added locally
          break;
        case "assistant_message":
          if (d?.content) addAssistantMessage(d.content as string);
          break;
        case "tool_use":
          if (d?.id && d?.name) {
            addToolUse(d.id as string, d.name as string, (d.input as string) || "");
          }
          break;
        case "tool_result":
          if (d?.id) {
            addToolResult(d.id as string, (d.content as string) || "", (d.isError as boolean) || false);
          }
          break;
        case "done":
          setLoading(false);
          if (sid && !sessionId) setSessionId(sid);
          break;
        case "error":
          setLoading(false);
          const errMsg = d?.error || "Unknown error";
          addAssistantMessage(`Error: ${errMsg}`);
          break;
        case "usage":
          if (d) setUsage(d as UsageInfo);
          break;
      }
    },
    [addAssistantMessage, addToolUse, addToolResult, sessionId, loadHistory]
  );

  return {
    messages,
    loading,
    sessionId,
    sessions,
    usage,
    setSessionId,
    addUserMessage,
    clearMessages,
    handleWSMessage,
  };
}
