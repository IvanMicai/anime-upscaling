"use client";

import { useState } from "react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { StatusBadge } from "@/components/status-badge";
import { cancelJob } from "@/lib/api";
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
  const [cancelling, setCancelling] = useState(false);

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
      </CardContent>
    </Card>
  );
}
