"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import Link from "next/link";
import { Button } from "@/components/ui/button";
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectLabel,
  SelectSeparator,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { FilePicker } from "@/components/file-picker";
import { createJob, getPipelines, getSettings, runPipeline } from "@/lib/api";
import { FOLDER_OPTIONS, type FolderKey } from "@/lib/file-utils";
import { computeFinalCanonicalFolder } from "@/components/pipeline-preview";
import type { GPUVendor, JobType, Pipeline, UpscaleProcessor, QualityPreset, PipelineStep } from "@/lib/types";
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

export default function NewJobPage() {
  const router = useRouter();
  const [type, setType] = useState<JobType>("upscale");
  const [source, setSource] = useState<FolderKey>("input");
  const [output, setOutput] = useState<FolderKey>("output");
  // Upscale
  const [scale, setScale] = useState<2 | 3 | 4>(2);
  const [processor, setProcessor] = useState<UpscaleProcessor>("realesrgan");
  const [model, setModel] = useState("realesr-animevideov3");
  const [noiseLevel, setNoiseLevel] = useState(0);
  // Interpolate
  const [multiplier, setMultiplier] = useState<2 | 3 | 4>(2);
  const [rifeModel, setRifeModel] = useState("rife-v4.6");
  const [sceneThresh, setSceneThresh] = useState(10);
  // Optimize
  const [codec, setCodec] = useState<PipelineStep["codec"]>("libx265");
  const [quality, setQuality] = useState<QualityPreset>("alta");
  const [preset, setPreset] = useState<PipelineStep["preset"]>("fast");
  const [tune, setTune] = useState<PipelineStep["tune"]>("animation");
  const [pixFmt, setPixFmt] = useState<PipelineStep["pix_fmt"]>("yuv420p10le");
  const [audioCodec, setAudioCodec] = useState<PipelineStep["audio_codec"]>("copy");
  const [resolution, setResolution] = useState<1 | 2 | 4>(1);
  const [threads, setThreads] = useState(0);
  const [useGPU, setUseGPU] = useState(false);
  const [gpuVendor, setGpuVendor] = useState<GPUVendor>("");

  const [selectedFiles, setSelectedFiles] = useState<string[]>([]);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [pipelines, setPipelines] = useState<Pipeline[]>([]);
  const [selectedPipelineId, setSelectedPipelineId] = useState<string | null>(null);

  useEffect(() => {
    getPipelines()
      .then(setPipelines)
      .catch(() => setPipelines([]));
    getSettings()
      .then((s) => setGpuVendor(s.gpu_vendor ?? ""))
      .catch(() => setGpuVendor(""));
  }, []);

  const isPipelineSelected = selectedPipelineId !== null;

  function handleTypeChange(v: string) {
    if (v.startsWith("pipeline:")) {
      const pipelineId = v.substring("pipeline:".length);
      setSelectedPipelineId(pipelineId);
      setSource("input");
      const p = pipelines.find((x) => x.id === pipelineId);
      if (p) setOutput(computeFinalCanonicalFolder(p.steps));
      return;
    }
    setSelectedPipelineId(null);
    const newType = v as JobType;
    setType(newType);

    // Reset to defaults
    setSource("input");
    switch (newType) {
      case "upscale":
        setScale(2);
        setProcessor("realesrgan");
        setModel("realesr-animevideov3");
        setNoiseLevel(0);
        break;
      case "interpolate":
        setMultiplier(2);
        setRifeModel("rife-v4.6");
        setSceneThresh(10);
        break;
      case "optimize":
        setCodec("libx265" as PipelineStep["codec"]);
        setQuality("alta" as QualityPreset);
        setPreset("fast" as PipelineStep["preset"]);
        setTune("animation" as PipelineStep["tune"]);
        setPixFmt("yuv420p10le" as PipelineStep["pix_fmt"]);
        setAudioCodec("copy" as PipelineStep["audio_codec"]);
        setResolution(1);
        setThreads(0);
        setUseGPU(false);
        break;
    }
  }

  async function submit(files?: string[]) {
    setSubmitting(true);
    setError(null);
    try {
      if (selectedPipelineId) {
        await runPipeline(selectedPipelineId, { files, source, output });
      } else {
        await createJob({
          type,
          files,
          source: source !== "input" ? source : undefined,
          ...(type === "upscale" && {
            scale,
            processor,
            model,
            noise_level: noiseLevel,
          }),
          ...(type === "interpolate" && {
            multiplier,
            rife_model: rifeModel,
            scene_thresh: sceneThresh,
          }),
          ...(type === "optimize" && {
            quality,
            codec,
            preset,
            tune,
            pix_fmt: pixFmt,
            audio_codec: audioCodec,
            resolution: resolution !== 1 ? resolution : undefined,
            threads: threads > 0 ? threads : undefined,
            use_gpu: useGPU || undefined,
          }),
        });
      }
      router.push("/");
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to create job");
    } finally {
      setSubmitting(false);
    }
  }

  const modelOptions = getModelOptions(processor);
  const validScales = getValidScales(processor, model);

  const isStreamCopy = codec === "copy";
  const isVP9 = codec === "libvpx-vp9";

  return (
    <div className="flex flex-col min-h-[calc(100vh-8rem)] sm:min-h-[calc(100vh-12rem)]">
      <Link href="/" className="text-sm text-blue-400 hover:underline">
        &larr; Back to Jobs
      </Link>
      <h2 className="text-lg font-semibold mt-6">Create Job</h2>

      <div className="flex flex-col flex-1 min-h-0 mt-4 gap-4">
        <div className="space-y-2">
          <label className="text-sm font-medium">Type</label>
          <Select
            value={isPipelineSelected ? `pipeline:${selectedPipelineId}` : type}
            onValueChange={handleTypeChange}
          >
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="upscale">Upscale</SelectItem>
              <SelectItem value="interpolate">Interpolate</SelectItem>
              <SelectItem value="optimize">Optimize</SelectItem>
              <SelectItem value="check">Check</SelectItem>
              {pipelines.length > 0 && (
                <>
                  <SelectSeparator />
                  <SelectGroup>
                    <SelectLabel>Saved Pipelines</SelectLabel>
                    {pipelines.map((p) => (
                      <SelectItem key={p.id} value={`pipeline:${p.id}`}>
                        {p.name}
                      </SelectItem>
                    ))}
                  </SelectGroup>
                </>
              )}
            </SelectContent>
          </Select>
        </div>

        <div className="space-y-3">
          <Field label="Pasta de origem">
            <Select value={source} onValueChange={(v) => setSource(v as FolderKey)}>
              <SelectTrigger className="h-8">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {FOLDER_OPTIONS.map((opt) => (
                  <SelectItem key={opt.value} value={opt.value}>{opt.label}</SelectItem>
                ))}
              </SelectContent>
            </Select>
          </Field>
          {isPipelineSelected && (
            <Field label="Pasta de destino">
              <Select value={output} onValueChange={(v) => setOutput(v as FolderKey)}>
                <SelectTrigger className="h-8">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {FOLDER_OPTIONS.map((opt) => (
                    <SelectItem key={opt.value} value={opt.value}>{opt.label}</SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </Field>
          )}
        </div>

        {!isPipelineSelected && (
          <div className="space-y-3">
            {type === "upscale" && (
              <>
                <Field label="Processador">
                  <Select
                    value={processor}
                    onValueChange={(v) => {
                      const p = v as UpscaleProcessor;
                      setProcessor(p);
                      const defModel =
                        v === "libplacebo" ? "anime4k-v4-a" :
                        v === "realcugan" ? "models-se" :
                        "realesr-animevideov3";
                      setModel(defModel);
                      const validScales = getValidScales(p, defModel);
                      if (!validScales.includes(scale)) setScale(validScales[0] as 2 | 3 | 4);
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
                    value={model}
                    onValueChange={(v) => {
                      setModel(v);
                      const validScales = getValidScales(processor, v);
                      if (!validScales.includes(scale)) setScale(validScales[0] as 2 | 3 | 4);
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
                    value={String(scale)}
                    onValueChange={(v) => setScale(Number(v) as 2 | 3 | 4)}
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
                    value={String(noiseLevel)}
                    onValueChange={(v) => setNoiseLevel(Number(v))}
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
            )}

            {type === "interpolate" && (
              <>
                <Field label="Multiplicador">
                  <Select
                    value={String(multiplier)}
                    onValueChange={(v) => setMultiplier(Number(v) as 2 | 3 | 4)}
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
                    value={rifeModel}
                    onValueChange={setRifeModel}
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
                    value={String(sceneThresh)}
                    onValueChange={(v) => setSceneThresh(Number(v))}
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

            {type === "optimize" && (
              <>
                <Field label="Codec de Vídeo">
                  <Select
                    value={codec}
                    onValueChange={(v) => {
                      setCodec(v as PipelineStep["codec"]);
                      if (v === "copy") {
                        setQuality("alta");
                        setPreset("fast" as PipelineStep["preset"]);
                        setTune("animation" as PipelineStep["tune"]);
                        setPixFmt("yuv420p10le" as PipelineStep["pix_fmt"]);
                      }
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
                {!isStreamCopy && !isVP9 && (
                  <Field label="Acelerar com GPU">
                    <label className="flex items-center gap-2 text-sm h-8">
                      <input
                        type="checkbox"
                        checked={useGPU}
                        disabled={!gpuVendor}
                        onChange={(e) => setUseGPU(e.target.checked)}
                        className="h-4 w-4"
                      />
                      <span className="text-muted-foreground">
                        {gpuVendor
                          ? `Usar ${gpuVendor.toUpperCase()} (hevc_${gpuVendor === "nvidia" ? "nvenc" : gpuVendor === "amd" ? "amf" : "qsv"}); compete com upscale pelos slots da GPU`
                          : "Configure GPU vendor em Settings para habilitar"}
                      </span>
                    </label>
                  </Field>
                )}
                {!isStreamCopy && (
                  <>
                    <Field label="Qualidade">
                      <Select
                        value={quality}
                        onValueChange={(v) => setQuality(v as QualityPreset)}
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
                          value={preset}
                          onValueChange={(v) => setPreset(v as PipelineStep["preset"])}
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
                          value={tune}
                          onValueChange={(v) => setTune(v as PipelineStep["tune"])}
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
                        value={pixFmt}
                        onValueChange={(v) => setPixFmt(v as PipelineStep["pix_fmt"])}
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
                    value={audioCodec}
                    onValueChange={(v) => setAudioCodec(v as PipelineStep["audio_codec"])}
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
                      value={String(resolution)}
                      onValueChange={(v) => setResolution(Number(v) as 1 | 2 | 4)}
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
                {!useGPU && (
                  <Field label="Threads">
                    <Select
                      value={String(threads)}
                      onValueChange={(v) => setThreads(Number(v))}
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
            )}

          </div>
        )}

        <div className="flex flex-col flex-1 min-h-0 gap-2">
          <label className="text-sm font-medium">Files</label>
          <FilePicker selected={selectedFiles} onChange={setSelectedFiles} dir={source} />
        </div>

        {error && <p className="text-sm text-red-400">{error}</p>}

        <div className="flex gap-2">
          <Button
            className="flex-1"
            onClick={() => submit()}
            disabled={submitting}
          >
            {submitting ? "Creating..." : "Run All"}
          </Button>
          <Button
            className="flex-1"
            variant="secondary"
            onClick={() => submit(selectedFiles)}
            disabled={submitting || selectedFiles.length === 0}
          >
            Run Selected ({selectedFiles.length})
          </Button>
        </div>
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
