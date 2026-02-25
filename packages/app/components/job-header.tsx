"use client";

import { useState, useEffect } from "react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { StatusBadge } from "@/components/status-badge";
import { cancelJob, getSources, exportFiles } from "@/lib/api";
import type { Job, Source } from "@/lib/types";

function formatTime(iso: string | null) {
  if (!iso) return "—";
  return new Date(iso).toLocaleString();
}

interface JobHeaderProps {
  job: Job;
  onCancelled: () => void;
}

export function JobHeader({ job, onCancelled }: JobHeaderProps) {
  const [cancelling, setCancelling] = useState(false);
  const [sources, setSources] = useState<Source[]>([]);
  const [exporting, setExporting] = useState(false);
  const [exportMsg, setExportMsg] = useState<string | null>(null);
  const [showExport, setShowExport] = useState(false);

  useEffect(() => {
    if (job.status !== "completed") return;
    getSources().then(setSources).catch(() => {});
  }, [job.status]);

  async function handleCancel() {
    setCancelling(true);
    try {
      await cancelJob(job.id);
      onCancelled();
    } catch {
      // ignore — poll will catch up
    } finally {
      setCancelling(false);
    }
  }

  function getExportFrom(): string {
    if (job.type === "upscale") return "output";
    return "optimized"; // optimize and pipeline
  }

  async function handleExport(sourceId: string) {
    setExporting(true);
    setExportMsg(null);
    try {
      const res = await exportFiles(sourceId, job.files, getExportFrom());
      setExportMsg(`Exported ${res.copied} file${res.copied !== 1 ? "s" : ""}`);
      setShowExport(false);
    } catch (err) {
      setExportMsg(err instanceof Error ? err.message : "Export failed");
    } finally {
      setExporting(false);
    }
  }

  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between gap-4">
        <div className="space-y-1">
          <CardTitle className="font-mono text-sm">{job.id}</CardTitle>
          <div className="flex items-center gap-2">
            <Badge variant="secondary" className="capitalize">
              {job.type}
            </Badge>
            <StatusBadge status={job.status} />
          </div>
        </div>
        <div className="flex items-center gap-2">
          {job.status === "completed" && job.type !== "check" && sources.length > 0 && (
            <div className="relative">
              <Button
                variant="secondary"
                size="sm"
                onClick={() => setShowExport(!showExport)}
                disabled={exporting}
              >
                {exporting ? "Exporting..." : "Export to Source"}
              </Button>
              {showExport && (
                <div className="absolute right-0 top-full mt-1 z-10 min-w-48 rounded-md border bg-popover p-1 shadow-md">
                  {sources.map((s) => (
                    <button
                      key={s.id}
                      className="w-full rounded-sm px-3 py-2 text-left text-sm hover:bg-accent hover:text-accent-foreground"
                      onClick={() => handleExport(s.id)}
                    >
                      <div className="font-medium">{s.name}</div>
                      <div className="text-xs text-muted-foreground font-mono truncate">
                        {s.path}
                      </div>
                    </button>
                  ))}
                </div>
              )}
            </div>
          )}
          {(job.status === "running" || job.status === "queued") && (
            <Button
              variant="destructive"
              size="sm"
              onClick={handleCancel}
              disabled={cancelling}
            >
              {cancelling ? "Cancelling..." : "Cancel"}
            </Button>
          )}
        </div>
      </CardHeader>
      <CardContent className="space-y-2 text-sm">
        <div className="flex gap-8">
          <div>
            <span className="text-muted-foreground">Created: </span>
            {formatTime(job.created_at)}
          </div>
          <div>
            <span className="text-muted-foreground">Finished: </span>
            {formatTime(job.finished_at)}
          </div>
        </div>
        {job.files && job.files.length > 0 && (
          <div>
            <span className="text-muted-foreground">Files: </span>
            <span className="font-mono">
              {job.files.join(", ")}
            </span>
          </div>
        )}
        {exportMsg && (
          <p className={`text-sm ${exportMsg.startsWith("Exported") ? "text-green-400" : "text-red-400"}`}>
            {exportMsg}
          </p>
        )}
      </CardContent>
    </Card>
  );
}
