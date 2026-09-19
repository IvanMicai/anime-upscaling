"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import Link from "next/link";
import {
  ArrowLeft,
  ChevronLeft,
  ChevronRight,
  Combine,
  Film,
  Maximize2,
  Play,
  ShieldCheck,
  Sliders,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { FilePicker } from "@/components/file-picker";
import { OperationFields } from "@/components/operation-fields";
import { ResultPreview } from "@/components/result-preview";
import { MergeFields, MERGE_DEFAULTS } from "@/components/merge-fields";
import { MergePreviewPanel } from "@/components/merge-preview-panel";
import { MergeVideoChoice } from "@/components/merge-video-choice";
import { createJob, getPipelines, getSettings, runPipeline } from "@/lib/api";
import { cn } from "@/lib/utils";
import { FOLDER_COLORS, type FolderKey } from "@/lib/file-utils";
import {
  OPERATION_DEFAULTS,
  type CreateJobRequest,
  type GPUVendor,
  type JobType,
  type MergeConfig,
  type MergePair,
  type Pipeline,
  type PipelineOperationType,
  type PipelineStep,
} from "@/lib/types";

const JOB_TYPES: {
  value: Exclude<JobType, "custom_pipeline">;
  label: string;
  icon: React.ComponentType<{ className?: string }>;
}[] = [
  { value: "merge", label: "Merge", icon: Combine },
  { value: "upscale", label: "Upscale", icon: Maximize2 },
  { value: "interpolate", label: "Interpolate", icon: Film },
  { value: "optimize", label: "Optimize", icon: Sliders },
  { value: "check", label: "Check", icon: ShieldCheck },
];

// Labels come from FOLDER_COLORS so the source buttons stay in sync with the
// file-picker legend tags (e.g. "output" reads "Upscaling", not "Output").
const SOURCE_OPTS = (["input", "merged", "output", "interpolated", "optimized"] as FolderKey[]).map(
  (value) => ({ value, label: FOLDER_COLORS[value].label }),
);

type WizardStep = 1 | 2 | 3;

// The merge settings are a THIRD step, after the files. Asked before them the
// only answer available is "Automático": the language options are read from the
// selection, so at step 1 there is nothing to choose from — and "Automático"
// keeps whichever picture has more pixels, which is how a whole season came out
// of an AI upscale with a burnt-in watermark.
const STEPS_BASE: { n: WizardStep; label: string }[] = [
  { n: 1, label: "Configurar" },
  { n: 2, label: "Selecionar arquivos" },
];
const STEP_MERGE: { n: WizardStep; label: string } = { n: 3, label: "Ajustes do merge" };

export default function NewJobPage() {
  const router = useRouter();
  const [step, setStep] = useState<WizardStep>(1);
  const [type, setType] = useState<JobType>("upscale");
  const [source, setSource] = useState<FolderKey>("input");
  const [cfg, setCfg] = useState<PipelineStep>(OPERATION_DEFAULTS.upscale);
  const [selectedPipelineId, setSelectedPipelineId] = useState<string | null>(null);

  const [mergeCfg, setMergeCfg] = useState<MergeConfig>(MERGE_DEFAULTS);
  const [selectedDirs, setSelectedDirs] = useState<string[]>([]);
  const [mergeLanguages, setMergeLanguages] = useState<string[]>([]);
  const [mergeFirstPair, setMergeFirstPair] = useState<MergePair | null>(null);

  const [selectedFiles, setSelectedFiles] = useState<string[]>([]);
  const [browsePath, setBrowsePath] = useState<string>("");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [pipelines, setPipelines] = useState<Pipeline[]>([]);
  const [gpuVendor, setGpuVendor] = useState<GPUVendor>("");

  useEffect(() => {
    getPipelines()
      .then(setPipelines)
      .catch(() => setPipelines([]));
    getSettings()
      .then((s) => setGpuVendor(s.gpu_vendor ?? ""))
      .catch(() => setGpuVendor(""));
  }, []);

  const isPipelineSelected = selectedPipelineId !== null;
  const isMerge = !isPipelineSelected && type === "merge";
  const steps = isMerge ? [...STEPS_BASE, STEP_MERGE] : STEPS_BASE;
  // Merging what is already merged makes no sense; every other stage is fair.
  const sourceOpts = isMerge ? SOURCE_OPTS.filter((o) => o.value !== "merged") : SOURCE_OPTS;
  const selectionCount = selectedFiles.length + selectedDirs.length;
  const selectedPipeline = pipelines.find((p) => p.id === selectedPipelineId);
  const showFields =
    !isPipelineSelected &&
    (type === "upscale" || type === "interpolate" || type === "optimize");

  const typeLabel = isPipelineSelected
    ? (selectedPipeline?.name ?? "Pipeline")
    : (JOB_TYPES.find((t) => t.value === type)?.label ?? type);
  const sourceLabel = SOURCE_OPTS.find((s) => s.value === source)?.label ?? source;

  function handleSelectType(t: Exclude<JobType, "custom_pipeline">) {
    setSelectedPipelineId(null);
    setType(t);
    setSource("input");
    setSelectedDirs([]);
    if (t !== "check" && t !== "merge") setCfg(OPERATION_DEFAULTS[t]);
  }

  function handleSelectPipeline(id: string) {
    setSelectedPipelineId(id);
    setSource("input");
  }

  function buildRequest(files?: string[]): CreateJobRequest {
    const base: CreateJobRequest = {
      type,
      files,
      source: source !== "input" ? source : undefined,
      path: browsePath || undefined,
    };
    if (type === "merge") {
      // Whole folders win over single files: "two folders" is the unit of a
      // merge, and mixing both would make the pairing hard to predict.
      return {
        ...base,
        files: selectedDirs.length > 0 ? undefined : files,
        paths: selectedDirs.length > 0 && files ? selectedDirs : undefined,
        merge_video: mergeCfg.video,
        merge_tick: mergeCfg.tick,
        merge_gap_fill: mergeCfg.gapFill,
        merge_force: mergeCfg.force || undefined,
        merge_default_audio: mergeCfg.defaultAudio || undefined,
      };
    }
    if (type === "upscale") {
      return {
        ...base,
        scale: cfg.scale,
        processor: cfg.processor,
        model: cfg.model,
        noise_level: cfg.noise_level,
      };
    }
    if (type === "interpolate") {
      return {
        ...base,
        multiplier: cfg.multiplier,
        rife_model: cfg.rife_model,
        scene_thresh: cfg.scene_thresh,
      };
    }
    if (type === "optimize") {
      const isCopy = cfg.codec === "copy";
      return {
        ...base,
        quality: cfg.quality,
        codec: cfg.codec,
        preset: cfg.preset,
        tune: cfg.tune,
        pix_fmt: cfg.pix_fmt,
        audio_codec: cfg.audio_codec,
        resolution: !isCopy && cfg.resolution !== 1 ? cfg.resolution : undefined,
        frame_rate:
          !isCopy && cfg.frame_rate_mode === "relative" && cfg.frame_rate !== 1
            ? cfg.frame_rate
            : undefined,
        frame_rate_mode:
          !isCopy && cfg.frame_rate_mode === "absolute" ? "absolute" : undefined,
        frame_rate_absolute:
          !isCopy &&
          cfg.frame_rate_mode === "absolute" &&
          cfg.frame_rate_absolute &&
          cfg.frame_rate_absolute > 0
            ? cfg.frame_rate_absolute
            : undefined,
        threads: cfg.threads && cfg.threads > 0 ? cfg.threads : undefined,
        use_gpu: cfg.use_gpu || undefined,
      };
    }
    return base;
  }

  async function submit(files?: string[]) {
    setSubmitting(true);
    setError(null);
    try {
      if (selectedPipelineId) {
        await runPipeline(selectedPipelineId, {
          files,
          source,
          path: browsePath || undefined,
        });
      } else {
        await createJob(buildRequest(files));
      }
      router.push("/");
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to create job");
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <Link
            href="/"
            className="inline-flex items-center gap-1 text-sm text-muted-foreground hover:text-foreground"
          >
            <ArrowLeft className="size-4" />
            Back to Jobs
          </Link>
          <h2 className="mt-2 text-xl font-bold">Create Job</h2>
        </div>

        {/* Wizard stepper */}
        <nav className="flex items-center gap-2 text-sm" aria-label="Etapas">
          {steps.map((s, i) => {
            const active = step === s.n;
            const done = step > s.n;
            // Only allow jumping to a step that's already been reached.
            const clickable = s.n <= step;
            return (
              <div key={s.n} className="flex items-center gap-2">
                {i > 0 && <span className="h-px w-6 bg-border sm:w-8" />}
                <button
                  type="button"
                  onClick={() => clickable && setStep(s.n)}
                  disabled={!clickable}
                  aria-current={active ? "step" : undefined}
                  className={cn(
                    "flex items-center gap-2 rounded-full border px-3 py-1.5 transition-colors",
                    active
                      ? "border-primary/60 bg-primary/10 text-foreground"
                      : done
                        ? "border-border text-muted-foreground hover:bg-secondary/50 hover:text-foreground"
                        : "border-border text-muted-foreground",
                    !clickable && "cursor-default",
                  )}
                >
                  <span
                    className={cn(
                      "flex size-5 items-center justify-center rounded-full text-xs font-semibold",
                      active || done
                        ? "bg-primary/20 text-foreground"
                        : "bg-secondary text-muted-foreground",
                    )}
                  >
                    {s.n}
                  </span>
                  <span className="hidden sm:inline">{s.label}</span>
                </button>
              </div>
            );
          })}
        </nav>
      </div>

      {/* Step 1 — Configure */}
      <div
        className={cn(
          "grid gap-6 lg:grid-cols-[minmax(0,1fr)_300px] lg:items-start",
          step !== 1 && "hidden",
        )}
      >
        <div className="min-w-0 space-y-5">
          <div className="space-y-2">
            <label className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
              O que fazer?
            </label>
            <div className="grid grid-cols-2 gap-2 sm:grid-cols-5">
              {JOB_TYPES.map((t) => {
                const active = !isPipelineSelected && type === t.value;
                const Icon = t.icon;
                return (
                  <button
                    key={t.value}
                    type="button"
                    onClick={() => handleSelectType(t.value)}
                    aria-pressed={active}
                    className={cn(
                      "flex items-center gap-2 rounded-md border px-3 py-2 text-sm transition-colors",
                      active
                        ? "border-primary/60 bg-primary/10 text-foreground"
                        : "border-border text-muted-foreground hover:bg-secondary/50 hover:text-foreground",
                    )}
                  >
                    <Icon className="size-4" />
                    {t.label}
                  </button>
                );
              })}
            </div>
            {pipelines.length > 0 && (
              <div className="space-y-1.5 pt-1">
                <label className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
                  Pipelines salvos
                </label>
                <div className="grid gap-2 sm:grid-cols-2">
                  {pipelines.map((p) => (
                    <button
                      key={p.id}
                      type="button"
                      onClick={() => handleSelectPipeline(p.id)}
                      aria-pressed={selectedPipelineId === p.id}
                      className={cn(
                        "flex items-center gap-2 rounded-md border px-3 py-2 text-left text-sm transition-colors",
                        selectedPipelineId === p.id
                          ? "border-primary/60 bg-primary/10 text-foreground"
                          : "border-border text-muted-foreground hover:bg-secondary/50 hover:text-foreground",
                      )}
                    >
                      <Play className="size-3.5 shrink-0" />
                      <span className="truncate">{p.name}</span>
                    </button>
                  ))}
                </div>
              </div>
            )}
          </div>

          <div className="space-y-1.5">
            <label className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
              Pasta de Origem
            </label>
            <div className="grid grid-cols-2 gap-2 sm:grid-cols-5">
              {sourceOpts.map((opt) => {
                const active = source === opt.value;
                const colors = FOLDER_COLORS[opt.value];
                return (
                  <button
                    key={opt.value}
                    type="button"
                    onClick={() => setSource(opt.value)}
                    aria-pressed={active}
                    className={cn(
                      "rounded-md border px-3 py-2 text-sm font-medium transition-colors",
                      active
                        ? cn(colors.badge, "ring-2 ring-inset ring-white/20")
                        : cn(
                            "border-border bg-transparent hover:bg-secondary/50",
                            colors.text,
                          ),
                    )}
                  >
                    {opt.label}
                  </button>
                );
              })}
            </div>
          </div>

          {showFields && (
            <OperationFields
              operation={type as PipelineOperationType}
              config={cfg}
              onChange={(patch) => setCfg((prev) => ({ ...prev, ...patch }))}
              gpuVendor={gpuVendor}
            />
          )}
        </div>

        {/* Preview + Next */}
        <div className="min-w-0 space-y-4 lg:sticky lg:top-4">
          {isPipelineSelected && selectedPipeline ? (
            <ResultPreview
              steps={selectedPipeline.steps}
              fileCount={selectedFiles.length}
            />
          ) : type === "merge" ? (
            <div className="rounded-md border border-border p-4 text-sm">
              <div className="font-medium">Resultado</div>
              <p className="mt-1 text-muted-foreground">
                Um <code>.mkv</code> por par em <span className="text-pink-400">Merged</span>: vídeo e
                áudio originais copiados sem recodificar + a segunda faixa sincronizada (AAC), com o
                veredito da sincronia nos metadados. CPU · FFmpeg.
              </p>
            </div>
          ) : type === "check" ? (
            <ResultPreview steps={[]} fileCount={selectedFiles.length} isCheck />
          ) : (
            <ResultPreview
              steps={[{ ...cfg, operation: type as PipelineOperationType }]}
              fileCount={selectedFiles.length}
            />
          )}
          <Button className="w-full" onClick={() => setStep(2)}>
            Próximo: selecionar arquivos
            <ChevronRight className="size-4" />
          </Button>
        </div>
      </div>

      {/* Step 2 — Select files (kept mounted to preserve selection/cache) */}
      <div
        className={cn(
          "flex flex-col gap-3",
          step !== 2 && "hidden",
        )}
      >
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div className="flex items-center gap-2 text-sm">
            <Button
              variant="ghost"
              size="sm"
              onClick={() => setStep(1)}
              className="text-muted-foreground"
            >
              <ChevronLeft className="size-4" />
              Voltar
            </Button>
            <span className="text-muted-foreground">
              <span className="font-medium text-foreground">{typeLabel}</span>
              {" · origem "}
              <span className="font-medium text-foreground">{sourceLabel}</span>
            </span>
          </div>
          <div className="flex items-center gap-2">
            {isMerge ? (
              <Button onClick={() => setStep(3)} disabled={selectionCount === 0}>
                Ajustes do merge ({selectionCount})
                <ChevronRight className="size-4" />
              </Button>
            ) : (
              <>
                <Button
                  variant="outline"
                  onClick={() => submit()}
                  disabled={submitting}
                >
                  {submitting ? "Criando..." : "Run All"}
                </Button>
                <Button
                  onClick={() => submit(selectedFiles)}
                  disabled={submitting || selectedFiles.length === 0}
                >
                  <Play className="size-4" />
                  Run Selected ({selectedFiles.length})
                </Button>
              </>
            )}
          </div>
        </div>

        {error && <p className="text-sm text-red-400">{error}</p>}

        {isMerge && step === 2 && (
          <MergePreviewPanel
            source={source}
            path={browsePath}
            files={selectedFiles}
            dirs={selectedDirs}
            onLanguages={setMergeLanguages}
            onFirstPair={setMergeFirstPair}
            video={mergeCfg.video}
            onPickVideo={(video) => setMergeCfg((prev) => ({ ...prev, video }))}
          />
        )}

        <div className="h-[calc(100vh-12rem)] min-h-0">
          <FilePicker
            selected={selectedFiles}
            onChange={setSelectedFiles}
            dir={source}
            path={browsePath}
            onPathChange={setBrowsePath}
            selectedDirs={isMerge ? selectedDirs : undefined}
            onDirsChange={isMerge ? setSelectedDirs : undefined}
          />
        </div>
      </div>

      {/* Step 3 — Merge settings. Only here are the languages known. */}
      {isMerge && (
        <div className={cn("flex flex-col gap-5", step !== 3 && "hidden")}>
          <div className="flex flex-wrap items-center justify-between gap-3">
            <Button
              variant="ghost"
              size="sm"
              onClick={() => setStep(2)}
              className="text-muted-foreground"
            >
              <ChevronLeft className="size-4" />
              Voltar
            </Button>
            <Button
              onClick={() => submit(selectedFiles)}
              disabled={submitting || selectionCount === 0}
            >
              <Play className="size-4" />
              {submitting ? "Criando..." : `Rodar (${selectionCount})`}
            </Button>
          </div>

          {error && <p className="text-sm text-red-400">{error}</p>}

          <section className="space-y-3">
            <h3 className="text-sm font-semibold">Qual versão dá a imagem</h3>
            <MergeVideoChoice
              source={source}
              pair={mergeFirstPair}
              video={mergeCfg.video}
              onPick={(video) => setMergeCfg((prev) => ({ ...prev, video }))}
            />
          </section>

          <section className="space-y-3 border-t border-border pt-5">
            <h3 className="text-sm font-semibold">Sincronia e faixas</h3>
            <MergeFields
              config={mergeCfg}
              onChange={(patch) => setMergeCfg((prev) => ({ ...prev, ...patch }))}
              languages={mergeLanguages}
            />
          </section>
        </div>
      )}
    </div>
  );
}
