"use client";

import { useState, useRef, useEffect } from "react";
import { useCommands } from "@/hooks/useCommands";
import CommandPalette from "@/components/CommandPalette";
import { Send } from "lucide-react";

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
  const textarea = useRef<HTMLTextAreaElement>(null);

  const {
    isOpen,
    filteredCommands,
    selectedIndex,
    handleInputChange,
    handleKeyDown,
    selectCommand,
    closePalette,
    registerClearCallback,
    inputRef,
  } = useCommands(onCommand);

  // Register a callback that properly clears React state
  useEffect(() => {
    registerClearCallback(() => {
      setInput("");
    });
  }, [registerClearCallback]);

  // Auto-resize textarea
  useEffect(() => {
    if (textarea.current && inputRef.current) {
      textarea.current = inputRef.current;
    }
    if (textarea.current) {
      textarea.current.style.height = "auto";
      textarea.current.style.height =
        Math.min(textarea.current.scrollHeight, 160) + "px";
    }
  }, [input, inputRef]);

  const handleSubmit = () => {
    const text = input.trim();
    if (!text) return;

    if (text.startsWith("/")) {
      const fullCommand = text.startsWith("/") ? text : "/" + text;
      onCommand(fullCommand);
    } else {
      onSend(text);
    }
    setInput("");
    closePalette();
  };

  const handleChange = (e: React.ChangeEvent<HTMLTextAreaElement>) => {
    const value = e.target.value;
    setInput(value);
    handleInputChange(value);
  };

  const combinedKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    // Let useCommands handle keyboard shortcuts when palette is open
    if (isOpen) {
      handleKeyDown(e);
      return;
    }

    // Default Enter behavior
    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault();
      handleSubmit();
    }
  };

  return (
    <div className="border-t border-zinc-800 bg-zinc-950 px-4 py-3">
      <div className="max-w-3xl mx-auto relative">
        {/* Command palette */}
        <CommandPalette
          isOpen={isOpen}
          commands={filteredCommands}
          selectedIndex={selectedIndex}
          onSelect={selectCommand}
          onClose={closePalette}
        />

        <div className="flex items-end gap-2">
          <textarea
            ref={inputRef}
            value={input}
            onChange={handleChange}
            onKeyDown={combinedKeyDown}
            placeholder={
              disabled ? "Running..." : "Message glaw... (Type / for commands)"
            }
            disabled={disabled}
            rows={1}
            className="flex-1 bg-zinc-900 border border-zinc-700 rounded-xl px-4 py-2.5 text-sm text-zinc-200 placeholder-zinc-600 focus:outline-none focus:border-emerald-600 resize-none disabled:opacity-50 transition-colors"
          />
          <button
            onClick={handleSubmit}
            disabled={disabled || !input.trim()}
            className="shrink-0 bg-emerald-600 hover:bg-emerald-500 disabled:bg-zinc-700 disabled:opacity-50 text-white rounded-lg p-2.5 transition-colors"
            title="Send message"
          >
            <Send className="w-5 h-5" />
          </button>
        </div>

        {/* Command hint */}
        {isOpen && filteredCommands.length > 0 && (
          <div className="mt-2 text-xs text-zinc-600 flex items-center gap-3">
            <span className="flex items-center gap-1">
              <kbd className="px-1.5 py-0.5 bg-zinc-800 rounded text-zinc-400">↑↓</kbd>
              Navigate
            </span>
            <span className="flex items-center gap-1">
              <kbd className="px-1.5 py-0.5 bg-zinc-800 rounded text-zinc-400">Tab</kbd>
              Autocomplete
            </span>
            <span className="flex items-center gap-1">
              <kbd className="px-1.5 py-0.5 bg-zinc-800 rounded text-zinc-400">Enter</kbd>
              Select
            </span>
            <span className="flex items-center gap-1">
              <kbd className="px-1.5 py-0.5 bg-zinc-800 rounded text-zinc-400">Esc</kbd>
              Close
            </span>
          </div>
        )}
      </div>
    </div>
  );
}
