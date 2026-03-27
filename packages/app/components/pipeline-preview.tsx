import type { PipelineStep } from "@/lib/types";
import { QUALITY_PRESETS } from "@/lib/types";
import { formatBytes } from "@/lib/file-utils";

interface VideoState {
  width: number;
  height: number;
  fps: number;
  optimized: boolean;
  crf: number | null;
}

const BPP_BY_CRF: Record<number, number> = {
  16: 0.120,
  19: 0.065,
  22: 0.040,
  26: 0.020,
};

export function computePreview(steps: PipelineStep[]): VideoState {
  const state: VideoState = { width: 1920, height: 1080, fps: 24, optimized: false, crf: null };
  for (const step of steps) {
    switch (step.operation) {
      case "upscale":
        state.width *= step.scale ?? 2;
        state.height *= step.scale ?? 2;
        break;
      case "interpolate":
        state.fps *= step.multiplier ?? 2;
        break;
      case "optimize": {
        const div = step.resolution ?? 1;
        state.width = Math.floor(state.width / div);
        state.height = Math.floor(state.height / div);
        state.optimized = true;
        state.crf = QUALITY_PRESETS[step.quality ?? "alta"].crf;
        break;
      }
    }
  }
  return state;
}

export function computeStateAt(steps: PipelineStep[], upToIndex: number): VideoState {
  return computePreview(steps.slice(0, upToIndex + 1));
}

export function estimateSize(state: VideoState): { perMin: number; ep24: number } | null {
  if (!state.optimized || state.crf === null) return null;
  const bpp = BPP_BY_CRF[state.crf] ?? 0.065;
  const bitrateBytes = (state.width * state.height * state.fps * bpp) / 8;
  const perMin = bitrateBytes * 60;
  const ep24 = perMin * 24;
  return { perMin, ep24 };
}

export function formatResolution(state: VideoState): string {
  return `${state.height}p`;
}

export function formatStateLabel(state: VideoState): string {
  const parts = [`${state.height}p`, `${state.fps}fps`];
  if (state.optimized) parts.push("(optimized)");
  return parts.join(" ");
}

export function formatSizeEstimate(state: VideoState): string | null {
  const est = estimateSize(state);
  if (!est) return null;
  return `~${formatBytes(est.ep24)} (24min) · ~${formatBytes(est.perMin)}/min`;
}

export function formatStepSummary(steps: PipelineStep[]): string {
  return steps.map((s) => {
    switch (s.operation) {
      case "upscale":
        return `Upscale ${s.scale ?? 2}x`;
      case "interpolate":
        return `Interpolate ${s.multiplier ?? 2}x`;
      case "optimize":
        return `Optimize (${QUALITY_PRESETS[s.quality ?? "alta"].label})`;
    }
  }).join(" → ");
}

interface PipelinePreviewProps {
  steps: PipelineStep[];
}

export function PipelinePreview({ steps }: PipelinePreviewProps) {
  if (steps.length === 0) {
    return (
      <div className="rounded-lg border border-dashed border-muted-foreground/25 p-4 text-center text-sm text-muted-foreground">
        Adicione steps para ver o preview
      </div>
    );
  }

  const initial = { width: 1920, height: 1080, fps: 24, optimized: false, crf: null };
  const final_ = computePreview(steps);
  const sizeEst = formatSizeEstimate(final_);

  return (
    <div className="rounded-lg border border-border bg-muted/50 p-4">
      <div className="text-sm font-medium text-muted-foreground mb-1">Resultado</div>
      <div className="text-base font-semibold">
        {formatStateLabel(initial)}
        <span className="mx-2 text-muted-foreground">→</span>
        {formatStateLabel(final_)}
      </div>
      {sizeEst && (
        <div className="text-sm text-muted-foreground mt-1">{sizeEst}</div>
      )}
    </div>
  );
}
