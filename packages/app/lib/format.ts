import type { Job, JobProgress } from "./types";

/** Localized timestamp for created/finished fields. */
export function formatTime(iso: string): string {
  return new Date(iso).toLocaleString();
}

/** Compact relative timestamp, e.g. "just now", "2m ago", "1h ago", "3d ago". */
export function formatRelativeTime(iso: string): string {
  const diff = Math.floor((Date.now() - new Date(iso).getTime()) / 1000);
  if (diff < 10) return "just now";
  if (diff < 60) return `${diff}s ago`;
  const m = Math.floor(diff / 60);
  if (m < 60) return `${m}m ago`;
  const h = Math.floor(m / 60);
  if (h < 24) return `${h}h ago`;
  const d = Math.floor(h / 24);
  return `${d}d ago`;
}

/** Label for a job's "Type" column: the pipeline name for custom pipelines. */
export function jobTypeLabel(job: Job): string {
  return job.type === "custom_pipeline" && job.pipeline_name
    ? job.pipeline_name
    : job.type;
}

/** Completion percentage (completed+failed+skipped over total). */
export function jobPercent(p: JobProgress): number {
  if (p.total <= 0) return 0;
  const done = p.completed + p.failed + p.skipped;
  return Math.round((done / p.total) * 100);
}

/** Progress summary, e.g. "75% · 9/12" or "8 ok · 1 err" when there are errors. */
export function jobProgressLabel(p: JobProgress): string {
  if (p.total <= 0) return "—";
  const done = p.completed + p.failed + p.skipped;
  if (p.failed > 0) return `${p.completed} ok · ${p.failed} err`;
  return `${jobPercent(p)}% · ${done}/${p.total}`;
}

/**
 * Format a remaining-seconds value as a compact ETA, e.g. "~28m", "~1h 04m",
 * "~45s". Used by per-worker gauges where seconds are known directly.
 */
export function formatEtaSeconds(seconds: number): string {
  if (!Number.isFinite(seconds) || seconds <= 0) return "—";
  const h = Math.floor(seconds / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  const s = Math.floor(seconds % 60);
  if (h > 0) return `~${h}h ${String(m).padStart(2, "0")}m`;
  if (m > 0) return `~${m}m ${s}s`;
  return `~${s}s`;
}

/**
 * Estimate remaining time for a job from how much has completed so far.
 * Extracted from job-list so the dashboard table and mobile cards share it.
 */
export function formatEta(job: Job): string {
  if (job.status !== "running") return "—";
  const p = job.progress;
  const processed = p.completed + p.failed;
  const remaining = p.total - p.skipped - processed;
  if (processed <= 0 || remaining <= 0) return "—";
  const elapsedMs = Date.now() - new Date(job.created_at).getTime();
  if (elapsedMs <= 0) return "—";
  const seconds = Math.round((remaining * elapsedMs) / processed / 1000);
  return formatEtaSeconds(seconds);
}

/** Human-readable process uptime from seconds, e.g. "6h 12m", "2d 4h". */
export function formatUptime(seconds: number): string {
  if (!seconds || seconds <= 0) return "—";
  const d = Math.floor(seconds / 86400);
  const h = Math.floor((seconds % 86400) / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  if (d > 0) return `${d}d ${h}h`;
  if (h > 0) return `${h}h ${m}m`;
  return `${m}m`;
}
