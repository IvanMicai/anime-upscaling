export type JobType = "upscale" | "optimize" | "pipeline" | "check" | "interpolate" | "custom_pipeline";

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
  pipeline_name?: string;
  pipeline_steps?: PipelineStep[];
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

// Pipeline types

export type PipelineOperationType = "upscale" | "interpolate" | "optimize";

export type QualityPreset = "ultra" | "alta" | "media" | "baixa";

export interface PipelineStep {
  operation: PipelineOperationType;
  scale?: 2 | 4;
  multiplier?: 2 | 3 | 4;
  rife_model?: string;
  scene_thresh?: number;
  quality?: QualityPreset;
  resolution?: 1 | 2 | 4;
  threads?: number;
}

export const QUALITY_PRESETS: Record<QualityPreset, { crf: number; label: string }> = {
  ultra: { crf: 16, label: "Ultra" },
  alta:  { crf: 19, label: "Alta" },
  media: { crf: 22, label: "Média" },
  baixa: { crf: 26, label: "Baixa" },
};

export interface Pipeline {
  id: string;
  name: string;
  steps: PipelineStep[];
  created_at: string;
  updated_at: string;
}

export interface CreatePipelineRequest {
  name: string;
  steps: PipelineStep[];
}

export interface UpdatePipelineRequest {
  name?: string;
  steps?: PipelineStep[];
}

export interface RunPipelineRequest {
  files?: string[];
}

