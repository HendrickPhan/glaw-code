export type CommandCategory = "core" | "workspace" | "session" | "git" | "automation";

export interface CommandSpec {
  cmd: string;
  desc: string;
  category: CommandCategory;
  argumentHint?: string;
  aliases?: string[];
}

export const COMMANDS: CommandSpec[] = [
  // Core commands
  { cmd: "/help", desc: "Show available commands", category: "core", aliases: ["h", "?"] },
  { cmd: "/status", desc: "Show runtime status", category: "core", aliases: ["st"] },
  { cmd: "/compact", desc: "Compact conversation history", category: "core" },
  { cmd: "/model", desc: "Show or change model", category: "core", argumentHint: "[model]" },
  { cmd: "/permissions", desc: "Show or change permission mode", category: "core", argumentHint: "[mode]", aliases: ["perm"] },
  { cmd: "/clear", desc: "Clear conversation", category: "core" },
  { cmd: "/cost", desc: "Show cost summary", category: "core" },
  { cmd: "/config", desc: "Read/write configuration", category: "core", argumentHint: "[key] [value]" },
  { cmd: "/memory", desc: "Manage memory/context", category: "core", argumentHint: "[add|read|delete] [name] [content]" },
  { cmd: "/version", desc: "Show version", category: "core", aliases: ["v"] },
  { cmd: "/teleport", desc: "Remote session teleport", category: "core", argumentHint: "<host:port>" },
  { cmd: "/debug-tool-call", desc: "Debug tool call tracing", category: "core", argumentHint: "[on|off]" },
  { cmd: "/plugin", desc: "Plugin management", category: "core", argumentHint: "[install|enable|disable|list]" },
  { cmd: "/agents", desc: "Agent management", category: "core", argumentHint: "[list|show|create|call|edit|delete|jobs]" },
  { cmd: "/skills", desc: "List available skills", category: "core" },
  { cmd: "/btw", desc: "Ask a side question during execution", category: "core", argumentHint: "<question>" },
  { cmd: "/tasks", desc: "Manage agent tasks", category: "core", argumentHint: "[create|list|update|delete]" },
  { cmd: "/yolo", desc: "Toggle yolo mode (auto-approve all tool calls)", category: "core" },
  // Workspace commands
  { cmd: "/init", desc: "Initialize .glaw directory", category: "workspace" },
  { cmd: "/diff", desc: "Show pending changes", category: "workspace" },
  { cmd: "/revert", desc: "Revert file changes from the last turn", category: "workspace", argumentHint: "[all]", aliases: ["undo"] },
  { cmd: "/analyze", desc: "Analyze project source code", category: "workspace", argumentHint: "[full|summary|graph]" },
  // Session commands
  { cmd: "/resume", desc: "Resume previous session", category: "session", argumentHint: "<session-id>" },
  { cmd: "/export", desc: "Export session data", category: "session", argumentHint: "[filename]" },
  { cmd: "/session", desc: "Session management", category: "session", argumentHint: "[list|load|new|delete|resume]" },
  // Git commands
  { cmd: "/branch", desc: "Git branch operations", category: "git", argumentHint: "[create|switch|list|delete]" },
  { cmd: "/worktree", desc: "Git worktree operations", category: "git", argumentHint: "[create|remove|list]" },
  { cmd: "/commit", desc: "Create a git commit", category: "git", argumentHint: "[message]" },
  { cmd: "/commit-push-pr", desc: "Commit, push, and create PR", category: "git", argumentHint: "[message]", aliases: ["cpp"] },
  { cmd: "/pr", desc: "Pull request operations", category: "git", argumentHint: "[create|list|view|checkout]" },
  { cmd: "/issue", desc: "GitHub issue operations", category: "git", argumentHint: "[create|list|view]" },
  // Automation commands
  { cmd: "/bughunter", desc: "Bug hunting mode", category: "automation", argumentHint: "[target]" },
  { cmd: "/ultraplan", desc: "Detailed planning mode", category: "automation", argumentHint: "[task description]" },
];

export function findCommands(query: string): CommandSpec[] {
  const filter = query.toLowerCase().replace(/^\//, "");
  if (!filter) return COMMANDS;

  return COMMANDS.filter((cmd) => {
    const name = cmd.cmd.toLowerCase().replace(/^\//, "");
    return name.startsWith(filter);
  });
}

export function getCommand(cmdName: string): CommandSpec | undefined {
  const normalizedName = cmdName.toLowerCase().replace(/^\//, "");
  return COMMANDS.find((cmd) => {
    const name = cmd.cmd.toLowerCase().replace(/^\//, "");
    return name === normalizedName || cmd.aliases?.includes(normalizedName);
  });
}

export function formatCommandHelp(spec: CommandSpec): string {
  const aliases = spec.aliases ? ` (${spec.aliases.join(", ")})` : "";
  const hint = spec.argumentHint ? ` ${spec.argumentHint}` : "";
  return `${spec.cmd}${hint}${aliases} - ${spec.desc}`;
}
