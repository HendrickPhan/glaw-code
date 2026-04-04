"use client";

import { SessionInfo } from "@/lib/types";

export default function Sidebar({
  sessions,
  currentId,
  onSelect,
  onNew,
}: {
  sessions: SessionInfo[];
  currentId: string;
  onSelect: (id: string) => void;
  onNew: () => void;
}) {
  return (
    <div className="w-64 bg-zinc-950 border-r border-zinc-800 flex flex-col h-full">
      {/* Header */}
      <div className="p-3 border-b border-zinc-800">
        <div className="flex items-center justify-between">
          <h1 className="text-sm font-bold text-zinc-200 tracking-wide">
            glaw-code
          </h1>
          <button
            onClick={onNew}
            className="text-zinc-500 hover:text-zinc-300 p-1 rounded hover:bg-zinc-800 transition-colors"
            title="New session"
          >
            <svg
              width="16"
              height="16"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              strokeWidth="2"
            >
              <line x1="12" y1="5" x2="12" y2="19" />
              <line x1="5" y1="12" x2="19" y2="12" />
            </svg>
          </button>
        </div>
      </div>

      {/* Session list */}
      <div className="flex-1 overflow-y-auto py-1">
        {sessions.length === 0 ? (
          <div className="text-xs text-zinc-600 text-center mt-8">
            No sessions yet
          </div>
        ) : (
          sessions.map((s) => (
            <button
              key={s.id}
              onClick={() => onSelect(s.id)}
              className={`w-full text-left px-3 py-2 text-sm transition-colors ${
                s.id === currentId
                  ? "bg-zinc-800/60 text-zinc-200"
                  : "text-zinc-500 hover:text-zinc-300 hover:bg-zinc-900"
              }`}
            >
              <div className="truncate font-medium">{s.id}</div>
              <div className="text-xs text-zinc-600 mt-0.5">
                {s.message_count} msgs ·{" "}
                {new Date(s.created_at).toLocaleDateString()}
              </div>
            </button>
          ))
        )}
      </div>

      {/* Footer */}
      <div className="p-3 border-t border-zinc-800 text-xs text-zinc-600">
        Powered by glaw-code
      </div>
    </div>
  );
}
