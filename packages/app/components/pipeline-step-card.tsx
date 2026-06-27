"use client";

import { useState } from "react";
import { ArrowUp, ArrowDown, ChevronDown, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { OperationFields } from "@/components/operation-fields";
import { cn } from "@/lib/utils";
import { sectionCardPlain } from "@/lib/section";
import {
  PROCESSOR_OPTIONS,
  QUALITY_PRESETS,
  type GPUVendor,
  type PipelineOperationType,
  type PipelineStep,
} from "@/lib/types";
import {
  codecLabel,
  computeStateAt,
  formatResolution,
  formatSizeEstimate,
} from "./pipeline-preview";

// Per-operation accent colors, matching the "add step" buttons and the file
// stage tags (upscale = blue, interpolate = purple, optimize = green).
const OP_STYLES: Record<
  PipelineOperationType,
  { border: string; text: string; badge: string; label: string }
> = {
  upscale: {
    border: "border-blue-500/50",
    text: "text-blue-400",
    badge: "bg-blue-500/20 text-blue-400",
    label: "Upscale",
  },
  interpolate: {
    border: "border-purple-500/50",
    text: "text-purple-400",
    badge: "bg-purple-500/20 text-purple-400",
    label: "Interpolate",
  },
  optimize: {
    border: "border-green-500/50",
    text: "text-green-400",
    badge: "bg-green-500/20 text-green-400",
    label: "Optimize",
  },
};

// Summary chips shown when a step is collapsed, e.g. RealESRGAN · animevideov3 · 2×.
function stepChips(step: PipelineStep): string[] {
  switch (step.operation) {
    case "upscale": {
      const proc =
        PROCESSOR_OPTIONS.find((p) => p.value === (step.processor ?? "realesrgan"))
          ?.label ?? "RealESRGAN";
      const chips = [proc, step.model ?? "realesr-animevideov3", `${step.scale ?? 2}×`];
      if ((step.noise_level ?? 0) > 0) chips.push(`ruído ${step.noise_level}`);
      return chips;
    }
    case "interpolate":
      return [`${step.multiplier ?? 2}×`, step.rife_model ?? "rife-v4.6"];
    case "optimize": {
      if (step.codec === "copy") return ["copy stream"];
      const q = QUALITY_PRESETS[step.quality ?? "alta"];
      const chips = [codecLabel(step.codec ?? "libx265"), `${q.label} · CRF${q.crf}`];
      if (step.pix_fmt === "yuv420p10le") chips.push("10-bit");
      else if (step.pix_fmt === "yuv444p") chips.push("4:4:4");
      if (step.use_gpu) chips.push("GPU");
      return chips;
    }
  }
}

interface PipelineStepCardProps {
  step: PipelineStep;
  index: number;
  totalSteps: number;
  allSteps: PipelineStep[];
  gpuVendor?: GPUVendor;
  onChange: (step: PipelineStep) => void;
  onRemove: () => void;
  onMoveUp: () => void;
  onMoveDown: () => void;
}

export function PipelineStepCard({
  step,
  index,
  totalSteps,
  allSteps,
  gpuVendor = "",
  onChange,
  onRemove,
  onMoveUp,
  onMoveDown,
}: PipelineStepCardProps) {
  const [expanded, setExpanded] = useState(false);
  const state = computeStateAt(allSteps, index);
  const sizeEst = formatSizeEstimate(state);
  const opStyle = OP_STYLES[step.operation];

  return (
    <div className={cn("py-4", sectionCardPlain, "sm:bg-card", opStyle.border)}>
      <div className="mb-3 flex items-center justify-between">
        <div className="flex items-center gap-2">
          <span
            className={cn(
              "flex h-6 w-6 items-center justify-center rounded-full text-xs font-bold",
              opStyle.badge,
            )}
          >
            {index + 1}
          </span>
          <span className={cn("text-sm font-semibold", opStyle.text)}>
            {opStyle.label}
          </span>
        </div>
        <div className="flex items-center gap-1">
          <Button
            variant="ghost"
            size="icon"
            className="h-7 w-7"
            aria-label={expanded ? "Recolher" : "Editar"}
            onClick={() => setExpanded((v) => !v)}
          >
            <ChevronDown
              className={cn(
                "h-3.5 w-3.5 transition-transform",
                expanded && "rotate-180",
              )}
            />
          </Button>
          <Button
            variant="ghost"
            size="icon"
            className="h-7 w-7"
            onClick={onMoveUp}
            disabled={index === 0}
          >
            <ArrowUp className="h-3.5 w-3.5" />
          </Button>
          <Button
            variant="ghost"
            size="icon"
            className="h-7 w-7"
            onClick={onMoveDown}
            disabled={index === totalSteps - 1}
          >
            <ArrowDown className="h-3.5 w-3.5" />
          </Button>
          <Button
            variant="ghost"
            size="icon"
            className="h-7 w-7 text-destructive hover:text-destructive"
            onClick={onRemove}
          >
            <X className="h-3.5 w-3.5" />
          </Button>
        </div>
      </div>

      {expanded ? (
        <OperationFields
          operation={step.operation}
          config={step}
          onChange={(patch) => onChange({ ...step, ...patch })}
          gpuVendor={gpuVendor}
        />
      ) : (
        <button
          type="button"
          onClick={() => setExpanded(true)}
          className="flex w-full flex-wrap gap-1.5 text-left"
        >
          {stepChips(step).map((c, i) => (
            <Badge key={i} variant="secondary" className="font-mono text-xs">
              {c}
            </Badge>
          ))}
        </button>
      )}

      <div className="mt-3 border-t border-border pt-3">
        <div className="text-sm text-muted-foreground">
          → {formatResolution(state)} · {state.fps}fps
          {state.optimized && ` (${codecLabel(state.codec)})`}
        </div>
        {sizeEst && (
          <div className="mt-0.5 text-xs text-muted-foreground/70">{sizeEst}</div>
        )}
      </div>
    </div>
  );
}
