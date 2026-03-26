export type JobType = "upscale" | "optimize" | "pipeline" | "check" | "interpolate";

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
  multiplier?: number;
  rife_model?: string;
  scene_thresh?: number;
  threads?: number;
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

export interface AudioTrack {
  index: number;
  language?: string;
  title?: string;
  codec?: string;
  channels?: number;
}

export interface SubtitleTrack {
  index: number;
  language?: string;
  title?: string;
  codec?: string;
}

export interface VideoFile {
  name: string;
  size: number;
  width?: number;
  height?: number;
  has_upscaled?: boolean;
  has_optimized?: boolean;
  has_input?: boolean;
  has_interpolated?: boolean;
  upscaled_size?: number;
  optimized_size?: number;
  input_size?: number;
  interpolated_size?: number;
  upscaled_width?: number;
  upscaled_height?: number;
  optimized_width?: number;
  optimized_height?: number;
  input_width?: number;
  input_height?: number;
  interpolated_width?: number;
  interpolated_height?: number;
  audio?: AudioTrack[];
  subtitles?: SubtitleTrack[];
  input_audio?: AudioTrack[];
  input_subtitles?: SubtitleTrack[];
  upscaled_audio?: AudioTrack[];
  upscaled_subtitles?: SubtitleTrack[];
  optimized_audio?: AudioTrack[];
  optimized_subtitles?: SubtitleTrack[];
  interpolated_audio?: AudioTrack[];
  interpolated_subtitles?: SubtitleTrack[];
}

export interface FilesResponse {
  dir: string;
  files: VideoFile[];
  cached_at?: string;
}

export interface CreateJobRequest {
  type: JobType;
  files?: string[];
  source?: "input" | "output" | "optimized";
  scale?: 2 | 4;
  resolution?: 1 | 2 | 4;
  multiplier?: 2 | 3 | 4;
  rife_model?: string;
  scene_thresh?: number;
  threads?: number;
}

export interface CreateJobResponse extends Job {}

export interface CancelJobResponse extends Job {}

export interface DeleteFilesRequest {
  items: { name: string; folders: string[] }[];
}

export interface DeleteFilesResponse {
  deleted: number;
  errors: string[];
}

export interface ApiError {
  error: string;
}

