import type { JobProgress } from "@/lib/types";

const sourceColors: Record<string, string> = {
  "GPU 0": "text-blue-400",
  "GPU 1": "text-purple-400",
  FFMPEG: "text-cyan-400",
};

export function ProgressBar({ progress }: { progress: JobProgress }) {
  const { total, completed, failed, skipped, current, containers } = progress;
  if (total === 0) return null;

  const pct = (n: number) => `${((n / total) * 100).toFixed(1)}%`;
  const done = completed + failed + skipped;

  const entries = containers
    ? Object.entries(containers).filter(([, c]) => c && c.frame > 0)
    : [];

  return (
    <div className="space-y-1.5">
      <div className="flex h-2.5 w-full overflow-hidden rounded-full bg-muted">
        {completed > 0 && (
          <div
            className="bg-green-500 transition-all"
            style={{ width: pct(completed) }}
          />
        )}
        {failed > 0 && (
          <div
            className="bg-red-500 transition-all"
            style={{ width: pct(failed) }}
          />
        )}
        {skipped > 0 && (
          <div
            className="bg-yellow-500 transition-all"
            style={{ width: pct(skipped) }}
          />
        )}
      </div>
      <div className="flex items-center justify-between text-xs text-muted-foreground">
        <span>
          {done}/{total} &mdash;{" "}
          <span className="text-green-400">{completed} ok</span>
          {failed > 0 && (
            <span className="text-red-400"> / {failed} err</span>
          )}
          {skipped > 0 && (
            <span className="text-yellow-400"> / {skipped} skip</span>
          )}
        </span>
        {current && (
          <span className="max-w-[50%] truncate font-mono">{current}</span>
        )}
      </div>
      {entries.map(([source, c]) => (
        <div
          key={source}
          className="flex items-center gap-3 font-mono text-xs text-muted-foreground"
        >
          <span className={sourceColors[source] ?? "text-muted-foreground"}>
            {source}
          </span>
          <span>
            Frame: {c.frame}
            {c.total_frames
              ? `/${c.total_frames} (${c.percent?.toFixed(1)}%)`
              : ""}
          </span>
          {c.fps > 0 && <span>FPS: {c.fps}</span>}
          {c.elapsed && <span>Elapsed: {c.elapsed}</span>}
          {c.speed && <span>Speed: {c.speed}</span>}
        </div>
      ))}
    </div>
  );
}
