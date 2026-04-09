"use client";

import { CommandSpec, formatCommandHelp } from "@/lib/commands";
import { Check } from "lucide-react";

interface CommandPaletteProps {
  isOpen: boolean;
  commands: CommandSpec[];
  selectedIndex: number;
  onSelect: (spec: CommandSpec) => void;
  onClose: () => void;
}

export default function CommandPalette({
  isOpen,
  commands,
  selectedIndex,
  onSelect,
  onClose,
}: CommandPaletteProps) {
  if (!isOpen || commands.length === 0) return null;

  // Group commands by category
  const grouped = commands.reduce((acc, cmd) => {
    if (!acc[cmd.category]) {
      acc[cmd.category] = [];
    }
    acc[cmd.category].push(cmd);
    return acc;
  }, {} as Record<string, CommandSpec[]>);

  const categoryOrder: Array<CommandSpec["category"]> = ["core", "session", "workspace", "git", "automation"];

  return (
    <div className="absolute bottom-full mb-2 left-0 right-0 bg-zinc-900 border border-zinc-700 rounded-xl shadow-2xl overflow-hidden z-50 max-h-96 overflow-y-auto">
      {categoryOrder.map((category) => {
        const categoryCommands = grouped[category];
        if (!categoryCommands || categoryCommands.length === 0) return null;

        return (
          <div key={category}>
            <div className="px-4 py-2 bg-zinc-950 border-b border-zinc-800">
              <span className="text-xs font-semibold text-zinc-500 uppercase tracking-wider">
                {category === "core" && "Core Commands"}
                {category === "session" && "Session Management"}
                {category === "workspace" && "Workspace"}
                {category === "git" && "Git Operations"}
                {category === "automation" && "Automation"}
              </span>
            </div>
            {categoryCommands.map((cmd, idx) => {
              const globalIndex = commands.indexOf(cmd);
              return (
                <button
                  key={cmd.cmd}
                  onClick={() => onSelect(cmd)}
                  className={`w-full text-left px-4 py-2.5 flex items-center gap-3 transition-colors ${
                    globalIndex === selectedIndex ? "bg-emerald-900/30" : "hover:bg-zinc-800"
                  }`}
                >
                  <span className={`font-mono text-sm shrink-0 w-32 ${
                    globalIndex === selectedIndex ? "text-emerald-400" : "text-emerald-500"
                  }`}>
                    {cmd.cmd}
                  </span>
                  <span className="text-zinc-400 text-sm flex-1">{cmd.desc}</span>
                  {globalIndex === selectedIndex && (
                    <Check className="w-4 h-4 text-emerald-500 shrink-0" />
                  )}
                </button>
              );
            })}
          </div>
        );
      })}
      <div className="px-4 py-2 bg-zinc-950 border-t border-zinc-800">
        <div className="flex items-center gap-4 text-xs text-zinc-600">
          <span className="flex items-center gap-1">
            <kbd className="px-1.5 py-0.5 bg-zinc-800 rounded text-zinc-400">↑↓</kbd>
            <span>Navigate</span>
          </span>
          <span className="flex items-center gap-1">
            <kbd className="px-1.5 py-0.5 bg-zinc-800 rounded text-zinc-400">Tab</kbd>
            <span>Autocomplete</span>
          </span>
          <span className="flex items-center gap-1">
            <kbd className="px-1.5 py-0.5 bg-zinc-800 rounded text-zinc-400">Enter</kbd>
            <span>Select</span>
          </span>
          <span className="flex items-center gap-1">
            <kbd className="px-1.5 py-0.5 bg-zinc-800 rounded text-zinc-400">Esc</kbd>
            <span>Close</span>
          </span>
        </div>
      </div>
    </div>
  );
}
