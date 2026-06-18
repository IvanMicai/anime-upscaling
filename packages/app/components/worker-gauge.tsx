import { Cpu } from "lucide-react";
import { sourceColorSet } from "@/lib/source-color";
import { formatEtaSeconds } from "@/lib/format";
import { cn } from "@/lib/utils";
import type { ContainerProgress } from "@/lib/types";

// Solid fill color per worker, mirroring the source-color families (GPU 0 blue,
// GPU 1 purple, …) so a gauge matches its log badge.
function barColor(source: string): string {
  const g = source.match(/^GPU (\d+)/);
  if (g) {
    const colors = ["bg-blue-500", "bg-purple-500", "bg-emerald-500", "bg-amber-500"];
    return colors[parseInt(g[1], 10) % colors.length];
  }
  if (source.startsWith("FFMPEG")) return "bg-cyan-500";
  return "bg-primary";
}

/**
 * Live per-worker gauge card for the job detail page: filename, a colored
 * progress bar and frame/fps/elapsed/ETA. One card per active GPU/FFmpeg stream.
 */
export function WorkerGauge({
  source,
  c,
}: {
  source: string;
  c: ContainerProgress;
}) {
  const pct =
    c.total_frames && c.total_frames > 0
      ? (c.frame / c.total_frames) * 100
      : (c.percent ?? 0);
  const eta =
    c.total_frames && c.fps > 0 && c.frame < c.total_frames
      ? formatEtaSeconds(Math.round((c.total_frames - c.frame) / c.fps))
      : null;

  return (
    <div className="rounded-lg border bg-card/50 p-3">
      <div className="flex items-center justify-between gap-2">
        <span
          className={cn(
            "flex items-center gap-1.5 font-mono text-sm font-medium",
            sourceColorSet(source).text,
          )}
        >
          <Cpu className="size-3.5" />
          {source}
        </span>
        {c.phase && (
          <span className="text-xs text-muted-foreground">{c.phase}</span>
        )}
      </div>

      {c.filename && (
        <div className="mt-1 truncate font-mono text-sm">{c.filename}</div>
      )}

      <div className="mt-2 h-1.5 w-full overflow-hidden rounded-full bg-muted">
        <div
          className={cn("h-full rounded-full transition-all", barColor(source))}
          style={{ width: `${Math.min(100, Math.max(0, pct))}%` }}
        />
      </div>

      <div className="mt-2 flex flex-wrap items-center gap-x-3 gap-y-0.5 font-mono text-xs tabular-nums text-muted-foreground">
        {c.total_frames ? (
          <span>
            {c.frame} / {c.total_frames} ({(c.percent ?? pct).toFixed(1)}%)
          </span>
        ) : c.frame > 0 ? (
          <span>frame {c.frame}</span>
        ) : null}
        {c.fps > 0 && <span>{c.fps} fps</span>}
        {c.elapsed && <span>elapsed {c.elapsed}</span>}
        {eta && <span className="text-emerald-400">ETA {eta}</span>}
      </div>
    </div>
  );
}
