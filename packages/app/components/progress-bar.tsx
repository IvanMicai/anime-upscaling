import type { JobProgress } from "@/lib/types";

export function ProgressBar({ progress }: { progress: JobProgress }) {
  const { total, completed, failed, skipped } = progress;
  if (total === 0) return null;

  const pct = (n: number) => `${((n / total) * 100).toFixed(1)}%`;
  const done = completed + failed + skipped;

  return (
    <div className="space-y-2 rounded-lg border bg-card/50 p-4">
      <div className="flex items-center justify-between">
        <span className="text-sm font-medium">Overall progress</span>
        <span className="font-mono text-xs text-muted-foreground tabular-nums">
          {done}/{total} &mdash;{" "}
          <span className="text-green-400">{completed} ok</span>
          {" · "}
          <span className={failed > 0 ? "text-red-400" : ""}>{failed} err</span>
          {" · "}
          <span className={skipped > 0 ? "text-yellow-400" : ""}>
            {skipped} skip
          </span>
        </span>
      </div>
      <div className="flex h-2.5 w-full overflow-hidden rounded-full bg-muted">
        {completed > 0 && (
          <div className="bg-green-500 transition-all" style={{ width: pct(completed) }} />
        )}
        {failed > 0 && (
          <div className="bg-red-500 transition-all" style={{ width: pct(failed) }} />
        )}
        {skipped > 0 && (
          <div className="bg-yellow-500 transition-all" style={{ width: pct(skipped) }} />
        )}
      </div>
    </div>
  );
}
