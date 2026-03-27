"use client";

import Link from "next/link";
import { PipelineBuilder } from "@/components/pipeline-builder";

export default function NewPipelinePage() {
  return (
    <div>
      <Link href="/pipelines" className="text-sm text-blue-400 hover:underline">
        &larr; Voltar para Pipelines
      </Link>
      <h2 className="text-lg font-semibold mt-6 mb-4">Novo Pipeline</h2>
      <PipelineBuilder />
    </div>
  );
}
