"use client";

import { useState } from "react";
import { WorkspaceInfo, SectionInfo } from "@/lib/types";

// --- Predefined section colors ---
const SECTION_COLORS = [
  "#3b82f6", // blue
  "#10b981", // emerald
  "#f59e0b", // amber
  "#ef4444", // red
  "#8b5cf6", // violet
  "#ec4899", // pink
  "#06b6d4", // cyan
  "#f97316", // orange
];

// --- Section Badge ---
function SectionBadge({
  section,
  onDelete,
}: {
  section: SectionInfo;
  onDelete: () => void;
}) {
  return (
    <div
      className="group flex items-center gap-1.5 px-2 py-1 rounded-md text-xs border"
      style={{
        backgroundColor: `${section.color || "#3b82f6"}15`,
        borderColor: `${section.color || "#3b82f6"}40`,
        color: section.color || "#3b82f6",
      }}
    >
      <span className="truncate max-w-[100px]">{section.name}</span>
      <button
        onClick={(e) => {
          e.stopPropagation();
          onDelete();
        }}
        className="opacity-0 group-hover:opacity-100 transition-opacity ml-0.5 hover:text-red-400"
        title="Remove section"
      >
        <svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="3">
          <line x1="18" y1="6" x2="6" y2="18" />
          <line x1="6" y1="6" x2="18" y2="18" />
        </svg>
      </button>
    </div>
  );
}

// --- Workspace Card ---
function WorkspaceCard({
  workspace,
  isActive,
  onSelect,
  onDelete,
  onAddSection,
  onDeleteSection,
}: {
  workspace: WorkspaceInfo;
  isActive: boolean;
  onSelect: () => void;
  onDelete: () => void;
  onAddSection: () => void;
  onDeleteSection: (sectionId: string) => void;
}) {
  const [expanded, setExpanded] = useState(isActive);

  return (
    <div
      className={`rounded-lg border transition-colors ${
        isActive
          ? "border-emerald-700/50 bg-emerald-950/20"
          : "border-zinc-800 bg-zinc-900/50 hover:bg-zinc-800/50"
      }`}
    >
      {/* Workspace header */}
      <button
        onClick={onSelect}
        className="w-full text-left px-3 py-2.5 flex items-center gap-2"
      >
        <div
          className={`w-2 h-2 rounded-full shrink-0 ${
            isActive ? "bg-emerald-500" : "bg-zinc-600"
          }`}
        />
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2">
            <span className="text-sm font-medium text-zinc-200 truncate">
              {workspace.name}
            </span>
            {isActive && (
              <span className="text-[10px] px-1.5 py-0.5 rounded bg-emerald-900/50 text-emerald-400 border border-emerald-800/50">
                active
              </span>
            )}
          </div>
          {workspace.path && (
            <div className="text-xs text-zinc-600 font-mono truncate mt-0.5">
              {workspace.path}
            </div>
          )}
        </div>
        <div className="flex items-center gap-1 shrink-0">
          {/* Toggle sections */}
          <button
            onClick={(e) => {
              e.stopPropagation();
              setExpanded(!expanded);
            }}
            className="text-zinc-600 hover:text-zinc-400 p-0.5 rounded transition-colors"
            title="Toggle sections"
          >
            <svg
              width="14"
              height="14"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              strokeWidth="2"
              className={`transition-transform ${expanded ? "rotate-180" : ""}`}
            >
              <polyline points="6 9 12 15 18 9" />
            </svg>
          </button>
          {/* Delete */}
          <button
            onClick={(e) => {
              e.stopPropagation();
              onDelete();
            }}
            className="text-zinc-600 hover:text-red-400 p-0.5 rounded transition-colors"
            title="Delete workspace"
          >
            <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
              <polyline points="3 6 5 6 21 6" />
              <path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2" />
            </svg>
          </button>
        </div>
      </button>

      {/* Sections */}
      {expanded && (
        <div className="px-3 pb-2.5 border-t border-zinc-800/50 pt-2">
          {/* Section badges */}
          <div className="flex flex-wrap gap-1.5 mb-2">
            {workspace.sections && workspace.sections.length > 0 ? (
              workspace.sections.map((sec) => (
                <SectionBadge
                  key={sec.id}
                  section={sec}
                  onDelete={() => onDeleteSection(sec.id)}
                />
              ))
            ) : (
              <span className="text-xs text-zinc-600">No sections</span>
            )}
          </div>
          {/* Add section button */}
          <button
            onClick={(e) => {
              e.stopPropagation();
              onAddSection();
            }}
            className="text-xs text-zinc-600 hover:text-emerald-400 flex items-center gap-1 transition-colors"
          >
            <svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="3">
              <line x1="12" y1="5" x2="12" y2="19" />
              <line x1="5" y1="12" x2="19" y2="12" />
            </svg>
            Add section
          </button>
        </div>
      )}
    </div>
  );
}

