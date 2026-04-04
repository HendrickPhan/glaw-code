"use client";

import { useState } from "react";
import { ChatMessage } from "@/lib/types";

function toolIcon(name: string) {
  switch (name) {
    case "bash": return "$";
    case "write_file": case "edit_file": return "✎";
    case "read_file": case "glob_search": case "grep_search": return "📄";
    case "web_fetch": case "web_search": return "🌐";
    default: return "⚙";
  }
}

function toolLabel(name: string) {
  switch (name) {
    case "bash": return "Ran command";
    case "write_file": return "Wrote file";
    case "edit_file": return "Edited file";
    case "read_file": return "Read file";
    case "glob_search": return "Searched files";
    case "grep_search": return "Searched content";
    default: return name;
  }
}

function parseToolInput(name: string, input: string): string {
  try {
    const obj = JSON.parse(input);
    switch (name) {
      case "bash":
        return obj.command || input;
      case "write_file":
      case "edit_file":
      case "read_file":
        return obj.path || input;
      case "glob_search":
        return obj.pattern || input;
      case "grep_search":
        return obj.pattern || input;
      default:
        return input.slice(0, 80);
    }
  } catch {
    return input.slice(0, 80);
  }
}

export default function ToolCall({ message }: { message: ChatMessage }) {
  const [expanded, setExpanded] = useState(false);
  const tu = message.toolUse;
  const tr = message.toolResult;

  if (!tu) return null;

  const displayInput = parseToolInput(tu.name, tu.input);
  const isRunning = !tr;
  const elapsed = tr?.elapsed ? `${(tr.elapsed / 1000).toFixed(1)}s` : "";

  return (
    <div className="px-4 py-1.5">
      <div className="max-w-3xl mx-auto">
        <button
          onClick={() => setExpanded(!expanded)}
          className={`w-full text-left flex items-center gap-2 py-1.5 px-3 rounded-lg border transition-colors ${
            isRunning
              ? "border-yellow-800/50 bg-yellow-900/10"
              : tr?.isError
              ? "border-red-800/50 bg-red-900/10"
              : "border-zinc-700/50 bg-zinc-800/30 hover:bg-zinc-800/50"
          }`}
        >
          {/* Spinner when running */}
          {isRunning ? (
            <div className="w-4 h-4 border-2 border-yellow-500 border-t-transparent rounded-full animate-spin shrink-0" />
          ) : (
            <span
              className={`text-sm shrink-0 ${
                tr?.isError ? "text-red-400" : "text-emerald-400"
              }`}
            >
              {tr?.isError ? "✗" : "✓"}
            </span>
          )}

          <span className="text-yellow-500 text-sm shrink-0">
            {toolIcon(tu.name)}
          </span>
          <span className="text-sm font-medium text-zinc-300 shrink-0">
            {toolLabel(tu.name)}
          </span>
          <span className="text-sm text-zinc-500 truncate flex-1">
            {displayInput}
          </span>
          {elapsed && (
            <span className="text-xs text-zinc-600 shrink-0">{elapsed}</span>
          )}
          <svg
            className={`w-4 h-4 text-zinc-600 transition-transform shrink-0 ${
              expanded ? "rotate-180" : ""
            }`}
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            strokeWidth="2"
          >
            <polyline points="6 9 12 15 18 9" />
          </svg>
        </button>

        {expanded && (
          <div className="mt-1 ml-6 border-l-2 border-zinc-800 pl-3 space-y-2 py-1">
            {/* Tool input */}
            <div>
              <div className="text-xs text-zinc-500 mb-0.5">Input</div>
              <pre className="text-xs text-zinc-400 bg-zinc-900/50 rounded p-2 overflow-x-auto max-h-40 whitespace-pre-wrap break-all">
                {(() => {
                  try {
                    return JSON.stringify(JSON.parse(tu.input), null, 2);
                  } catch {
                    return tu.input;
                  }
                })()}
              </pre>
            </div>
            {/* Tool result */}
            {tr && (
              <div>
                <div className="text-xs text-zinc-500 mb-0.5">
                  {tr.isError ? "Error" : "Output"}
                </div>
                <pre
                  className={`text-xs rounded p-2 overflow-x-auto max-h-60 whitespace-pre-wrap break-all ${
                    tr.isError
                      ? "text-red-400 bg-red-950/30"
                      : "text-zinc-400 bg-zinc-900/50"
                  }`}
                >
                  {tr.content.length > 2000
                    ? tr.content.slice(0, 2000) + "\n... (truncated)"
                    : tr.content}
                </pre>
              </div>
            )}
          </div>
        )}
      </div>
    </div>
  );
}
