export interface WSMessage {
  type: string;
  data?: unknown;
  session_id?: string;
}

export interface ChatMessage {
  id: string;
  role: "user" | "assistant" | "tool";
  content: string;
  timestamp: number;
  toolUse?: ToolUseInfo;
  toolResult?: ToolResultInfo;
}

export interface ToolUseInfo {
  id: string;
  name: string;
  input: string;
}

export interface ToolResultInfo {
  id: string;
  content: string;
  isError: boolean;
  elapsed?: number;
}

export interface SessionInfo {
  id: string;
  created_at: string;
  message_count: number;
}

export interface UsageInfo {
  inputTokens: number;
  outputTokens: number;
  totalCostUSD: number;
}

// eslint-disable-next-line @typescript-eslint/no-explicit-any
export type WSIncoming = Record<string, any>;
