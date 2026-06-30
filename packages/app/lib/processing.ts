import type { Job, VideoFile } from "./types";
import type { FolderKey } from "./file-utils";

/**
 * Maps a processing operation to the stage column it produces in the file list.
 * `check`/`cleanup` produce no stage output, so they have no column indicator.
 * The `output` column is labelled "Upscaling" in FOLDER_COLORS.
 */
export const OP_TO_COLUMN: Record<string, FolderKey | null> = {
  upscale: "output",
  interpolate: "interpolated",
  optimize: "optimized",
  check: null,
  cleanup: null,
};

/** Friendly label per operation. */
export const OP_LABEL: Record<string, string> = {
  upscale: "Upscaling",
  interpolate: "Interpolating",
  optimize: "Optimizing",
  check: "Checking",
  cleanup: "Cleaning",
};

/**
 * Normalize a relative path so library rows and job/container filenames compare
 * equal: forward slashes, no leading `./` or `/`. The relative path within a
 * stage tree (e.g. `season1/ep01.mkv`) is identical across stages, so the match
 * is independent of which directory pill is selected.
 */
export function normalizeRelPath(p: string): string {
  return p.replace(/\\/g, "/").replace(/^\.?\//, "");
}

export interface FileProcessing {
  percent: number | null;
  status: "running" | "queued";
  /** Worker label, e.g. "GPU 0", "FFMPEG 1" (running only). */
  source?: string;
  /** FFmpeg sub-phase, e.g. "Encode" | "Remux" | "Check" (running only). */
  phase?: string;
  jobType: string;
  /** Operations a queued file will go through (first one determines its column). */
  queuedOp?: string | null;
  /** Pipeline step operations, used to disambiguate GPU upscale vs interpolate. */
  pipelineOps?: string[];
}

export interface ResolvedProcessing {
  column: FolderKey | null;
  label: string;
  percent: number | null;
  status: "running" | "queued";
}

/**
 * Build a map from a file's normalized relative path to its current processing
 * state, derived from the live jobs list. Running entries carry a percentage and
 * the worker/phase needed to resolve which stage column they belong to; queued
 * entries (files in a job that haven't started a worker yet) carry none. Running
 * always wins over queued for the same file.
 */
export function buildProcessingMap(jobs: Job[]): Map<string, FileProcessing> {
  const map = new Map<string, FileProcessing>();

  for (const job of jobs) {
    if (job.status !== "running" && job.status !== "queued") continue;

    const isPipeline = job.type === "custom_pipeline";
    const pipelineOps = isPipeline
      ? (job.pipeline_steps ?? []).map((s) => s.operation)
      : undefined;
    const queuedOp = isPipeline ? (pipelineOps?.[0] ?? null) : job.type;

    const containers = job.progress?.containers ?? {};
    const active = new Set<string>();

    // Running: one entry per in-flight worker. The container map key is the
    // worker label (e.g. "FFMPEG 1") — keep it to resolve the stage column.
    for (const [source, c] of Object.entries(containers)) {
      if (!c?.filename) continue;
      const key = normalizeRelPath(c.filename);
      active.add(key);
      const percent =
        c.percent ??
        (c.total_frames && c.total_frames > 0
          ? (c.frame / c.total_frames) * 100
          : null);
      map.set(key, {
        percent,
        status: "running",
        source,
        phase: c.phase,
        jobType: job.type,
        pipelineOps,
      });
    }

    // Queued: files in this job that aren't yet in a worker.
    for (const f of job.files ?? []) {
      const key = normalizeRelPath(f);
      if (active.has(key)) continue;
      if (map.get(key)?.status === "running") continue;
      map.set(key, { percent: null, status: "queued", jobType: job.type, queuedOp });
    }
  }

  return map;
}

/**
 * Resolve which operation a file is undergoing. Simple jobs are the operation
 * itself. For custom pipelines the per-job `progress.current` is unreliable
 * (often empty), so infer the running step from the worker + ffmpeg phase, and
 * disambiguate GPU upscale-vs-interpolate using the file's existing stages.
 */
function operationFor(info: FileProcessing, file: VideoFile): string | null {
  if (info.status === "queued") return info.queuedOp ?? null;
  if (info.jobType !== "custom_pipeline") return info.jobType;

  const phase = info.phase;
  if (phase === "Encode" || phase === "Remux") return "optimize";
  if (phase === "Check") return "check";

  const src = info.source ?? "";
  if (src.startsWith("FFMPEG")) return "optimize";
  if (src.startsWith("GPU")) {
    const ops = info.pipelineOps ?? [];
    const hasUpscale = ops.includes("upscale");
    const hasInterpolate = ops.includes("interpolate");
    if (hasUpscale && !hasInterpolate) return "upscale";
    if (hasInterpolate && !hasUpscale) return "interpolate";
    // Both steps exist: a file that already has an upscaled output but no
    // interpolated one is on the interpolate step.
    return file.has_upscaled && !file.has_interpolated ? "interpolate" : "upscale";
  }
  return null;
}

/** Resolve the stage column + label to show for a file's processing state. */
export function resolveProcessing(
  info: FileProcessing,
  file: VideoFile,
): ResolvedProcessing {
  const op = operationFor(info, file);
  return {
    column: op ? (OP_TO_COLUMN[op] ?? null) : null,
    label: (op && OP_LABEL[op]) || "Processing",
    percent: info.percent,
    status: info.status,
  };
}
