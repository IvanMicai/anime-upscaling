import { ArrowUp, ArrowDown, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import type { PipelineStep, PipelineOperationType, UpscaleProcessor, GPUVendor } from "@/lib/types";
import {
  PROCESSOR_OPTIONS,
  NOISE_LEVEL_OPTIONS,
  RIFE_MODEL_OPTIONS,
  CODEC_OPTIONS,
  PRESET_OPTIONS,
  TUNE_OPTIONS,
  PIX_FMT_OPTIONS,
  AUDIO_CODEC_OPTIONS,
  SCALE_LABELS,
  getModelOptions,
  getValidScales,
} from "@/lib/types";
import { computeStateAt, formatResolution, formatSizeEstimate, codecLabel } from "./pipeline-preview";

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
  const state = computeStateAt(allSteps, index);
  const sizeEst = formatSizeEstimate(state);

  function updateField(updates: Partial<PipelineStep>) {
    onChange({ ...step, ...updates });
  }

  function handleOperationChange(op: string) {
    const base: PipelineStep = { operation: op as PipelineOperationType };
    switch (op) {
      case "upscale":
        base.scale = 2;
        base.processor = "realesrgan";
        base.model = "realesr-animevideov3";
        base.noise_level = 0;
        break;
      case "interpolate":
        base.multiplier = 2;
        base.rife_model = "rife-v4.6";
        base.scene_thresh = 10;
        break;
      case "optimize":
        base.quality = "alta";
        base.resolution = 1;
        base.frame_rate = 1;
        base.threads = 0;
        base.codec = "libx265";
        base.preset = "fast";
        base.tune = "animation";
        base.pix_fmt = "yuv420p10le";
        base.audio_codec = "copy";
        break;
    }
    onChange(base);
  }

  return (
    <div className="rounded-lg border border-border bg-card p-4">
      <div className="flex items-center justify-between mb-3">
        <div className="flex items-center gap-2">
          <span className="flex h-6 w-6 items-center justify-center rounded-full bg-primary text-primary-foreground text-xs font-bold">
            {index + 1}
          </span>
          <Select value={step.operation} onValueChange={handleOperationChange}>
            <SelectTrigger className="w-[120px] sm:w-[160px] h-8 text-sm font-semibold">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="upscale">Upscale</SelectItem>
              <SelectItem value="interpolate">Interpolate</SelectItem>
              <SelectItem value="optimize">Optimize</SelectItem>
            </SelectContent>
          </Select>
        </div>
        <div className="flex items-center gap-1">
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

      <div className="space-y-3">
        {step.operation === "upscale" && (() => {
          const proc = step.processor ?? "realesrgan";
          const modelOptions = getModelOptions(proc);
          const defaultModel = modelOptions[0].value;
          const currentModel = step.model ?? defaultModel;
          const validScales = getValidScales(proc, currentModel);
          return (
            <>
              <Field label="Processador">
                <Select
                  value={proc}
                  onValueChange={(v) => {
                    const p = v as PipelineStep["processor"];
                    const defModel =
                      v === "libplacebo" ? "anime4k-v4-a" :
                      v === "realcugan" ? "models-se" :
                      "realesr-animevideov3";
                    const newScales = getValidScales(p as UpscaleProcessor, defModel);
                    const currentScale = step.scale ?? 2;
                    const newScale = newScales.includes(currentScale) ? currentScale : newScales[0];
                    updateField({ processor: p, model: defModel, scale: newScale as 2 | 3 | 4 });
                  }}
                >
                  <SelectTrigger className="h-8">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {PROCESSOR_OPTIONS.map((opt) => (
                      <SelectItem key={opt.value} value={opt.value}>
                        {opt.label} — {opt.desc}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </Field>
              <Field label="Modelo">
                <Select
                  value={step.model ?? defaultModel}
                  onValueChange={(v) => {
                    const newScales = getValidScales(proc as UpscaleProcessor, v);
                    const currentScale = step.scale ?? 2;
                    const updates: Partial<PipelineStep> = { model: v };
                    if (!newScales.includes(currentScale)) updates.scale = newScales[0] as 2 | 3 | 4;
                    updateField(updates);
                  }}
                >
                  <SelectTrigger className="h-8">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {modelOptions.map((opt) => (
                      <SelectItem key={opt.value} value={opt.value}>
                        {opt.label} — {opt.desc}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </Field>
              <Field label="Scale">
                <Select
                  value={String(step.scale ?? 2)}
                  onValueChange={(v) => updateField({ scale: Number(v) as 2 | 3 | 4 })}
                >
                  <SelectTrigger className="h-8">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {validScales.map((s) => (
                      <SelectItem key={s} value={String(s)}>
                        {SCALE_LABELS[s] ?? `${s}x`}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </Field>
              <Field label="Redução de Ruído">
                <Select
                  value={String(step.noise_level ?? 0)}
                  onValueChange={(v) => updateField({ noise_level: Number(v) })}
                >
                  <SelectTrigger className="h-8">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {NOISE_LEVEL_OPTIONS.map((opt) => (
                      <SelectItem key={opt.value} value={String(opt.value)}>
                        {opt.label} — {opt.desc}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </Field>
            </>
          );
        })()}

        {step.operation === "interpolate" && (
          <>
            <Field label="Multiplicador">
              <Select
                value={String(step.multiplier ?? 2)}
                onValueChange={(v) => updateField({ multiplier: Number(v) as 2 | 3 | 4 })}
              >
                <SelectTrigger className="h-8">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="2">2x — Dobra o framerate</SelectItem>
                  <SelectItem value="3">3x — Triplica o framerate</SelectItem>
                  <SelectItem value="4">4x — Quadruplica o framerate</SelectItem>
                </SelectContent>
              </Select>
            </Field>
            <Field label="Modelo RIFE">
              <Select
                value={step.rife_model ?? "rife-v4.6"}
                onValueChange={(v) => updateField({ rife_model: v })}
              >
                <SelectTrigger className="h-8">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {RIFE_MODEL_OPTIONS.map((opt) => (
                    <SelectItem key={opt.value} value={opt.value}>
                      {opt.label} — {opt.desc}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </Field>
            <Field label="Detecção de Cena">
              <Select
                value={String(step.scene_thresh ?? 10)}
                onValueChange={(v) => updateField({ scene_thresh: Number(v) })}
              >
                <SelectTrigger className="h-8">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="5">Alta (5) — Detecta mudanças sutis entre cenas</SelectItem>
                  <SelectItem value="10">Média (10) — Balanço entre precisão e performance</SelectItem>
                  <SelectItem value="20">Baixa (20) — Só detecta transições óbvias</SelectItem>
                  <SelectItem value="100">Desativada (100) — Interpola tudo sem distinção</SelectItem>
                </SelectContent>
              </Select>
            </Field>
          </>
        )}

        {step.operation === "optimize" && (() => {
          const isStreamCopy = step.codec === "copy";
          const isVP9 = step.codec === "libvpx-vp9";
          const gpuEligible = !isStreamCopy && !isVP9;
          const gpuSupported = gpuEligible && gpuVendor !== "";
          const useGPU = !!step.use_gpu && gpuSupported;
          return (
            <>
              <Field label="Codec de Vídeo">
                <Select
                  value={step.codec ?? "libx265"}
                  onValueChange={(v) => {
                    const updates: Partial<PipelineStep> = { codec: v as PipelineStep["codec"] };
                    if (v === "copy" || v === "libvpx-vp9") {
                      updates.use_gpu = false;
                    }
                    if (v === "copy") {
                      updates.quality = undefined;
                      updates.preset = undefined;
                      updates.tune = undefined;
                      updates.pix_fmt = undefined;
                    }
                    updateField(updates);
                  }}
                >
                  <SelectTrigger className="h-8">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {CODEC_OPTIONS.map((opt) => (
                      <SelectItem key={opt.value} value={opt.value}>
                        {opt.label} — {opt.desc}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </Field>
              {gpuEligible && (
                <Field label="Acelerar com GPU">
                  <label className="flex items-center gap-2 text-sm">
                    <Checkbox
                      checked={useGPU}
                      disabled={!gpuSupported}
                      onCheckedChange={(c) => updateField({ use_gpu: c === true })}
                    />
                    <span className="text-muted-foreground">
                      {gpuSupported
                        ? "Usa encoder de hardware (NVENC/AMF/QSV) e disputa slots com upscale"
                        : "Configure um vendor de GPU em /settings para habilitar"}
                    </span>
                  </label>
                </Field>
              )}
              {!isStreamCopy && (
                <>
                  <Field label="Qualidade">
                    <Select
                      value={step.quality ?? "alta"}
                      onValueChange={(v) => updateField({ quality: v as PipelineStep["quality"] })}
                    >
                      <SelectTrigger className="h-8">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="ultra">Ultra (CRF 16)</SelectItem>
                        <SelectItem value="alta">Alta (CRF 19)</SelectItem>
                        <SelectItem value="media">Média (CRF 22)</SelectItem>
                        <SelectItem value="baixa">Baixa (CRF 26)</SelectItem>
                      </SelectContent>
                    </Select>
                  </Field>
                  {!isVP9 && !useGPU && (
                    <Field label="Preset">
                      <Select
                        value={step.preset ?? "fast"}
                        onValueChange={(v) => updateField({ preset: v as PipelineStep["preset"] })}
                      >
                        <SelectTrigger className="h-8">
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          {PRESET_OPTIONS.map((opt) => (
                            <SelectItem key={opt.value} value={opt.value}>
                              {opt.label} — {opt.desc}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                    </Field>
                  )}
                  {!isVP9 && !useGPU && (
                    <Field label="Tune">
                      <Select
                        value={step.tune ?? "animation"}
                        onValueChange={(v) => updateField({ tune: v as PipelineStep["tune"] })}
                      >
                        <SelectTrigger className="h-8">
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          {TUNE_OPTIONS.map((opt) => (
                            <SelectItem key={opt.value} value={opt.value}>
                              {opt.label} — {opt.desc}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                    </Field>
                  )}
                  <Field label="Formato de Pixel">
                    <Select
                      value={step.pix_fmt ?? "yuv420p10le"}
                      onValueChange={(v) => updateField({ pix_fmt: v as PipelineStep["pix_fmt"] })}
                    >
                      <SelectTrigger className="h-8">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        {PIX_FMT_OPTIONS.map((opt) => (
                          <SelectItem key={opt.value} value={opt.value}>
                            {opt.label} — {opt.desc}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </Field>
                </>
              )}
              <Field label="Codec de Áudio">
                <Select
                  value={step.audio_codec ?? "copy"}
                  onValueChange={(v) => updateField({ audio_codec: v as PipelineStep["audio_codec"] })}
                >
                  <SelectTrigger className="h-8">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {AUDIO_CODEC_OPTIONS.map((opt) => (
                      <SelectItem key={opt.value} value={opt.value}>
                        {opt.label} — {opt.desc}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </Field>
              {!isStreamCopy && (
                <Field label="Resolução">
                  <Select
                    value={String(step.resolution ?? 1)}
                    onValueChange={(v) => updateField({ resolution: Number(v) as 1 | 2 | 4 })}
                  >
                    <SelectTrigger className="h-8">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="1">Original</SelectItem>
                      <SelectItem value="2">1/2</SelectItem>
                      <SelectItem value="4">1/4</SelectItem>
                    </SelectContent>
                  </Select>
                </Field>
              )}
              {!isStreamCopy && (() => {
                const mode = step.frame_rate_mode ?? "relative";
                return (
                  <Field label="Frame Rate">
                    <div className="flex gap-2">
                      <Select
                        value={mode}
                        onValueChange={(v) => {
                          const next = v as "relative" | "absolute";
                          if (next === "absolute") {
                            updateField({
                              frame_rate_mode: "absolute",
                              frame_rate_absolute: step.frame_rate_absolute ?? 24,
                            });
                          } else {
                            updateField({
                              frame_rate_mode: "relative",
                              frame_rate_absolute: undefined,
                            });
                          }
                        }}
                      >
                        <SelectTrigger className="h-8 w-[110px]">
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          <SelectItem value="relative">Relativo</SelectItem>
                          <SelectItem value="absolute">Absoluto</SelectItem>
                        </SelectContent>
                      </Select>
                      {mode === "relative" ? (
                        <Select
                          value={String(step.frame_rate ?? 1)}
                          onValueChange={(v) => updateField({ frame_rate: Number(v) as 1 | 2 | 4 })}
                        >
                          <SelectTrigger className="h-8 w-[110px]">
                            <SelectValue />
                          </SelectTrigger>
                          <SelectContent>
                            <SelectItem value="1">Original</SelectItem>
                            <SelectItem value="2">1/2</SelectItem>
                            <SelectItem value="4">1/4</SelectItem>
                          </SelectContent>
                        </Select>
                      ) : (
                        <input
                          type="number"
                          min={1}
                          step={1}
                          value={step.frame_rate_absolute ?? ""}
                          placeholder="fps"
                          onChange={(e) => {
                            const raw = e.target.value;
                            const n = raw === "" ? undefined : Math.max(1, parseFloat(raw));
                            updateField({ frame_rate_absolute: n });
                          }}
                          className="h-8 w-[110px] rounded-md border bg-transparent px-3 text-sm outline-none focus:ring-2 focus:ring-ring"
                        />
                      )}
                    </div>
                  </Field>
                );
              })()}
              {!useGPU && (
                <Field label="Threads">
                  <Select
                    value={String(step.threads ?? 0)}
                    onValueChange={(v) => updateField({ threads: Number(v) })}
                  >
                    <SelectTrigger className="h-8">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="0">Auto</SelectItem>
                      <SelectItem value="1">1</SelectItem>
                      <SelectItem value="2">2</SelectItem>
                      <SelectItem value="4">4</SelectItem>
                      <SelectItem value="8">8</SelectItem>
                      <SelectItem value="16">16</SelectItem>
                      <SelectItem value="32">32</SelectItem>
                    </SelectContent>
                  </Select>
                </Field>
              )}
            </>
          );
        })()}
      </div>

      <div className="mt-3 pt-3 border-t border-border">
        <div className="text-sm text-muted-foreground">
          → {formatResolution(state)} · {state.fps}fps
          {state.optimized && ` (${codecLabel(state.codec)})`}
        </div>
        {sizeEst && (
          <div className="text-xs text-muted-foreground/70 mt-0.5">{sizeEst}</div>
        )}
      </div>
    </div>
  );
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="flex flex-col gap-1 sm:flex-row sm:items-center sm:gap-3">
      <label className="text-sm text-muted-foreground sm:w-28 sm:shrink-0">{label}</label>
      <div className="flex-1">{children}</div>
    </div>
  );
}
