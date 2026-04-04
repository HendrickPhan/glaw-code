"use client";

import { useState } from "react";
import { ChatMessage } from "@/lib/types";
import MarkdownRenderer from "./MarkdownRenderer";
import ToolCall from "./ToolCall";

function toolIcon(name: string) {
  switch (name) {
    case "bash":
      return "$";
    case "write_file":
    case "edit_file":
      return "✎";
    case "read_file":
    case "glob_search":
    case "grep_search":
      return "📄";
    case "web_fetch":
    case "web_search":
      return "🌐";
    default:
      return "⚙";
  }
}

export default function MessageBubble({ message }: { message: ChatMessage }) {
  const [collapsed, setCollapsed] = useState(false);

  if (message.role === "tool") {
    return <ToolCall message={message} />;
  }

  const isUser = message.role === "user";
  const lines = message.content.split("\n");
  const isLong = lines.length > 3;
  const icon = isUser ? "👤" : "✦";

  return (
    <div
      className={`group px-4 py-3 ${
        isUser
          ? "bg-zinc-800/40"
          : "bg-transparent"
      }`}
    >
      <div className="max-w-3xl mx-auto flex gap-3">
        <div
          className={`shrink-0 w-7 h-7 rounded-full flex items-center justify-center text-sm ${
            isUser
              ? "bg-blue-600/20 text-blue-400"
              : "bg-emerald-600/20 text-emerald-400"
          }`}
        >
          {icon}
        </div>
        <div className="flex-1 min-w-0">
          <div className="text-xs text-zinc-500 mb-1 font-medium">
            {isUser ? "You" : "glaw"}
          </div>

          {isUser ? (
            <div className="text-zinc-200 whitespace-pre-wrap">{message.content}</div>
          ) : (
            <>
              {isLong && collapsed ? (
                <div>
                  <button
                    onClick={() => setCollapsed(false)}
                    className="text-zinc-500 hover:text-zinc-300 text-sm flex items-center gap-1 transition-colors"
                  >
                    <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                      <polyline points="6 9 12 15 18 9" />
                    </svg>
                    {lines[0].slice(0, 80)}
                    {lines[0].length > 80 ? "..." : ""}
                    <span className="text-zinc-600 ml-1">({lines.length} lines)</span>
                  </button>
                </div>
              ) : (
                <div className="relative">
                  <div className="prose-invert">
                    <MarkdownRenderer content={message.content} />
                  </div>
                  {isLong && (
                    <button
                      onClick={() => setCollapsed(true)}
                      className="mt-1 text-xs text-zinc-600 hover:text-zinc-400 transition-colors"
                    >
                      Collapse
                    </button>
                  )}
                </div>
              )}
            </>
          )}
        </div>
      </div>
    </div>
  );
}
