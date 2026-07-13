import type { PipelineStep, Pipeline } from "@/lib/types";

export const upscaleStep: PipelineStep = {
  operation: "upscale",
  scale: 2,
  processor: "realesrgan",
  model: "realesr-animevideov3",
  noise_level: 0,
};

export const upscale4xStep: PipelineStep = {
  operation: "upscale",
  scale: 4,
  processor: "realesrgan",
  model: "realesrgan-plus-anime",
  noise_level: 0,
};

export const interpolateStep: PipelineStep = {
  operation: "interpolate",
  multiplier: 2,
  rife_model: "rife-v4.6",
  scene_thresh: 10,
};

export const optimizeStep: PipelineStep = {
  operation: "optimize",
  quality: "alta",
  resolution: 1,
  frame_rate: 1,
  threads: 0,
  codec: "libx265",
  preset: "fast",
  tune: "animation",
  pix_fmt: "yuv420p10le",
  audio_codec: "copy",
};

export const optimizeCopyStep: PipelineStep = {
  operation: "optimize",
  quality: "alta",
  codec: "copy",
};

export const fullPipelineSteps: PipelineStep[] = [
  upscaleStep,
  interpolateStep,
  optimizeStep,
];

export const samplePipeline: Pipeline = {
  id: "pl_a1b2c3",
  name: "Anime 4K Pipeline",
  steps: fullPipelineSteps,
  created_at: "2026-04-20T10:00:00Z",
  updated_at: "2026-04-25T15:30:00Z",
};