// --- Add Section Form ---
function AddSectionForm({
  onSubmit,
  onCancel,
}: {
  onSubmit: (name: string, description: string, color: string) => void;
  onCancel: () => void;
}) {
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [color, setColor] = useState(SECTION_COLORS[0]);

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (!name.trim()) return;
    onSubmit(name.trim(), description.trim(), color);
  };

  return (
    <form onSubmit={handleSubmit} className="space-y-2">
      <input
        type="text"
        value={name}
        onChange={(e) => setName(e.target.value)}
        placeholder="Section name"
        className="w-full bg-zinc-900 border border-zinc-700 rounded-md px-2.5 py-1.5 text-xs text-zinc-200 placeholder-zinc-600 focus:outline-none focus:border-zinc-500"
        autoFocus
      />
      <input
        type="text"
        value={description}
        onChange={(e) => setDescription(e.target.value)}
        placeholder="Description (optional)"
        className="w-full bg-zinc-900 border border-zinc-700 rounded-md px-2.5 py-1.5 text-xs text-zinc-200 placeholder-zinc-600 focus:outline-none focus:border-zinc-500"
      />
      {/* Color picker */}
      <div className="flex items-center gap-1.5">
        <span className="text-xs text-zinc-500">Color:</span>
        {SECTION_COLORS.map((c) => (
          <button
            key={c}
            type="button"
            onClick={() => setColor(c)}
            className={`w-4 h-4 rounded-full border-2 transition-transform ${
              color === c ? "scale-125 border-white" : "border-transparent"
            }`}
            style={{ backgroundColor: c }}
          />
        ))}
      </div>
      <div className="flex gap-2">
        <button
          type="submit"
          disabled={!name.trim()}
          className="text-xs bg-emerald-700 hover:bg-emerald-600 disabled:bg-zinc-700 disabled:opacity-50 text-white px-3 py-1 rounded-md transition-colors"
        >
          Add
        </button>
        <button
          type="button"
          onClick={onCancel}
          className="text-xs text-zinc-500 hover:text-zinc-300 px-3 py-1 transition-colors"
        >
          Cancel
        </button>
      </div>
    </form>
  );
}

// --- Create Workspace Form ---
function CreateWorkspaceForm({
  onSubmit,
  onCancel,
}: {
  onSubmit: (name: string, path: string, description: string) => void;
  onCancel: () => void;
}) {
  const [name, setName] = useState("");
  const [path, setPath] = useState("");
  const [description, setDescription] = useState("");

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (!name.trim()) return;
    onSubmit(name.trim(), path.trim(), description.trim());
  };

  return (
    <form onSubmit={handleSubmit} className="space-y-2 p-3 bg-zinc-900/80 border border-zinc-700 rounded-lg">
      <div className="text-xs font-medium text-zinc-300 mb-1">Create Workspace</div>
      <input
        type="text"
        value={name}
        onChange={(e) => setName(e.target.value)}
        placeholder="Workspace name *"
        className="w-full bg-zinc-950 border border-zinc-700 rounded-md px-2.5 py-1.5 text-xs text-zinc-200 placeholder-zinc-600 focus:outline-none focus:border-zinc-500"
        autoFocus
      />
      <input
        type="text"
        value={path}
        onChange={(e) => setPath(e.target.value)}
        placeholder="Working directory path (e.g., /home/user/project)"
        className="w-full bg-zinc-950 border border-zinc-700 rounded-md px-2.5 py-1.5 text-xs text-zinc-200 placeholder-zinc-600 focus:outline-none focus:border-zinc-500 font-mono"
      />
      <textarea
        value={description}
        onChange={(e) => setDescription(e.target.value)}
        placeholder="Description (optional)"
        className="w-full bg-zinc-950 border border-zinc-700 rounded-md px-2.5 py-1.5 text-xs text-zinc-200 placeholder-zinc-600 focus:outline-none focus:border-zinc-500 resize-none"
        rows={2}
      />
      <div className="flex gap-2">
        <button
          type="submit"
          disabled={!name.trim()}
          className="text-xs bg-emerald-700 hover:bg-emerald-600 disabled:bg-zinc-700 disabled:opacity-50 text-white px-3 py-1.5 rounded-md transition-colors"
        >
          Create
        </button>
        <button
          type="button"
          onClick={onCancel}
          className="text-xs text-zinc-500 hover:text-zinc-300 px-3 py-1.5 transition-colors"
        >
          Cancel
        </button>
      </div>
    </form>
  );
}

