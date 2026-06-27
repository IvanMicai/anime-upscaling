"use client";

import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { OptionButtons } from "@/components/option-buttons";
import {
  PROCESSOR_OPTIONS,
  NOISE_LEVEL_OPTIONS,
  RIFE_MODEL_OPTIONS,
  CODEC_OPTIONS,
  PRESET_OPTIONS,
  TUNE_OPTIONS,
  PIX_FMT_OPTIONS,
  AUDIO_CODEC_OPTIONS,
  getModelOptions,
  getValidScales,
} from "@/lib/types";
import type {
  GPUVendor,
  PipelineOperationType,
  PipelineStep,
  QualityPreset,
  UpscaleProcessor,
} from "@/lib/types";

function Field({
  label,
  children,
}: {
  label: string;
  children: React.ReactNode;
}) {
  return (
    <div className="space-y-1.5">
      <label className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
        {label}
      </label>
      {children}
    </div>
  );
}

const SCALE_OPTS = [
  { value: 2, label: "2×" },
  { value: 3, label: "3×" },
  { value: 4, label: "4×" },
] as const;

const RES_OPTS = [
  { value: 1, label: "Original" },
  { value: 2, label: "1/2" },
  { value: 4, label: "1/4" },
] as const;

/**
 * Renders the config controls for a single processing operation. Shared by the
 * Create Job page and the pipeline builder's expanded step card so both stay in
 * sync. State lives with the caller; this component reads `config` and emits
 * partial patches via `onChange`.
 */
