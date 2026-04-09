"use client";

import { CheckCircle2, XCircle, AlertCircle, Info, FileText, GitBranch, Package, Settings, User } from "lucide-react";

interface CommandResultProps {
  command: string;
  message: string;
  type?: "success" | "error" | "warning" | "info";
}

export default function CommandResult({ command, message, type = "info" }: CommandResultProps) {
  // Determine icon and color based on type or content
  let Icon = Info;
  let colorClass = "text-blue-400";
  let bgClass = "bg-blue-900/20";
  let borderClass = "border-blue-800/50";

  if (type === "error" || message.toLowerCase().includes("error") || message.toLowerCase().includes("failed")) {
    Icon = XCircle;
    colorClass = "text-red-400";
    bgClass = "bg-red-900/20";
    borderClass = "border-red-800/50";
  } else if (type === "warning" || message.toLowerCase().includes("warning")) {
    Icon = AlertCircle;
    colorClass = "text-yellow-400";
    bgClass = "bg-yellow-900/20";
    borderClass = "border-yellow-800/50";
  } else if (type === "success" || message.toLowerCase().includes("created") || message.toLowerCase().includes("completed") || message.toLowerCase().includes("success")) {
    Icon = CheckCircle2;
    colorClass = "text-emerald-400";
    bgClass = "bg-emerald-900/20";
    borderClass = "border-emerald-800/50";
  }

  // Detect content type for better icon selection
  if (command.includes("help") || command.includes("version") || command.includes("status")) {
    Icon = Info;
    colorClass = "text-blue-400";
    bgClass = "bg-blue-900/20";
    borderClass = "border-blue-800/50";
  } else if (command.includes("config") || command.includes("memory") || command.includes("settings")) {
    Icon = Settings;
    colorClass = "text-purple-400";
    bgClass = "bg-purple-900/20";
    borderClass = "border-purple-800/50";
  } else if (command.includes("git") || command.includes("branch") || command.includes("commit") || command.includes("pr") || command.includes("issue")) {
    Icon = GitBranch;
    colorClass = "text-orange-400";
    bgClass = "bg-orange-900/20";
    borderClass = "border-orange-800/50";
  } else if (command.includes("agents") || command.includes("skills")) {
    Icon = User;
    colorClass = "text-cyan-400";
    bgClass = "bg-cyan-900/20";
    borderClass = "border-cyan-800/50";
  } else if (command.includes("export") || command.includes("analyze")) {
    Icon = FileText;
    colorClass = "text-indigo-400";
    bgClass = "bg-indigo-900/20";
    borderClass = "border-indigo-800/50";
  }

  // Format the message for better display
  const formattedMessage = message
    .split('\n')
    .map((line, idx) => {
      // Highlight command names
      if (line.match(/^\s*\/[\w\-]+/)) {
        return (
          <span key={idx} className="text-emerald-400 font-mono">
            {line}
          </span>
        );
      }
      // Highlight section headers
      if (line.match(/^[A-Z]+:/) || line.match(/^\s*[A-Z][a-z]+:/)) {
        return (
          <span key={idx} className="text-zinc-300 font-semibold">
            {line}
          </span>
        );
      }
      // Highlight JSON keys
      if (line.match(/^\s*"[^"]+":\s*/)) {
        return (
          <span key={idx} className="text-yellow-300 font-mono">
            {line}
          </span>
        );
      }
      return line;
    })
    .reduce((acc, line, idx) => {
      if (idx > 0) acc.push(<br key={`br-${idx}`} />);
      acc.push(line);
      return acc;
    }, [] as React.ReactNode[]);

  return (
    <div className={`my-2 mx-4 p-3 rounded-lg border ${bgClass} ${borderClass}`}>
      <div className="flex items-start gap-2">
        <Icon className={`w-5 h-5 mt-0.5 shrink-0 ${colorClass}`} />
        <div className="flex-1 min-w-0">
          <div className={`text-xs font-mono mb-1 ${colorClass}`}>
            /{command}
          </div>
          <div className="text-sm text-zinc-300 whitespace-pre-wrap break-words leading-relaxed">
            {formattedMessage}
          </div>
        </div>
      </div>
    </div>
  );
}
