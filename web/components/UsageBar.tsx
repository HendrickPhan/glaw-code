"use client";

import { UsageInfo } from "@/lib/types";

export default function UsageBar({ usage }: { usage: UsageInfo | null }) {
  if (!usage) return null;

  return (
    <div className="flex items-center gap-3 text-xs text-zinc-500 px-4 py-1.5 border-t border-zinc-800/50 bg-zinc-950/50">
      <span>
        Tokens: {usage.inputTokens.toLocaleString()} in +{" "}
        {usage.outputTokens.toLocaleString()} out
      </span>
      <span className="text-zinc-700">│</span>
      <span>Cost: ${usage.totalCostUSD.toFixed(4)}</span>
    </div>
  );
}
