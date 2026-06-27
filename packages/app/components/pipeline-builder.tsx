"use client";

import { useEffect, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import { Plus } from "lucide-react";
import { Button } from "@/components/ui/button";
import { PipelineStepCard } from "@/components/pipeline-step-card";
import { ResultPreview } from "@/components/result-preview";
import type { PipelineStep, Pipeline, GPUVendor } from "@/lib/types";
import { OPERATION_DEFAULTS } from "@/lib/types";
import { createPipeline, updatePipeline, getSettings } from "@/lib/api";

interface PipelineBuilderProps {
  pipeline?: Pipeline;
}

export function PipelineBuilder({ pipeline: existing }: PipelineBuilderProps) {
  const router = useRouter();
  const [name, setName] = useState(existing?.name ?? "");
  // Each step carries a stable client-side id used as its React key, so the
  // card instance (and its expanded/collapsed state) follows the step when it's
  // reordered, instead of staying pinned to a position.
  const initialSteps = existing?.steps ?? [];
  const idCounter = useRef(initialSteps.length);
  const [items, setItems] = useState<{ id: number; step: PipelineStep }[]>(() =>
    initialSteps.map((step, i) => ({ id: i, step })),
  );
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [gpuVendor, setGpuVendor] = useState<GPUVendor>("");

  const steps = items.map((it) => it.step);

  useEffect(() => {
    getSettings()
      .then((s) => setGpuVendor(s.gpu_vendor ?? ""))
      .catch(() => {});
  }, []);

  function addStep(operation: PipelineStep["operation"]) {
    setItems((prev) => [
      ...prev,
      { id: idCounter.current++, step: { ...OPERATION_DEFAULTS[operation] } },
    ]);
  }

  function updateStep(index: number, step: PipelineStep) {
    setItems((prev) => prev.map((it, i) => (i === index ? { ...it, step } : it)));
  }

  function removeStep(index: number) {
    setItems((prev) => prev.filter((_, i) => i !== index));
  }

  function moveStep(from: number, to: number) {
    setItems((prev) => {
      if (to < 0 || to >= prev.length) return prev;
      const next = [...prev];
      const [moved] = next.splice(from, 1);
      next.splice(to, 0, moved);
      return next;
    });
  }

  async function handleSave() {
    if (!name.trim()) {
      setError("Nome é obrigatório");
      return;
    }
    if (steps.length === 0) {
      setError("Adicione pelo menos um step");
      return;
    }

    setSaving(true);
    setError(null);

    try {
      if (existing) {
        await updatePipeline(existing.id, { name: name.trim(), steps });
      } else {
        await createPipeline({ name: name.trim(), steps });
      }
      router.push("/pipelines");
    } catch (err) {
      setError(err instanceof Error ? err.message : "Falha ao salvar");
    } finally {
      setSaving(false);
    }
  }

  return (
    <div className="flex flex-col gap-6">
      <div className="space-y-2">
        <label className="text-sm font-medium">Nome</label>
        <input
          type="text"
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder="Ex: Full Quality Pipeline"
          className="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-sm transition-colors placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
        />
      </div>

      {/* Input card */}
      <div className="rounded-lg border border-dashed border-muted-foreground/25 p-3">
        <div className="text-xs font-semibold text-muted-foreground uppercase tracking-wider mb-1">Input</div>
        <div className="text-sm">1080p · 24fps</div>
      </div>

      {/* Steps */}
      {items.map(({ id, step }, i) => (
        <div key={id} className="relative">
          {/* Arrow connector */}
          <div className="flex justify-center -mt-3 mb-2">
            <div className="text-muted-foreground/50 text-lg">↓</div>
          </div>
          <PipelineStepCard
            step={step}
            index={i}
            totalSteps={steps.length}
            allSteps={steps}
            gpuVendor={gpuVendor}
            onChange={(s) => updateStep(i, s)}
            onRemove={() => removeStep(i)}
            onMoveUp={() => moveStep(i, i - 1)}
            onMoveDown={() => moveStep(i, i + 1)}
          />
        </div>
      ))}

      {/* Add step buttons — color-coded per operation */}
      <div className="flex flex-col gap-2 sm:flex-row sm:gap-2">
        <button
          type="button"
          className="inline-flex flex-1 items-center justify-center gap-1 rounded-md border border-blue-500/40 bg-blue-500/10 px-3 py-2 text-sm font-medium text-blue-400 transition-colors hover:bg-blue-500/20"
          onClick={() => addStep("upscale")}
        >
          <Plus className="size-3.5" />
          Upscale
        </button>
        <button
          type="button"
          className="inline-flex flex-1 items-center justify-center gap-1 rounded-md border border-purple-500/40 bg-purple-500/10 px-3 py-2 text-sm font-medium text-purple-400 transition-colors hover:bg-purple-500/20"
          onClick={() => addStep("interpolate")}
        >
          <Plus className="size-3.5" />
          Interpolate
        </button>
        <button
          type="button"
          className="inline-flex flex-1 items-center justify-center gap-1 rounded-md border border-green-500/40 bg-green-500/10 px-3 py-2 text-sm font-medium text-green-400 transition-colors hover:bg-green-500/20"
          onClick={() => addStep("optimize")}
        >
          <Plus className="size-3.5" />
          Optimize
        </button>
      </div>

      {/* Preview — same "Resultado Estimado" card used on /jobs/new */}
      {steps.length === 0 ? (
        <div className="rounded-xl border border-dashed border-muted-foreground/25 p-4 text-center text-sm text-muted-foreground">
          Adicione steps para ver o preview
        </div>
      ) : (
        <ResultPreview steps={steps} fileCount={0} />
      )}

      {error && <p className="text-sm text-red-400">{error}</p>}

      <Button onClick={handleSave} disabled={saving}>
        {saving ? "Salvando..." : existing ? "Atualizar Pipeline" : "Salvar Pipeline"}
      </Button>
    </div>
  );
}
