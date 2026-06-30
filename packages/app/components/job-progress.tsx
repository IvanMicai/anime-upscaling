import { cn } from "@/lib/utils";
import type { JobProgress } from "@/lib/types";

/**
 * Thin segmented progress bar for a job, used inline in the jobs table rows and
 * mobile cards. Green = completed, red = failed, amber = skipped.
 */
export function JobMiniProgress({
  p,
  className,
}: {
  p: JobProgress;
  className?: string;
}) {
  const total = p.total || 1;
  const ok = (p.completed / total) * 100;
  const err = (p.failed / total) * 100;
  const skip = (p.skipped / total) * 100;
  return (
    <span
      className={cn(
        "flex h-1.5 shrink-0 overflow-hidden rounded-full bg-muted",
        className,
      )}
    >
      <span className="bg-emerald-500" style={{ width: `${ok}%` }} />
      <span className="bg-red-500" style={{ width: `${err}%` }} />
      <span className="bg-amber-500" style={{ width: `${skip}%` }} />
    </span>
  );
}
