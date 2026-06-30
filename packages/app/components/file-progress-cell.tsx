import { cn } from "@/lib/utils";
import { FOLDER_COLORS, type FolderKey } from "@/lib/file-utils";
import type { ResolvedProcessing } from "@/lib/processing";

// Solid fill colors for the determinate bar, matching each stage's accent.
const BAR_FILL: Record<FolderKey, string> = {
  input: "bg-yellow-500",
  output: "bg-blue-500",
  optimized: "bg-green-500",
  interpolated: "bg-purple-500",
};

/**
 * In-cell processing indicator for the file list's stage columns. Running shows
 * a pulsing dot + percent + a thin determinate bar in the stage color; queued
 * shows a discreet "Na fila" pill (purple, matching the queued StatusBadge).
 */
export function FileProgressCell({ info }: { info: ResolvedProcessing }) {
  if (info.status === "queued") {
    return (
      <span className="inline-flex items-center rounded-full border border-purple-500/30 bg-purple-500/20 px-1.5 py-0.5 text-[10px] font-medium text-purple-400">
        Na fila
      </span>
    );
  }

  const key = info.column;
  const pct = info.percent != null ? Math.round(info.percent) : null;
  const textColor = key ? FOLDER_COLORS[key].text : "text-blue-400";
  const fill = key ? BAR_FILL[key] : "bg-blue-500";

  return (
    <span className="inline-flex flex-col items-end gap-1 align-middle">
      <span
        className={cn(
          "inline-flex items-center gap-1 text-xs font-medium tabular-nums",
          textColor,
        )}
      >
        <span className="h-1.5 w-1.5 rounded-full bg-current animate-pulse" />
        {pct != null ? `${pct}%` : "…"}
      </span>
      <span className="flex h-1 w-16 overflow-hidden rounded-full bg-muted">
        <span
          className={cn("h-full transition-[width] duration-500", fill)}
          style={{ width: `${pct ?? 0}%` }}
        />
      </span>
    </span>
  );
}
