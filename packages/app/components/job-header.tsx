"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { StatusBadge } from "@/components/status-badge";
import { cancelJob, deleteJob } from "@/lib/api";
import type { Job } from "@/lib/types";

function formatTime(iso: string | null) {
  if (!iso) return "—";
  return new Date(iso).toLocaleString();
}

interface JobHeaderProps {
  job: Job;
  onCancelled: () => void;
}

export function JobHeader({ job, onCancelled }: JobHeaderProps) {
  const router = useRouter();
  const [cancelling, setCancelling] = useState(false);
  const [removing, setRemoving] = useState(false);

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

  async function handleRemove() {
    const active = job.status === "running" || job.status === "queued";
    const message = active
      ? "Cancel this running job and remove it?"
      : "Remove this job?";
    if (!window.confirm(message)) return;
    setRemoving(true);
    router.push("/");
    try {
      await deleteJob(job.id);
    } catch (err) {
      window.alert(
        `Failed to remove job: ${err instanceof Error ? err.message : "unknown error"}`,
      );
    }
  }

  return (
    <Card>
      <CardHeader className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between sm:gap-4">
        <div className="space-y-1">
          <CardTitle className="font-mono text-sm">{job.id}</CardTitle>
          <div className="flex items-center gap-2">
            <Badge variant="secondary" className="capitalize">
              {job.type === "custom_pipeline" && job.pipeline_name ? job.pipeline_name : job.type}
            </Badge>
            <StatusBadge status={job.status} />
          </div>
        </div>
        <div className="flex items-center gap-2">
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
          <Button
            variant="outline"
            size="sm"
            onClick={handleRemove}
            disabled={removing}
          >
            {removing ? "Removing..." : "Remove"}
          </Button>
        </div>
      </CardHeader>
      <CardContent className="space-y-2 text-sm">
        <div className="flex flex-col gap-1 sm:flex-row sm:gap-8">
          <div>
            <span className="text-muted-foreground">Created: </span>
            {formatTime(job.created_at)}
          </div>
          <div>
            <span className="text-muted-foreground">Finished: </span>
            {formatTime(job.finished_at)}
          </div>
        </div>
      </CardContent>
    </Card>
  );
}
