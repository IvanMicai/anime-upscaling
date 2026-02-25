"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import Link from "next/link";
import { Button } from "@/components/ui/button";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { FilePicker } from "@/components/file-picker";
import { createJob } from "@/lib/api";
import type { JobType } from "@/lib/types";

export default function NewJobPage() {
  const router = useRouter();
  const [type, setType] = useState<JobType>("upscale");
  const [source, setSource] = useState<"input" | "output" | "optimized">("input");
  const [scale, setScale] = useState<2 | 4>(2);
  const [selectedFiles, setSelectedFiles] = useState<string[]>([]);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  function handleTypeChange(v: string) {
    setType(v as JobType);
    if (v !== "optimize" && v !== "check") {
      setSource("input");
    }
    if (v !== "upscale" && v !== "pipeline") {
      setScale(2);
    }
  }

  async function submit(files?: string[]) {
    setSubmitting(true);
    setError(null);
    try {
      await createJob({
        type,
        files,
        ...(source !== "input" && { source }),
        ...((type === "upscale" || type === "pipeline") && { scale }),
      });
      router.push("/");
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to create job");
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div className="flex flex-col min-h-[calc(100vh-12rem)]">
      <Link href="/" className="text-sm text-blue-400 hover:underline">
        &larr; Back to Jobs
      </Link>
      <h2 className="text-lg font-semibold mt-6">Create Job</h2>

      <div className="flex flex-col flex-1 min-h-0 mt-4 gap-4">
        <div className="space-y-2">
          <label className="text-sm font-medium">Type</label>
          <Select
            value={type}
            onValueChange={handleTypeChange}
          >
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="upscale">Upscale</SelectItem>
              <SelectItem value="optimize">Optimize</SelectItem>
              <SelectItem value="pipeline">Pipeline</SelectItem>
              <SelectItem value="check">Check</SelectItem>
            </SelectContent>
          </Select>
        </div>

        {(type === "optimize" || type === "check") && (
          <div className="space-y-2">
            <label className="text-sm font-medium">Source</label>
            <Select
              value={source}
              onValueChange={(v) => setSource(v as "input" | "output" | "optimized")}
            >
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="input">Input</SelectItem>
                <SelectItem value="output">Output (upscaled)</SelectItem>
                {type === "check" && (
                  <SelectItem value="optimized">Optimized</SelectItem>
                )}
              </SelectContent>
            </Select>
          </div>
        )}

        {(type === "upscale" || type === "pipeline") && (
          <div className="space-y-2">
            <label className="text-sm font-medium">Scale</label>
            <Select
              value={String(scale)}
              onValueChange={(v) => setScale(Number(v) as 2 | 4)}
            >
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="2">2x</SelectItem>
                <SelectItem value="4">4x</SelectItem>
              </SelectContent>
            </Select>
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
