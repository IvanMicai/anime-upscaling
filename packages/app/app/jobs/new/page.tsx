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
  const [resolution, setResolution] = useState<1 | 2 | 4>(1);
  const [multiplier, setMultiplier] = useState<2 | 3 | 4>(2);
  const [rifeModel, setRifeModel] = useState("rife-v4.6");
  const [sceneThresh, setSceneThresh] = useState(10);
  const [threads, setThreads] = useState(0);
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
    if (v !== "optimize") {
      setResolution(1);
    }
    if (v !== "interpolate") {
      setMultiplier(2);
      setRifeModel("rife-v4.6");
      setSceneThresh(10);
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
        ...(type === "optimize" && resolution !== 1 && { resolution }),
        ...(type === "interpolate" && {
          multiplier,
          rife_model: rifeModel,
          scene_thresh: sceneThresh,
        }),
        ...(threads > 0 && { threads }),
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
              <SelectItem value="interpolate">Interpolate</SelectItem>
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

        {type === "optimize" && (
          <div className="space-y-2">
            <label className="text-sm font-medium">Resolution</label>
            <Select
              value={String(resolution)}
              onValueChange={(v) => setResolution(Number(v) as 1 | 2 | 4)}
            >
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="1">Original</SelectItem>
                <SelectItem value="2">1/2</SelectItem>
                <SelectItem value="4">1/4</SelectItem>
              </SelectContent>
            </Select>
          </div>
        )}

        {type === "interpolate" && (
          <>
            <div className="space-y-2">
              <label className="text-sm font-medium">Multiplier</label>
              <Select
                value={String(multiplier)}
                onValueChange={(v) => setMultiplier(Number(v) as 2 | 3 | 4)}
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="2">2x</SelectItem>
                  <SelectItem value="3">3x</SelectItem>
                  <SelectItem value="4">4x</SelectItem>
                </SelectContent>
              </Select>
            </div>

            <div className="space-y-2">
              <label className="text-sm font-medium">RIFE Model</label>
              <Select value={rifeModel} onValueChange={setRifeModel}>
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="rife-v4.6">rife-v4.6</SelectItem>
                  <SelectItem value="rife-v4.25">rife-v4.25</SelectItem>
                  <SelectItem value="rife-v4.25-lite">rife-v4.25-lite</SelectItem>
                  <SelectItem value="rife-v4.26">rife-v4.26</SelectItem>
                </SelectContent>
              </Select>
            </div>

            <div className="space-y-2">
              <label className="text-sm font-medium">Scene Detection</label>
              <Select
                value={String(sceneThresh)}
                onValueChange={(v) => setSceneThresh(Number(v))}
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="5">High (5)</SelectItem>
                  <SelectItem value="10">Medium (10)</SelectItem>
                  <SelectItem value="20">Low (20)</SelectItem>
                  <SelectItem value="100">Off (100)</SelectItem>
                </SelectContent>
              </Select>
            </div>

          </>
        )}

        {(type === "optimize" || type === "pipeline") && (
          <div className="space-y-2">
            <label className="text-sm font-medium">Threads</label>
            <Select
              value={String(threads)}
              onValueChange={(v) => setThreads(Number(v))}
            >
              <SelectTrigger>
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
