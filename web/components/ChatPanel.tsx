"use client";

import { useEffect, useRef } from "react";
import { ChatMessage } from "@/lib/types";
import MessageBubble from "./MessageBubble";
import ToolCall from "./ToolCall";
import CommandResult from "./CommandResult";

export default function ChatPanel({
  messages,
  loading,
}: {
  messages: ChatMessage[];
  loading: boolean;
}) {
  const bottom = useRef<HTMLDivElement>(null);

  useEffect(() => {
    bottom.current?.scrollIntoView({ behavior: "smooth" });
  }, [messages]);

  return (
    <div className="flex-1 overflow-y-auto">
      {messages.length === 0 ? (
        <div className="flex items-center justify-center h-full">
          <div className="text-center space-y-3 max-w-md">
            <div className="text-4xl">✦</div>
            <h2 className="text-xl font-semibold text-zinc-300">
              Welcome to glaw-code
            </h2>
            <p className="text-zinc-500 text-sm leading-relaxed">
              Your AI coding assistant. Ask me to create files, edit code, run
              commands, search your codebase, or use <span className="text-emerald-400 font-mono">/</span> for commands.
            </p>
            <div className="flex flex-wrap justify-center gap-2 pt-2">
              {[
                "Create a Go HTTP server",
                "Explain the project structure",
                "Find all TODO comments",
                "Write unit tests for main.go",
              ].map((suggestion) => (
                <div
                  key={suggestion}
                  className="text-xs text-zinc-500 bg-zinc-900 border border-zinc-800 rounded-lg px-3 py-1.5"
                >
                  {suggestion}
                </div>
              ))}
            </div>
            <div className="pt-4 border-t border-zinc-800 mt-4">
              <p className="text-xs text-zinc-600 mb-2">Quick commands:</p>
              <div className="flex flex-wrap justify-center gap-2">
                {["/help", "/status", "/analyze", "/agents"].map((cmd) => (
                  <div
                    key={cmd}
                    className="text-xs text-emerald-600 bg-emerald-950/50 border border-emerald-900/50 rounded-lg px-3 py-1 font-mono"
                  >
                    {cmd}
                  </div>
                ))}
              </div>
            </div>
          </div>
        </div>
      ) : (
        <div className="divide-y divide-zinc-800/30">
          {messages.map((msg) => {
            if (msg.role === "tool") {
              return <ToolCall key={msg.id} message={msg} />;
            }
            if (msg.isCommand) {
              return (
                <CommandResult
                  key={msg.id}
                  command={msg.command || "unknown"}
                  message={msg.content}
                />
              );
            }
            return <MessageBubble key={msg.id} message={msg} />;
          })}
        </div>
      )}

      {/* Loading indicator */}
      {loading && (
        <div className="px-4 py-3">
          <div className="max-w-3xl mx-auto flex items-center gap-2 text-zinc-500">
            <div className="w-2 h-2 bg-emerald-500 rounded-full animate-pulse" />
            <span className="text-sm">Thinking...</span>
          </div>
        </div>
      )}

      <div ref={bottom} />
    </div>
  );
}
