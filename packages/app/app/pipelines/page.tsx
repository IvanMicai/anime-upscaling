"use client";

import { useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { Plus, Play, Pencil, Trash2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { FilePicker } from "@/components/file-picker";
import { usePoll } from "@/lib/use-poll";
import { getPipelines, deletePipeline, runPipeline } from "@/lib/api";
import { FOLDER_OPTIONS, type FolderKey } from "@/lib/file-utils";
import type { Pipeline } from "@/lib/types";
import {
  computePreview,
  formatStateLabel,
  formatSizeEstimate,
  formatStepSummary,
} from "@/components/pipeline-preview";

export default function PipelinesPage() {
  const { data: pipelines, refresh } = usePoll(getPipelines, 5000);
  const router = useRouter();
  const [runTarget, setRunTarget] = useState<Pipeline | null>(null);
  const [selectedFiles, setSelectedFiles] = useState<string[]>([]);
  const [source, setSource] = useState<FolderKey>("input");
  const [browsePath, setBrowsePath] = useState<string>("");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function handleDelete(id: string) {
    try {
      await deletePipeline(id);
      refresh();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Falha ao deletar");
    }
  }

  async function handleRun(files?: string[]) {
    if (!runTarget) return;
    setSubmitting(true);
    setError(null);
    try {
      await runPipeline(runTarget.id, { files, source, path: browsePath || undefined });
      setRunTarget(null);
      router.push("/");
    } catch (err) {
      setError(err instanceof Error ? err.message : "Falha ao executar");
    } finally {
      setSubmitting(false);
    }
  }

  const initial = { width: 1920, height: 1080, fps: 24, optimized: false, crf: null, codec: null };

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <h2 className="text-lg font-semibold">Pipelines</h2>
        <Link href="/pipelines/new">
          <Button size="sm">
            <Plus className="h-3.5 w-3.5 mr-1" />
            Novo Pipeline
          </Button>
        </Link>
      </div>

      {error && <p className="text-sm text-red-400 mb-4">{error}</p>}

      {!pipelines || pipelines.length === 0 ? (
        <div className="rounded-lg border border-dashed border-muted-foreground/25 p-8 text-center">
          <p className="text-sm text-muted-foreground mb-3">Nenhum pipeline criado</p>
          <Link href="/pipelines/new">
            <Button variant="outline" size="sm">
              <Plus className="h-3.5 w-3.5 mr-1" />
              Criar primeiro pipeline
            </Button>
          </Link>
        </div>
      ) : (
        <div className="space-y-3">
          {pipelines.map((p) => {
            const final_ = computePreview(p.steps);
            const sizeEst = formatSizeEstimate(final_);
            return (
              <div key={p.id} className="rounded-lg border border-border bg-card p-4">
                <div className="flex items-start justify-between">
                  <div className="flex-1 min-w-0">
                    <h3 className="font-semibold text-sm">{p.name}</h3>
                    <p className="text-xs text-muted-foreground mt-1">
                      {formatStepSummary(p.steps)}
                    </p>
                    <p className="text-xs text-muted-foreground mt-0.5">
                      {formatStateLabel(initial)}
                      <span className="mx-1">→</span>
                      {formatStateLabel(final_)}
                    </p>
                    {sizeEst && (
                      <p className="text-xs text-muted-foreground/70 mt-0.5">{sizeEst}</p>
                    )}
                  </div>
                  <div className="flex items-center gap-1 ml-2 sm:ml-3 shrink-0">
                    <Button
                      variant="outline"
                      size="xs"
                      onClick={() => {
                        setSelectedFiles([]);
                        setBrowsePath("");
                        setError(null);
                        setSource("input");
                        setRunTarget(p);
                      }}
                    >
                      <Play className="h-3 w-3 mr-1" />
                      Executar
                    </Button>
                    <Link href={`/pipelines/${p.id}/edit`}>
                      <Button variant="ghost" size="icon" className="h-7 w-7">
                        <Pencil className="h-3 w-3" />
                      </Button>
                    </Link>
                    <Button
                      variant="ghost"
                      size="icon"
                      className="h-7 w-7 text-destructive hover:text-destructive"
                      onClick={() => handleDelete(p.id)}
                    >
                      <Trash2 className="h-3 w-3" />
                    </Button>
                  </div>
                </div>
              </div>
            );
          })}
        </div>
      )}

      {/* Run dialog */}
      <Dialog open={!!runTarget} onOpenChange={(open) => !open && setRunTarget(null)}>
        <DialogContent className="sm:max-w-2xl max-h-[85vh] sm:max-h-[80vh] flex flex-col">
          <DialogHeader>
            <DialogTitle>Executar: {runTarget?.name}</DialogTitle>
          </DialogHeader>
          <div className="space-y-1">
            <label className="text-xs text-muted-foreground">Pasta de origem</label>
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
          </div>
          <div className="flex-1 min-h-0 overflow-auto">
            <FilePicker
              selected={selectedFiles}
              onChange={setSelectedFiles}
              dir={source}
              path={browsePath}
              onPathChange={setBrowsePath}
            />
          </div>
          {error && <p className="text-sm text-red-400">{error}</p>}
          <div className="flex gap-2 pt-2">
            <Button className="flex-1" onClick={() => handleRun()} disabled={submitting}>
              {submitting ? "Executando..." : "Run All"}
            </Button>
            <Button
              className="flex-1"
              variant="secondary"
              onClick={() => handleRun(selectedFiles)}
              disabled={submitting || selectedFiles.length === 0}
            >
              Run Selected ({selectedFiles.length})
            </Button>
          </div>
        </DialogContent>
      </Dialog>
    </div>
  );
}
