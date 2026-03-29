"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { Plus } from "lucide-react";
import { Button } from "@/components/ui/button";
import { PipelineStepCard } from "@/components/pipeline-step-card";
import { PipelinePreview } from "@/components/pipeline-preview";
import type { PipelineStep, Pipeline } from "@/lib/types";
import { createPipeline, updatePipeline } from "@/lib/api";

interface PipelineBuilderProps {
  pipeline?: Pipeline;
}

export function PipelineBuilder({ pipeline: existing }: PipelineBuilderProps) {
  const router = useRouter();
  const [name, setName] = useState(existing?.name ?? "");
  const [steps, setSteps] = useState<PipelineStep[]>(existing?.steps ?? []);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  function addStep(operation: PipelineStep["operation"]) {
    const step: PipelineStep = { operation };
    switch (operation) {
      case "upscale":
        step.scale = 2;
        step.processor = "realesrgan";
        step.model = "realesr-animevideov3";
        step.noise_level = 0;
        break;
      case "interpolate":
        step.multiplier = 2;
        step.rife_model = "rife-v4.6";
        step.scene_thresh = 10;
        break;
      case "optimize":
        step.quality = "alta";
        step.resolution = 1;
        step.threads = 0;
        step.codec = "libx265";
        step.preset = "fast";
        step.tune = "animation";
        step.pix_fmt = "yuv420p10le";
        step.audio_codec = "copy";
        break;
    }
    setSteps([...steps, step]);
  }

  function updateStep(index: number, step: PipelineStep) {
    const next = [...steps];
    next[index] = step;
    setSteps(next);
  }

  function removeStep(index: number) {
    setSteps(steps.filter((_, i) => i !== index));
  }

  function moveStep(from: number, to: number) {
    if (to < 0 || to >= steps.length) return;
    const next = [...steps];
    const [moved] = next.splice(from, 1);
    next.splice(to, 0, moved);
    setSteps(next);
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
      {steps.map((step, i) => (
        <div key={i} className="relative">
          {/* Arrow connector */}
          <div className="flex justify-center -mt-3 mb-2">
            <div className="text-muted-foreground/50 text-lg">↓</div>
          </div>
          <PipelineStepCard
            step={step}
            index={i}
            totalSteps={steps.length}
            allSteps={steps}
            onChange={(s) => updateStep(i, s)}
            onRemove={() => removeStep(i)}
            onMoveUp={() => moveStep(i, i - 1)}
            onMoveDown={() => moveStep(i, i + 1)}
          />
        </div>
      ))}

      {/* Add step buttons */}
      <div className="flex flex-col gap-2 sm:flex-row sm:gap-2">
        <Button
          variant="outline"
          size="sm"
          className="sm:flex-1"
          onClick={() => addStep("upscale")}
        >
          <Plus className="h-3.5 w-3.5 mr-1" />
          Upscale
        </Button>
        <Button
          variant="outline"
          size="sm"
          className="sm:flex-1"
          onClick={() => addStep("interpolate")}
        >
          <Plus className="h-3.5 w-3.5 mr-1" />
          Interpolate
        </Button>
        <Button
          variant="outline"
          size="sm"
          className="sm:flex-1"
          onClick={() => addStep("optimize")}
        >
          <Plus className="h-3.5 w-3.5 mr-1" />
          Optimize
        </Button>
      </div>

      {/* Preview */}
      <PipelinePreview steps={steps} />

      {error && <p className="text-sm text-red-400">{error}</p>}

      <Button onClick={handleSave} disabled={saving}>
        {saving ? "Salvando..." : existing ? "Atualizar Pipeline" : "Salvar Pipeline"}
      </Button>
    </div>
  );
}