export function OperationFields({
  operation,
  config,
  onChange,
  gpuVendor = "",
}: {
  operation: PipelineOperationType;
  config: PipelineStep;
  onChange: (patch: Partial<PipelineStep>) => void;
  gpuVendor?: GPUVendor;
}) {
  if (operation === "upscale") {
    const processor = config.processor ?? "realesrgan";
    const model = config.model ?? "realesr-animevideov3";
    const scale = config.scale ?? 2;
    const modelOptions = getModelOptions(processor);
    const validScales = getValidScales(processor, model);

    return (
      <div className="space-y-4">
        <Field label="Processador">
          <OptionButtons
            columns={1}
            value={processor}
            onChange={(v) => {
              const p = v as UpscaleProcessor;
              const defModel =
                p === "libplacebo"
                  ? "anime4k-v4-a"
                  : p === "realcugan"
                    ? "models-se"
                    : "realesr-animevideov3";
              const vs = getValidScales(p, defModel);
              onChange({
                processor: p,
                model: defModel,
                scale: (vs.includes(scale) ? scale : vs[0]) as 2 | 3 | 4,
              });
            }}
            options={PROCESSOR_OPTIONS.map((o) => ({
              value: o.value,
              label: o.label,
              desc: o.desc,
            }))}
          />
        </Field>
        <Field label="Modelo">
          <Select
            value={model}
            onValueChange={(v) => {
              const vs = getValidScales(processor, v);
              onChange({
                model: v,
                scale: (vs.includes(scale) ? scale : vs[0]) as 2 | 3 | 4,
              });
            }}
          >
            <SelectTrigger>
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
          <OptionButtons
            columns={3}
            value={scale}
            onChange={(v) => onChange({ scale: v as 2 | 3 | 4 })}
            options={SCALE_OPTS.filter((s) => validScales.includes(s.value))}
          />
        </Field>
        <Field label="Redução de Ruído">
          <OptionButtons
            columns={4}
            value={config.noise_level ?? 0}
            onChange={(v) => onChange({ noise_level: v })}
            options={NOISE_LEVEL_OPTIONS.map((o) => ({
              value: o.value,
              label: o.label,
            }))}
          />
        </Field>
      </div>
    );
  }

  if (operation === "interpolate") {
    return (
      <div className="space-y-4">
        <Field label="Multiplicador">
          <OptionButtons
            columns={3}
            value={config.multiplier ?? 2}
            onChange={(v) => onChange({ multiplier: v as 2 | 3 | 4 })}
            options={[
              { value: 2, label: "2×" },
              { value: 3, label: "3×" },
              { value: 4, label: "4×" },
            ]}
          />
        </Field>
        <Field label="Modelo RIFE">
          <Select
            value={config.rife_model ?? "rife-v4.6"}
            onValueChange={(v) => onChange({ rife_model: v })}
          >
            <SelectTrigger>
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
            value={String(config.scene_thresh ?? 10)}
            onValueChange={(v) => onChange({ scene_thresh: Number(v) })}
          >
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="5">Alta (5) — mudanças sutis</SelectItem>
              <SelectItem value="10">Média (10) — balanceado</SelectItem>
              <SelectItem value="20">Baixa (20) — só transições óbvias</SelectItem>
              <SelectItem value="100">Desativada (100)</SelectItem>
            </SelectContent>
          </Select>
        </Field>
      </div>
    );
  }

  // optimize
  const codec = config.codec ?? "libx265";
  const isStreamCopy = codec === "copy";
  const isVP9 = codec === "libvpx-vp9";
  const useGPU = config.use_gpu ?? false;
  const frameRateMode = config.frame_rate_mode ?? "relative";

  return (
    <div className="space-y-4">
      <Field label="Codec de Vídeo">
        <Select
          value={codec}
          onValueChange={(v) => {
            const patch: Partial<PipelineStep> = {
              codec: v as PipelineStep["codec"],
            };
            if (v === "copy") {
              patch.frame_rate = 1;
              patch.frame_rate_mode = "relative";
              patch.frame_rate_absolute = undefined;
              patch.use_gpu = false;
            }
            onChange(patch);
          }}
        >
          <SelectTrigger>
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

      {!isStreamCopy && !isVP9 && (
        <label className="flex items-center gap-2 text-sm">
          <input
            type="checkbox"
            checked={useGPU}
            disabled={!gpuVendor}
            onChange={(e) => onChange({ use_gpu: e.target.checked })}
            className="size-4"
          />
          <span className="text-muted-foreground">
            {gpuVendor
              ? `Acelerar com ${gpuVendor.toUpperCase()} — compete com upscale pelos slots da GPU`
              : "Configure o GPU vendor em Settings para habilitar"}
          </span>
        </label>
      )}

      {!isStreamCopy && (
        <Field label="Qualidade">
          <Select
            value={config.quality ?? "alta"}
            onValueChange={(v) => onChange({ quality: v as QualityPreset })}
          >
            <SelectTrigger>
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
      )}

      {!isStreamCopy && !isVP9 && !useGPU && (
        <>
          <Field label="Preset">
            <Select
              value={config.preset ?? "fast"}
              onValueChange={(v) => onChange({ preset: v as PipelineStep["preset"] })}
            >
              <SelectTrigger>
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
          <Field label="Tune">
            <Select
              value={config.tune ?? "animation"}
              onValueChange={(v) => onChange({ tune: v as PipelineStep["tune"] })}
            >
              <SelectTrigger>
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
        </>
      )}

      {!isStreamCopy && (
        <Field label="Formato de Pixel">
          <Select
            value={config.pix_fmt ?? "yuv420p10le"}
            onValueChange={(v) => onChange({ pix_fmt: v as PipelineStep["pix_fmt"] })}
          >
            <SelectTrigger>
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
      )}

      <Field label="Codec de Áudio">
        <Select
          value={config.audio_codec ?? "copy"}
          onValueChange={(v) =>
            onChange({ audio_codec: v as PipelineStep["audio_codec"] })
          }
        >
          <SelectTrigger>
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
          <OptionButtons
            columns={3}
            value={config.resolution ?? 1}
            onChange={(v) => onChange({ resolution: v as 1 | 2 | 4 })}
            options={RES_OPTS}
          />
        </Field>
      )}

      {!isStreamCopy && (
        <Field label="Frame Rate">
          <div className="flex gap-2">
            <Select
              value={frameRateMode}
              onValueChange={(v) => {
                const next = v as "relative" | "absolute";
                onChange({
                  frame_rate_mode: next,
                  frame_rate_absolute:
                    next === "absolute"
                      ? (config.frame_rate_absolute ?? 24)
                      : undefined,
                });
              }}
            >
              <SelectTrigger className="w-[120px]">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="relative">Relativo</SelectItem>
                <SelectItem value="absolute">Absoluto</SelectItem>
              </SelectContent>
            </Select>
            {frameRateMode === "relative" ? (
              <Select
                value={String(config.frame_rate ?? 1)}
                onValueChange={(v) => onChange({ frame_rate: Number(v) as 1 | 2 | 4 })}
              >
                <SelectTrigger className="flex-1">
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
                value={config.frame_rate_absolute ?? ""}
                placeholder="fps"
                onChange={(e) => {
                  const raw = e.target.value;
                  onChange({
                    frame_rate_absolute:
                      raw === "" ? undefined : Math.max(1, parseFloat(raw)),
                  });
                }}
                className="h-9 flex-1 rounded-md border bg-transparent px-3 text-sm outline-none focus:ring-2 focus:ring-ring"
              />
            )}
          </div>
        </Field>
      )}

      {!useGPU && (
        <Field label="Threads">
          <Select
            value={String(config.threads ?? 0)}
            onValueChange={(v) => onChange({ threads: Number(v) })}
          >
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {[0, 1, 2, 4, 8, 16, 32].map((t) => (
                <SelectItem key={t} value={String(t)}>
                  {t === 0 ? "Auto" : String(t)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </Field>
      )}
    </div>
  );
}
