"use client";

import { useState, useRef, useEffect } from "react";

const COMMANDS = [
  { cmd: "/help", desc: "Show available commands" },
  { cmd: "/clear", desc: "Clear conversation" },
  { cmd: "/model", desc: "Show or change model" },
  { cmd: "/cost", desc: "Show cost summary" },
  { cmd: "/revert", desc: "Revert last changes" },
  { cmd: "/revert all", desc: "Revert all changes" },
  { cmd: "/status", desc: "Show runtime status" },
  { cmd: "/compact", desc: "Compact conversation history" },
  { cmd: "/diff", desc: "Show pending changes" },
  { cmd: "/session list", desc: "List sessions" },
  { cmd: "/quit", desc: "Exit" },
];

export default function InputBar({
  onSend,
  onCommand,
  disabled,
}: {
  onSend: (msg: string) => void;
  onCommand: (cmd: string) => void;
  disabled?: boolean;
}) {
  const [input, setInput] = useState("");
  const [showCommands, setShowCommands] = useState(false);
  const [filter, setFilter] = useState("");
  const textarea = useRef<HTMLTextAreaElement>(null);
  const cmdRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (textarea.current) {
      textarea.current.style.height = "auto";
      textarea.current.style.height =
        Math.min(textarea.current.scrollHeight, 160) + "px";
    }
  }, [input]);

  useEffect(() => {
    function handleClick(e: MouseEvent) {
      if (cmdRef.current && !cmdRef.current.contains(e.target as Node)) {
        setShowCommands(false);
      }
    }
    document.addEventListener("mousedown", handleClick);
    return () => document.removeEventListener("mousedown", handleClick);
  }, []);

  const filtered = COMMANDS.filter((c) =>
    c.cmd.startsWith(filter)
  );

  const handleSubmit = () => {
    const text = input.trim();
    if (!text) return;

    if (text.startsWith("/")) {
      onCommand(text);
    } else {
      onSend(text);
    }
    setInput("");
    setShowCommands(false);
  };

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault();
      handleSubmit();
    }
    if (e.key === "Escape") {
      setShowCommands(false);
    }
  };

  const handleChange = (val: string) => {
    setInput(val);
    if (val.startsWith("/")) {
      setFilter(val);
      setShowCommands(true);
    } else {
      setShowCommands(false);
    }
  };

  return (
    <div className="border-t border-zinc-800 bg-zinc-950 px-4 py-3">
      <div className="max-w-3xl mx-auto relative" ref={cmdRef}>
        {/* Command palette */}
        {showCommands && filtered.length > 0 && (
          <div className="absolute bottom-full mb-2 left-0 right-0 bg-zinc-900 border border-zinc-700 rounded-xl shadow-xl overflow-hidden z-10">
            {filtered.map((c) => (
              <button
                key={c.cmd}
                onClick={() => {
                  onCommand(c.cmd);
                  setInput("");
                  setShowCommands(false);
                }}
                className="w-full text-left px-4 py-2 hover:bg-zinc-800 flex items-center gap-3 transition-colors"
              >
                <span className="text-emerald-400 font-mono text-sm">
                  {c.cmd}
                </span>
                <span className="text-zinc-500 text-sm">{c.desc}</span>
              </button>
            ))}
          </div>
        )}

        <div className="flex items-end gap-2">
          <textarea
            ref={textarea}
            value={input}
            onChange={(e) => handleChange(e.target.value)}
            onKeyDown={handleKeyDown}
            placeholder={
              disabled ? "Running..." : "Message glaw... (/ for commands)"
            }
            disabled={disabled}
            rows={1}
            className="flex-1 bg-zinc-900 border border-zinc-700 rounded-xl px-4 py-2.5 text-sm text-zinc-200 placeholder-zinc-600 focus:outline-none focus:border-zinc-500 resize-none disabled:opacity-50 transition-colors"
          />
          <button
            onClick={handleSubmit}
            disabled={disabled || !input.trim()}
            className="shrink-0 bg-emerald-600 hover:bg-emerald-500 disabled:bg-zinc-700 disabled:opacity-50 text-white rounded-lg p-2.5 transition-colors"
          >
            <svg
              width="18"
              height="18"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              strokeWidth="2"
              strokeLinecap="round"
              strokeLinejoin="round"
            >
              <line x1="22" y1="2" x2="11" y2="13" />
              <polygon points="22 2 15 22 11 13 2 9 22 2" />
            </svg>
          </button>
        </div>
      </div>
    </div>
  );
}
