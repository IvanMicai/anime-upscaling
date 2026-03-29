export type JobType = "upscale" | "optimize" | "check" | "interpolate" | "custom_pipeline";

export type JobStatus = "queued" | "running" | "completed" | "failed" | "cancelled";

export type LogLevel = "INFO" | "OK" | "ERRO" | "SKIP" | "WARN" | "STEP";

export type LogSource = "GPU 0" | "GPU 1" | "FFMPEG" | "PIPELINE";

export interface ContainerProgress {
  frame: number;
  fps: number;
  total_frames?: number;
  elapsed?: string;
  speed?: string;
  percent?: number;
  filename?: string;
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
  // Upscale
  scale?: 2 | 4;
  processor?: UpscaleProcessor;
  model?: string;
  noise_level?: number;
  // Interpolate
  multiplier?: 2 | 3 | 4;
  rife_model?: string;
  scene_thresh?: number;
  // Optimize
  quality?: QualityPreset;
  resolution?: 1 | 2 | 4;
  threads?: number;
  codec?: "libx265" | "libx264" | "libvpx-vp9" | "copy";
  preset?: "ultrafast" | "superfast" | "veryfast" | "fast" | "medium" | "slow" | "slower" | "veryslow";
  tune?: "animation" | "film" | "grain" | "zerolatency" | "none";
  pix_fmt?: "yuv420p10le" | "yuv420p" | "yuv444p";
  audio_codec?: "copy" | "aac" | "libopus" | "libmp3lame";
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

export type UpscaleProcessor = "realesrgan" | "libplacebo" | "realcugan";

export interface PipelineStep {
  operation: PipelineOperationType;
  scale?: 2 | 4;
  processor?: UpscaleProcessor;
  model?: string;
  noise_level?: number;
  multiplier?: 2 | 3 | 4;
  rife_model?: string;
  scene_thresh?: number;
  quality?: QualityPreset;
  resolution?: 1 | 2 | 4;
  threads?: number;
  codec?: "libx265" | "libx264" | "libvpx-vp9" | "copy";
  preset?: "ultrafast" | "superfast" | "veryfast" | "fast" | "medium" | "slow" | "slower" | "veryslow";
  tune?: "animation" | "film" | "grain" | "zerolatency" | "none";
  pix_fmt?: "yuv420p10le" | "yuv420p" | "yuv444p";
  audio_codec?: "copy" | "aac" | "libopus" | "libmp3lame";
}

export const QUALITY_PRESETS: Record<QualityPreset, { crf: number; label: string }> = {
  ultra: { crf: 16, label: "Ultra" },
  alta:  { crf: 19, label: "Alta" },
  media: { crf: 22, label: "Média" },
  baixa: { crf: 26, label: "Baixa" },
};

// Upscale options

export const PROCESSOR_OPTIONS = [
  { value: "realesrgan", label: "RealESRGAN", desc: "IA para super-resolução, melhor para anime" },
  { value: "libplacebo", label: "libplacebo", desc: "Shaders GPU (Anime4K), rápido e customizável" },
  { value: "realcugan", label: "RealCUGAN", desc: "IA otimizada para ilustrações com redução de ruído" },
] as const;

export const REALESRGAN_MODELS = [
  { value: "realesr-animevideov3", label: "Anime Video v3", desc: "Otimizado para vídeos de anime (recomendado)" },
  { value: "realesrgan-plus-anime", label: "Plus Anime", desc: "Otimizado para imagens de anime" },
  { value: "realesrgan-plus", label: "Plus", desc: "Modelo genérico para qualquer conteúdo" },
] as const;

export const LIBPLACEBO_SHADERS = [
  { value: "anime4k-v4-a", label: "Anime4K A", desc: "Rápido, boa qualidade geral" },
  { value: "anime4k-v4-a+a", label: "Anime4K A+A", desc: "Mais detalhes, um pouco mais lento" },
  { value: "anime4k-v4-b", label: "Anime4K B", desc: "Balanço entre velocidade e qualidade" },
  { value: "anime4k-v4-b+b", label: "Anime4K B+B", desc: "Mais detalhes no modo B" },
  { value: "anime4k-v4-c", label: "Anime4K C", desc: "Máxima qualidade, mais lento" },
  { value: "anime4k-v4-c+a", label: "Anime4K C+A", desc: "Qualidade máxima com restauração" },
  { value: "anime4k-v4.1-gan", label: "Anime4K v4.1 GAN", desc: "Rede adversarial generativa, resultado mais nítido" },
] as const;

export const REALCUGAN_MODELS = [
  { value: "models-se", label: "Standard Edition", desc: "Bom equilíbrio entre qualidade e velocidade (padrão)" },
  { value: "models-pro", label: "Pro", desc: "Maior qualidade, mais lento" },
  { value: "models-nose", label: "No Sharpening", desc: "Sem sharpening, resultado mais suave" },
] as const;

export const NOISE_LEVEL_OPTIONS = [
  { value: 0, label: "Desativado", desc: "Sem redução de ruído" },
  { value: 1, label: "Baixo", desc: "Leve redução, preserva detalhes" },
  { value: 2, label: "Médio", desc: "Redução moderada de ruído" },
  { value: 3, label: "Alto", desc: "Máxima redução, pode perder detalhes finos" },
] as const;

// Interpolate options

export const RIFE_MODEL_OPTIONS = [
  { value: "rife-v4.6", label: "v4.6", desc: "Mais recente, melhor qualidade geral (recomendado)" },
  { value: "rife-v4.26", label: "v4.26", desc: "Experimental, alta qualidade" },
  { value: "rife-v4.25", label: "v4.25", desc: "Muito boa qualidade" },
  { value: "rife-v4.25-lite", label: "v4.25 Lite", desc: "Mais rápido, boa qualidade" },
  { value: "rife-v4", label: "v4", desc: "Estável, boa qualidade" },
  { value: "rife-v3.1", label: "v3.1", desc: "Legado, rápido" },
  { value: "rife-v3.0", label: "v3.0", desc: "Legado" },
  { value: "rife-v2.4", label: "v2.4", desc: "Legado" },
  { value: "rife-v2.3", label: "v2.3", desc: "Legado" },
  { value: "rife-v2", label: "v2", desc: "Legado" },
  { value: "rife-anime", label: "Anime", desc: "Otimizado para anime" },
  { value: "rife-UHD", label: "UHD", desc: "Para vídeos ultra HD" },
  { value: "rife-HD", label: "HD", desc: "Para vídeos HD" },
  { value: "rife", label: "Original", desc: "Modelo base" },
] as const;

// Optimize options

export const CODEC_OPTIONS = [
  { value: "libx265", label: "H.265 / HEVC", desc: "Melhor compressão, arquivos menores" },
  { value: "libx264", label: "H.264 / AVC", desc: "Compatibilidade universal" },
  { value: "libvpx-vp9", label: "VP9", desc: "Codec aberto do Google/YouTube" },
  { value: "copy", label: "Copiar stream", desc: "Sem re-encode, quase instantâneo" },
] as const;

export const PRESET_OPTIONS = [
  { value: "ultrafast", label: "Ultrafast", desc: "Mais rápido, arquivo maior" },
  { value: "superfast", label: "Superfast", desc: "Muito rápido" },
  { value: "veryfast", label: "Veryfast", desc: "Bem rápido" },
  { value: "fast", label: "Fast", desc: "Rápido, bom equilíbrio" },
  { value: "medium", label: "Medium", desc: "Padrão do FFmpeg" },
  { value: "slow", label: "Slow", desc: "Lento, arquivo menor" },
  { value: "slower", label: "Slower", desc: "Bem lento" },
  { value: "veryslow", label: "Veryslow", desc: "Menor arquivo possível" },
] as const;

export const TUNE_OPTIONS = [
  { value: "animation", label: "Animação", desc: "Otimizado para anime/cartoon" },
  { value: "film", label: "Filme", desc: "Para conteúdo live-action" },
  { value: "grain", label: "Grão", desc: "Preserva textura granulada de filmes" },
  { value: "zerolatency", label: "Zero Latência", desc: "Para streaming em tempo real" },
  { value: "none", label: "Nenhum", desc: "Sem otimização de tune" },
] as const;

export const PIX_FMT_OPTIONS = [
  { value: "yuv420p10le", label: "10-bit (yuv420p10le)", desc: "Melhor qualidade de cor e degradês" },
  { value: "yuv420p", label: "8-bit (yuv420p)", desc: "Máxima compatibilidade" },
  { value: "yuv444p", label: "4:4:4 (yuv444p)", desc: "100% nitidez de cor, arquivo maior" },
] as const;

export const AUDIO_CODEC_OPTIONS = [
  { value: "copy", label: "Copiar original", desc: "Mantém áudio sem alteração" },
  { value: "aac", label: "AAC", desc: "Compatível com MP4/streaming" },
  { value: "libopus", label: "Opus", desc: "Melhor qualidade em baixo bitrate" },
  { value: "libmp3lame", label: "MP3", desc: "Compatibilidade universal" },
] as const;

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

