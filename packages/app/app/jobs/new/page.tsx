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
  const [selectedFiles, setSelectedFiles] = useState<string[]>([]);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function submit(files?: string[]) {
    setSubmitting(true);
    setError(null);
    try {
      await createJob({ type, files });
      router.push("/");
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to create job");
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div className="space-y-6">
      <Link href="/" className="text-sm text-blue-400 hover:underline">
        &larr; Back to Jobs
      </Link>
      <h2 className="text-lg font-semibold">Create Job</h2>

      <div className="space-y-4 max-w-lg">
        <div className="space-y-2">
          <label className="text-sm font-medium">Type</label>
          <Select
            value={type}
            onValueChange={(v) => setType(v as JobType)}
          >
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="upscale">Upscale</SelectItem>
              <SelectItem value="optimize">Optimize</SelectItem>
              <SelectItem value="pipeline">Pipeline</SelectItem>
            </SelectContent>
          </Select>
        </div>

        <div className="space-y-2">
          <label className="text-sm font-medium">Files</label>
          <FilePicker selected={selectedFiles} onChange={setSelectedFiles} />
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
