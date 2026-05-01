import type { Job, JobProgress, LogEntry } from "@/lib/types";

const minutesAgo = (n: number) =>
  new Date(Date.now() - n * 60 * 1000).toISOString();

export const queuedProgress: JobProgress = {
  total: 5,
  completed: 0,
  failed: 0,
  skipped: 0,
  current: "",
  containers: null,
};

export const runningProgress: JobProgress = {
  total: 5,
  completed: 2,
  failed: 0,
  skipped: 0,
  current: "ep03.mkv",
  containers: {
    "GPU 0": {
      frame: 1240,
      fps: 18.4,
      total_frames: 34_280,
      elapsed: "00:01:07",
      speed: "0.76x",
      percent: 3.6,
      filename: "ep03.mkv",
      phase: "upscale",
    },
    "FFMPEG": {
      frame: 0,
      fps: 0,
      phase: "encoding",
      elapsed: "00:00:08",
    },
  },
};

export const completedProgress: JobProgress = {
  total: 5,
  completed: 4,
  failed: 0,
  skipped: 1,
  current: "",
  containers: null,
};

export const failedProgress: JobProgress = {
  total: 5,
  completed: 2,
  failed: 1,
  skipped: 0,
  current: "ep03.mkv",
  containers: null,
};

export const queuedJob: Job = {
  id: "job_q9k2lm",
  type: "upscale",
  status: "queued",
  scale: 2,
  frame_rate: 1,
  files: ["ep01.mkv", "ep02.mkv"],
  progress: queuedProgress,
  created_at: "2026-04-29T12:30:00Z",
  finished_at: null,
};

export const runningJob: Job = {
  id: "job_r8a3bc",
  type: "upscale",
  status: "running",
  scale: 2,
  frame_rate: 1,
  files: ["ep01.mkv", "ep02.mkv", "ep03.mkv", "ep04.mkv", "ep05.mkv"],
  progress: runningProgress,
  created_at: minutesAgo(30),
  finished_at: null,
};

export const completedJob: Job = {
  id: "job_c1d2ef",
  type: "interpolate",
  status: "completed",
  scale: 1,
  frame_rate: 1,
  multiplier: 2,
  rife_model: "rife-v4.6",
  files: ["ep01.mkv", "ep02.mkv", "ep03.mkv", "ep04.mkv", "ep05.mkv"],
  progress: completedProgress,
  created_at: "2026-04-28T08:00:00Z",
  finished_at: "2026-04-28T10:24:00Z",
};

export const failedJob: Job = {
  id: "job_f4g5hi",
  type: "optimize",
  status: "failed",
  scale: 1,
  frame_rate: 1,
  files: ["ep01.mkv", "ep02.mkv", "ep03.mkv", "ep04.mkv", "ep05.mkv"],
  progress: failedProgress,
  created_at: "2026-04-27T18:00:00Z",
  finished_at: "2026-04-27T18:42:00Z",
};

export const cancelledJob: Job = {
  id: "job_x9y8z7",
  type: "check",
  status: "cancelled",
  scale: 1,
  frame_rate: 1,
  files: ["ep01.mkv"],
  progress: {
    total: 1,
    completed: 0,
    failed: 0,
    skipped: 0,
    current: "",
    containers: null,
  },
  created_at: "2026-04-26T09:00:00Z",
  finished_at: "2026-04-26T09:01:30Z",
};

export const customPipelineJob: Job = {
  id: "job_p5q6rs",
  type: "custom_pipeline",
  status: "running",
  scale: 1,
  frame_rate: 1,
  pipeline_name: "Anime 4K Pipeline",
  pipeline_steps: [
    { operation: "upscale", scale: 2, processor: "realesrgan", model: "realesr-animevideov3" },
    { operation: "interpolate", multiplier: 2, rife_model: "rife-v4.6" },
    { operation: "optimize", quality: "alta", codec: "libx265", preset: "fast" },
  ],
  files: ["ep01.mkv", "ep02.mkv", "ep03.mkv"],
  progress: runningProgress,
  created_at: minutesAgo(90),
  finished_at: null,
};

export const sampleJobs: Job[] = [
  runningJob,
  queuedJob,
  completedJob,
  failedJob,
  cancelledJob,
  customPipelineJob,
];

export const sampleLogs: LogEntry[] = [
  { source: "PIPELINE", level: "STEP", index: 0, message: "Starting pipeline for 5 files", time: "2026-04-29T12:00:01Z" },
  { source: "PIPELINE", level: "INFO", index: 1, message: "Loaded settings: 1 GPU, 2 streams/gpu", time: "2026-04-29T12:00:01Z" },
  { source: "GPU 0", level: "INFO", index: 2, message: "[1/3] Upscale 2x: ep01.mkv", time: "2026-04-29T12:00:02Z" },
  { source: "GPU 0", level: "INFO", index: 3, message: "Frame 240/34280 (0.7%) — 19.2 fps", time: "2026-04-29T12:00:14Z" },
  { source: "GPU 0", level: "OK", index: 4, message: "Finished ep01.mkv in 00:25:14", time: "2026-04-29T12:25:16Z" },
  { source: "GPU 0", level: "INFO", index: 5, message: "[2/3] Interpolate 2x: ep01.mkv", time: "2026-04-29T12:25:17Z" },
  { source: "GPU 0", level: "WARN", index: 6, message: "Slow frame rate detected (8.4 fps)", time: "2026-04-29T12:30:00Z" },
  { source: "FFMPEG", level: "INFO", index: 7, message: "[3/3] Optimize (alta): ep01.mkv", time: "2026-04-29T12:25:18Z" },
  { source: "FFMPEG", level: "ERRO", index: 8, message: "Audio track index out of range", time: "2026-04-29T12:25:19Z" },
  { source: "FFMPEG", level: "SKIP", index: 9, message: "Skipped subtitle track 2 (no codec)", time: "2026-04-29T12:25:19Z" },
  { source: "PIPELINE", level: "STEP", index: 10, message: "Concluído: ep01.mkv", time: "2026-04-29T12:25:20Z" },
];
