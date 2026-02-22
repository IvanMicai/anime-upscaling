export type JobType = "upscale" | "optimize" | "pipeline";

export type JobStatus = "running" | "completed" | "failed" | "cancelled";

export type LogLevel = "INFO" | "OK" | "ERRO" | "SKIP" | "WARN";

export type LogSource = "GPU 0" | "GPU 1" | "FFMPEG";

export interface ContainerProgress {
  frame: number;
  fps: number;
  total_frames?: number;
  elapsed?: string;
  speed?: string;
  percent?: number;
}

export interface JobProgress {
  total: number;
  completed: number;
  failed: number;
  skipped: number;
  current: string;
  container?: ContainerProgress | null;
}

export interface Job {
  id: string;
  type: JobType;
  status: JobStatus;
  files: string[];
  progress: JobProgress;
  created_at: string;
  finished_at: string | null;
}

export interface LogEntry {
  source: LogSource;
  level: LogLevel;
  index: number;
  message: string;
  time: string;
}

export interface FilesResponse {
  dir: string;
  files: string[];
}

export interface CreateJobRequest {
  type: JobType;
  files?: string[];
}

export interface CreateJobResponse extends Job {}

export interface CancelJobResponse extends Job {}

export interface ApiError {
  error: string;
}