// --- Main Workspace Panel ---
export default function WorkspacePanel({
  workspaces,
  onActivate,
  onCreate,
  onDelete,
  onCreateSection,
  onDeleteSection,
}: {
  workspaces: WorkspaceInfo[];
  onActivate: (id: string) => void;
  onCreate: (name: string, path: string, description: string) => void;
  onDelete: (id: string) => void;
  onCreateSection: (workspaceId: string, name: string, description: string, color: string) => void;
  onDeleteSection: (workspaceId: string, sectionId: string) => void;
}) {
  const [showCreate, setShowCreate] = useState(false);
  const [addingSectionTo, setAddingSectionTo] = useState<string | null>(null);

  return (
    <div className="flex flex-col h-full">
      {/* Header */}
      <div className="p-3 border-b border-zinc-800">
        <div className="flex items-center justify-between">
          <h2 className="text-xs font-semibold text-zinc-400 uppercase tracking-wider">
            Workspaces
          </h2>
          <button
            onClick={() => setShowCreate(!showCreate)}
            className="text-zinc-500 hover:text-emerald-400 p-1 rounded hover:bg-zinc-800 transition-colors"
            title="Create workspace"
          >
            <svg
              width="14"
              height="14"
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

      {/* Create form */}
      {showCreate && (
        <div className="p-3 border-b border-zinc-800">
          <CreateWorkspaceForm
            onSubmit={(name, path, description) => {
              onCreate(name, path, description);
              setShowCreate(false);
            }}
            onCancel={() => setShowCreate(false)}
          />
        </div>
      )}

      {/* Add section form */}
      {addingSectionTo && (
        <div className="p-3 border-b border-zinc-800">
          <AddSectionForm
            onSubmit={(name, description, color) => {
              onCreateSection(addingSectionTo, name, description, color);
              setAddingSectionTo(null);
            }}
            onCancel={() => setAddingSectionTo(null)}
          />
        </div>
      )}

      {/* Workspace list */}
      <div className="flex-1 overflow-y-auto p-3 space-y-2">
        {workspaces.length === 0 && !showCreate ? (
          <div className="text-center py-8">
            <div className="text-2xl mb-2">📁</div>
            <div className="text-xs text-zinc-600 leading-relaxed">
              No workspaces yet.
              <br />
              Create one to organize your projects.
            </div>
            <button
              onClick={() => setShowCreate(true)}
              className="mt-3 text-xs text-emerald-500 hover:text-emerald-400 transition-colors"
            >
              + Create workspace
            </button>
          </div>
        ) : (
          workspaces.map((ws) => (
            <WorkspaceCard
              key={ws.id}
              workspace={ws}
              isActive={ws.is_active}
              onSelect={() => onActivate(ws.id)}
              onDelete={() => onDelete(ws.id)}
              onAddSection={() => setAddingSectionTo(ws.id)}
              onDeleteSection={(sectionId) => onDeleteSection(ws.id, sectionId)}
            />
          ))
        )}
      </div>

      {/* Footer */}
      <div className="p-3 border-t border-zinc-800 text-xs text-zinc-600">
        {workspaces.length} workspace{workspaces.length !== 1 ? "s" : ""}
      </div>
    </div>
  );
}
