"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import Link from "next/link";
import { ArrowLeft, ChevronLeft, ChevronRight, Play } from "lucide-react";
import { Button } from "@/components/ui/button";
import { FilePicker } from "@/components/file-picker";
import { MergeFields, MERGE_DEFAULTS } from "@/components/merge-fields";
import { MergePreviewPanel } from "@/components/merge-preview-panel";
import { MergeVideoChoice } from "@/components/merge-video-choice";
import {
  SourceFolderPicker,
  WizardStepper,
  sourceLabel,
  type WizardStep,
} from "@/components/job-wizard";
import { createJob } from "@/lib/api";
import { cn } from "@/lib/utils";
import type { FolderKey } from "@/lib/file-utils";
import type { MergeConfig, MergePair } from "@/lib/types";

// The merge settings are a THIRD step, after the files. Asked before them the
// only answer available is "Automático": the language options are read from the
// selection, so at step 1 there is nothing to choose from — and "Automático"
// keeps whichever picture has more pixels, which is how a whole season came out
// of an AI upscale with a burnt-in watermark.
const STEPS: { n: WizardStep; label: string }[] = [
  { n: 1, label: "Configurar" },
  { n: 2, label: "Selecionar arquivos" },
  { n: 3, label: "Ajustes do merge" },
];

export default function NewMergePage() {
  const router = useRouter();
  const [step, setStep] = useState<WizardStep>(1);
  const [source, setSource] = useState<FolderKey>("input");

  const [mergeCfg, setMergeCfg] = useState<MergeConfig>(MERGE_DEFAULTS);
  const [selectedDirs, setSelectedDirs] = useState<string[]>([]);
  const [mergeLanguages, setMergeLanguages] = useState<string[]>([]);
  const [mergeFirstPair, setMergeFirstPair] = useState<MergePair | null>(null);

  const [selectedFiles, setSelectedFiles] = useState<string[]>([]);
  const [browsePath, setBrowsePath] = useState<string>("");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const selectionCount = selectedFiles.length + selectedDirs.length;

  function handleSource(s: FolderKey) {
    setSource(s);
    setSelectedDirs([]);
  }

  async function submit() {
    setSubmitting(true);
    setError(null);
    try {
      // Whole folders win over single files: "two folders" is the unit of a
      // merge, and mixing both would make the pairing hard to predict.
      await createJob({
        type: "merge",
        source: source !== "input" ? source : undefined,
        path: browsePath || undefined,
        files: selectedDirs.length > 0 ? undefined : selectedFiles,
        paths: selectedDirs.length > 0 ? selectedDirs : undefined,
        merge_video: mergeCfg.video,
        merge_tick: mergeCfg.tick,
        merge_gap_fill: mergeCfg.gapFill,
        merge_force: mergeCfg.force || undefined,
        merge_default_audio: mergeCfg.defaultAudio || undefined,
      });
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
          <h2 className="mt-2 text-xl font-bold">New Merge Job</h2>
        </div>
        <WizardStepper steps={STEPS} step={step} onStep={setStep} />
      </div>

      {/* Step 1 — Configure */}
      <div
        className={cn(
          "grid gap-6 lg:grid-cols-[minmax(0,1fr)_300px] lg:items-start",
          step !== 1 && "hidden",
        )}
      >
        <div className="min-w-0 space-y-5">
          {/* Merging what is already merged makes no sense; every other stage is fair. */}
          <SourceFolderPicker value={source} onChange={handleSource} exclude={["merged"]} />
        </div>

        <div className="min-w-0 space-y-4 lg:sticky lg:top-4">
          <div className="rounded-md border border-border p-4 text-sm">
            <div className="font-medium">Resultado</div>
            <p className="mt-1 text-muted-foreground">
              Um <code>.mkv</code> por par em <span className="text-pink-400">Merged</span>: vídeo e
              áudio originais copiados sem recodificar + a segunda faixa sincronizada (AAC), com o
              veredito da sincronia nos metadados. CPU · FFmpeg.
            </p>
          </div>
          <Button className="w-full" onClick={() => setStep(2)}>
            Próximo: selecionar arquivos
            <ChevronRight className="size-4" />
          </Button>
        </div>
      </div>

      {/* Step 2 — Select files (kept mounted to preserve selection/cache) */}
      <div className={cn("flex flex-col gap-3", step !== 2 && "hidden")}>
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
              <span className="font-medium text-foreground">Merge</span>
              {" · origem "}
              <span className="font-medium text-foreground">{sourceLabel(source)}</span>
            </span>
          </div>
          <Button onClick={() => setStep(3)} disabled={selectionCount === 0}>
            Ajustes do merge ({selectionCount})
            <ChevronRight className="size-4" />
          </Button>
        </div>

        {step === 2 && (
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
            selectedDirs={selectedDirs}
            onDirsChange={setSelectedDirs}
          />
        </div>
      </div>

      {/* Step 3 — Merge settings. Only here are the languages known. */}
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
          <Button onClick={submit} disabled={submitting || selectionCount === 0}>
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
    </div>
  );
}
