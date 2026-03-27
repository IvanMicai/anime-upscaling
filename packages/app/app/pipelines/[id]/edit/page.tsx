"use client";

import { useEffect, useState } from "react";
import { useParams } from "next/navigation";
import Link from "next/link";
import { PipelineBuilder } from "@/components/pipeline-builder";
import { getPipeline } from "@/lib/api";
import type { Pipeline } from "@/lib/types";

export default function EditPipelinePage() {
  const params = useParams();
  const id = params.id as string;
  const [pipeline, setPipeline] = useState<Pipeline | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    getPipeline(id)
      .then(setPipeline)
      .catch((err) => setError(err instanceof Error ? err.message : "Failed to load pipeline"));
  }, [id]);

  if (error) {
    return (
      <div>
        <Link href="/pipelines" className="text-sm text-blue-400 hover:underline">
          &larr; Voltar para Pipelines
        </Link>
        <p className="text-sm text-red-400 mt-4">{error}</p>
      </div>
    );
  }

  if (!pipeline) {
    return (
      <div>
        <Link href="/pipelines" className="text-sm text-blue-400 hover:underline">
          &larr; Voltar para Pipelines
        </Link>
        <p className="text-sm text-muted-foreground mt-4">Carregando...</p>
      </div>
    );
  }

  return (
    <div>
      <Link href="/pipelines" className="text-sm text-blue-400 hover:underline">
        &larr; Voltar para Pipelines
      </Link>
      <h2 className="text-lg font-semibold mt-6 mb-4">Editar Pipeline</h2>
      <PipelineBuilder pipeline={pipeline} />
    </div>
  );
}
