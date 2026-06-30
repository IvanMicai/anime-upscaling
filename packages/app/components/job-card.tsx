"use client";

import Link from "next/link";
import { Trash2 } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { StatusBadge } from "@/components/status-badge";
import { JobMiniProgress } from "@/components/job-progress";
import {
  formatEta,
  formatRelativeTime,
  jobPercent,
  jobTypeLabel,
} from "@/lib/format";
import type { Job } from "@/lib/types";

export function JobCard({
  job,
  onRequestRemove,
  removing,
}: {
  job: Job;
  onRequestRemove: (job: Job) => void;
  removing?: boolean;
}) {
  const p = job.progress;
  const fileCount = job.files?.length ?? 0;
  const pct = jobPercent(p);
  const eta = formatEta(job);

  return (
    <Link
      href={`/jobs/${job.id}`}
      className="block min-w-0 rounded-lg border bg-card/50 p-4 transition-colors hover:bg-card"
    >
      <div className="flex items-center justify-between gap-2">
        <Badge
          variant="secondary"
          className="min-w-0 shrink font-mono capitalize"
        >
          <span className="min-w-0 truncate">{jobTypeLabel(job)}</span>
        </Badge>
        <div className="flex shrink-0 items-center gap-1">
          <StatusBadge status={job.status} />
          <Button
            variant="ghost"
            size="icon-sm"
            aria-label="Remove job"
            title="Remove job"
            disabled={removing}
            onClick={(e) => {
              e.preventDefault();
              e.stopPropagation();
              onRequestRemove(job);
            }}
          >
            <Trash2 className="size-4 text-red-400" />
          </Button>
        </div>
      </div>

      <div className="mt-3 flex items-baseline gap-2">
        <span className="text-2xl font-bold tabular-nums">
          {p.total > 0 ? `${pct}%` : "—"}
        </span>
        <span className="text-sm text-muted-foreground">
          {p.total > 0
            ? `${p.completed + p.failed + p.skipped} / ${p.total}`
            : `${fileCount} file${fileCount !== 1 ? "s" : ""}`}
        </span>
      </div>

      <JobMiniProgress p={p} className="mt-2" />

      <div className="mt-2 flex items-center justify-between font-mono text-xs text-muted-foreground">
        <span>
          {job.status === "running"
            ? `ETA ${eta}`
            : job.status === "queued"
              ? "waiting for slot"
              : ""}
        </span>
        <span>{formatRelativeTime(job.created_at)}</span>
      </div>
    </Link>
  );
}
