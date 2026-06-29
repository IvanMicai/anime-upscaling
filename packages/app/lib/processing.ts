import type { Job } from "./types";
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

/** Friendly label per operation, shown next to the percentage. */
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
  column: FolderKey | null;
  percent: number | null;
  status: "running" | "queued";
  label: string;
}

/** Step prefix emitted by custom pipelines, e.g. "[2/3] Upscale 2x: ep01.mkv". */
const RE_STEP = /^\[(\d+)\/\d+\]/;

/**
 * Resolve the operation a job is currently performing. Simple job types are the
 * operation itself. For custom pipelines, parse the `[n/N]` prefix from the
 * latest progress message and read the matching step. This is a best-effort
 * guess: `progress.current` is job-level, so with several files in flight it may
 * reflect a different file's step.
 */
function jobOperation(job: Job): string | null {
  if (job.type !== "custom_pipeline") return job.type;
  const m = RE_STEP.exec(job.progress?.current ?? "");
  if (!m) return null;
  const idx = parseInt(m[1], 10) - 1;
  return job.pipeline_steps?.[idx]?.operation ?? null;
}

/**
 * Build a map from a file's normalized relative path to its current processing
 * state, derived from the live jobs list. Running entries carry a percentage;
 * queued entries (files in a job that haven't started a worker yet) carry none.
 * Running always wins over queued for the same file.
 */
export function buildProcessingMap(jobs: Job[]): Map<string, FileProcessing> {
  const map = new Map<string, FileProcessing>();

  for (const job of jobs) {
    if (job.status !== "running" && job.status !== "queued") continue;

    const op = jobOperation(job);
    const column = op ? (OP_TO_COLUMN[op] ?? null) : null;
    const label = (op && OP_LABEL[op]) || "Processing";

    const containers = job.progress?.containers ?? {};
    const active = new Set<string>();

    // Running: one entry per in-flight worker.
    for (const c of Object.values(containers)) {
      if (!c?.filename) continue;
      const key = normalizeRelPath(c.filename);
      active.add(key);
      const percent =
        c.percent ??
        (c.total_frames && c.total_frames > 0
          ? (c.frame / c.total_frames) * 100
          : null);
      map.set(key, { column, percent, status: "running", label });
    }

    // Queued: files in this job that aren't yet in a worker.
    for (const f of job.files ?? []) {
      const key = normalizeRelPath(f);
      if (active.has(key)) continue;
      const existing = map.get(key);
      if (existing?.status === "running") continue;
      map.set(key, { column, percent: null, status: "queued", label });
    }
  }

  return map;
}
