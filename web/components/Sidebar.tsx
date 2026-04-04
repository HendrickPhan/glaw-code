"use client";

import { useState } from "react";
import { SessionInfo, WorkspaceInfo } from "@/lib/types";
import WorkspacePanel from "./WorkspacePanel";

// Tab type for the sidebar sections
type SidebarTab = "sessions" | "workspaces";

export default function Sidebar({
  sessions,
  currentId,
  onSelect,
  onNew,
  workspaces,
  onActivateWorkspace,
  onCreateWorkspace,
  onDeleteWorkspace,
  onCreateSection,
  onDeleteSection,
}: {
  sessions: SessionInfo[];
  currentId: string;
  onSelect: (id: string) => void;
  onNew: () => void;
  workspaces: WorkspaceInfo[];
  onActivateWorkspace: (id: string) => void;
  onCreateWorkspace: (name: string, path: string, description: string) => void;
  onDeleteWorkspace: (id: string) => void;
  onCreateSection: (workspaceId: string, name: string, description: string, color: string) => void;
  onDeleteSection: (workspaceId: string, sectionId: string) => void;
}) {
  const [activeTab, setActiveTab] = useState<SidebarTab>("sessions");

  // Find the active workspace for display
  const activeWorkspace = workspaces.find((w) => w.is_active);

  return (
    <div className="w-72 bg-zinc-950 border-r border-zinc-800 flex flex-col h-full">
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

        {/* Active workspace indicator */}
        {activeWorkspace && (
          <div className="mt-2 flex items-center gap-2 px-2 py-1.5 rounded-md bg-zinc-900 border border-zinc-800">
            <div className="w-2 h-2 rounded-full bg-emerald-500 shrink-0" />
            <div className="min-w-0 flex-1">
              <div className="text-xs font-medium text-zinc-300 truncate">
                {activeWorkspace.name}
              </div>
              {activeWorkspace.path && (
                <div className="text-[10px] text-zinc-600 font-mono truncate">
                  {activeWorkspace.path}
                </div>
              )}
            </div>
            <div className="flex gap-0.5">
              {activeWorkspace.sections.slice(0, 3).map((sec) => (
                <div
                  key={sec.id}
                  className="w-2 h-2 rounded-full"
                  style={{ backgroundColor: sec.color || "#3b82f6" }}
                  title={sec.name}
                />
              ))}
              {activeWorkspace.sections.length > 3 && (
                <span className="text-[9px] text-zinc-600">
                  +{activeWorkspace.sections.length - 3}
                </span>
              )}
            </div>
          </div>
        )}
      </div>

      {/* Tab bar */}
      <div className="flex border-b border-zinc-800">
        <button
          onClick={() => setActiveTab("sessions")}
          className={`flex-1 px-3 py-2 text-xs font-medium transition-colors ${
            activeTab === "sessions"
              ? "text-zinc-200 border-b-2 border-emerald-500"
              : "text-zinc-500 hover:text-zinc-300"
          }`}
        >
          Sessions
        </button>
        <button
          onClick={() => setActiveTab("workspaces")}
          className={`flex-1 px-3 py-2 text-xs font-medium transition-colors relative ${
            activeTab === "workspaces"
              ? "text-zinc-200 border-b-2 border-emerald-500"
              : "text-zinc-500 hover:text-zinc-300"
          }`}
        >
          Workspaces
          {workspaces.length > 0 && (
            <span className="ml-1 text-[10px] bg-zinc-800 text-zinc-400 px-1.5 py-0.5 rounded-full">
              {workspaces.length}
            </span>
          )}
        </button>
      </div>

      {/* Tab content */}
      {activeTab === "sessions" ? (
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
      ) : (
        <WorkspacePanel
          workspaces={workspaces}
          onActivate={onActivateWorkspace}
          onCreate={onCreateWorkspace}
          onDelete={onDeleteWorkspace}
          onCreateSection={onCreateSection}
          onDeleteSection={onDeleteSection}
        />
      )}

      {/* Footer */}
      <div className="p-3 border-t border-zinc-800 text-xs text-zinc-600">
        Powered by glaw-code
      </div>
    </div>
  );
}
