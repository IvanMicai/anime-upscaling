export type JobType = "upscale" | "optimize" | "pipeline" | "check";

export type JobStatus = "queued" | "running" | "completed" | "failed" | "cancelled";

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
  containers?: Record<string, ContainerProgress> | null;
}

export interface Job {
  id: string;
  type: JobType;
  status: JobStatus;
  scale: number;
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

export interface VideoFile {
  name: string;
  size: number;
  has_upscaled?: boolean;
  has_optimized?: boolean;
  has_input?: boolean;
  upscaled_size?: number;
  optimized_size?: number;
  input_size?: number;
}

export interface FilesResponse {
  dir: string;
  files: VideoFile[];
}

export interface CreateJobRequest {
  type: JobType;
  files?: string[];
  source?: "input" | "output" | "optimized";
  scale?: 2 | 4;
  resolution?: 1 | 2 | 4;
}

export interface CreateJobResponse extends Job {}

export interface CancelJobResponse extends Job {}

export interface ApiError {
  error: string;
}

export interface Source {
  id: string;
  name: string;
  path: string;
}

export interface SourceFile {
  name: string;
  size: number;
  in_input?: boolean;
  in_output?: boolean;
  in_optimized?: boolean;
  input_size?: number;
  output_size?: number;
  optimized_size?: number;
}
