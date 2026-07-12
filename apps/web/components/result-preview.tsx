"use client";

import { Sparkles, ArrowRight } from "lucide-react";
import {
  computePreview,
  estimateSize,
  formatResolution,
} from "@/components/pipeline-preview";
import { formatBytes } from "@/lib/file-utils";
import { cn } from "@/lib/utils";
import { sectionCardPlain } from "@/lib/section";
import { PROCESSOR_OPTIONS } from "@/lib/types";
import type { PipelineOperationType, PipelineStep } from "@/lib/types";

function workerLabel(operation: PipelineOperationType, config: PipelineStep): string {
  if (operation === "optimize") {
    return config.use_gpu ? "GPU · FFmpeg (NVENC)" : "CPU · FFmpeg";
  }
  if (operation === "upscale") {
    const proc = PROCESSOR_OPTIONS.find(
      (p) => p.value === (config.processor ?? "realesrgan"),
    );
    return `GPU · ${proc?.label ?? "RealESRGAN"}`;
  }
  if (operation === "cleanup") {
    return "Limpeza · deleta arquivos";
  }
  return "GPU · RIFE";
}

/**
 * Live "estimated result" panel for the Create Job page. Reuses the pipeline
 * builder's estimation helpers so a single operation and a saved pipeline
 * render in the exact same structure (initial → final transition + size
 * estimate). Pass `steps` as the operation chain (one step for a plain
 * operation, the pipeline's steps for a saved pipeline), or `isCheck` for the
 * integrity-check variant which produces no output.
 */
export function ResultPreview({
  steps,
  fileCount,
  isCheck = false,
}: {
  steps: PipelineStep[];
  fileCount: number;
  isCheck?: boolean;
}) {
  if (isCheck) {
    return (
      <div className={cn(sectionCardPlain, "sm:rounded-xl")}>
        <div className="mb-3 flex items-center gap-2 text-sm font-medium text-blue-400">
          <Sparkles className="size-4" />
          RESULTADO ESTIMADO
        </div>
        <p className="text-sm text-muted-foreground">
          Verificação de integridade — não altera nem gera arquivos.
        </p>
      </div>
    );
  }

  const initial = computePreview([]);
  const final = computePreview(steps);
  const est = estimateSize(final);
  const total = est ? est.ep24 * Math.max(1, fileCount) : null;
  const worker =
    steps.length === 1
      ? workerLabel(steps[0].operation, steps[0])
      : `Pipeline · ${steps.length} etapas`;

  return (
    <div className="rounded-xl border bg-card/50 p-4">
      <div className="mb-3 flex items-center gap-2 text-sm font-medium text-blue-400">
        <Sparkles className="size-4" />
        RESULTADO ESTIMADO
      </div>

      <div className="flex items-center gap-3">
        <div className="flex-1 rounded-lg border border-yellow-500/30 bg-yellow-500/10 px-3 py-2 text-center">
          <div className="font-mono text-lg font-bold text-yellow-400">
            {formatResolution(initial)}
          </div>
          <div className="text-xs text-muted-foreground">{initial.fps} fps</div>
        </div>
        <ArrowRight className="size-4 shrink-0 text-muted-foreground" />
        <div className="flex-1 rounded-lg border border-blue-500/30 bg-blue-500/10 px-3 py-2 text-center">
          <div className="font-mono text-lg font-bold text-blue-400">
            {formatResolution(final)}
          </div>
          <div className="text-xs text-muted-foreground">{final.fps} fps</div>
        </div>
      </div>

      <dl className="mt-4 space-y-2 text-sm">
        {total !== null && est && (
          <>
            <div className="flex items-center justify-between">
              <dt className="text-muted-foreground">Tamanho est.</dt>
              <dd className="font-mono">~{formatBytes(total)}</dd>
            </div>
            <div className="flex items-center justify-between">
              <dt className="text-muted-foreground">Por arquivo</dt>
              <dd className="font-mono">~{formatBytes(est.ep24)}</dd>
            </div>
          </>
        )}
        <div className="flex items-center justify-between">
          <dt className="text-muted-foreground">Arquivos</dt>
          <dd className="font-mono">{fileCount || "todos"}</dd>
        </div>
        <div className="flex items-center justify-between">
          <dt className="text-muted-foreground">Worker</dt>
          <dd className="font-mono text-xs">{worker}</dd>
        </div>
      </dl>

      {!est && (
        <p className="mt-3 text-xs text-muted-foreground">
          Tamanho final depende do bitrate da fonte (sem etapa de otimização).
        </p>
      )}
    </div>
  );
}
