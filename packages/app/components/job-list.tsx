"use client";

import Link from "next/link";
import { useState } from "react";
import { Trash2 } from "lucide-react";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Button } from "@/components/ui/button";
import { StatusBadge } from "@/components/status-badge";
import { Badge } from "@/components/ui/badge";
import { deleteJob } from "@/lib/api";
import type { Job } from "@/lib/types";

function formatTime(iso: string) {
  return new Date(iso).toLocaleString();
}

export function JobList({
  jobs,
  onRemove,
}: {
  jobs: Job[];
  onRemove?: () => void;
}) {
  const [removingId, setRemovingId] = useState<string | null>(null);

  async function handleRemove(job: Job) {
    const active = job.status === "running" || job.status === "queued";
    const message = active
      ? "Cancel this running job and remove it?"
      : "Remove this job?";
    if (!window.confirm(message)) return;
    setRemovingId(job.id);
    try {
      await deleteJob(job.id);
      onRemove?.();
    } catch (err) {
      window.alert(
        `Failed to remove job: ${err instanceof Error ? err.message : "unknown error"}`,
      );
    } finally {
      setRemovingId(null);
    }
  }

  if (jobs.length === 0) {
    return (
      <div className="flex h-40 items-center justify-center rounded-lg border border-dashed text-muted-foreground">
        No jobs yet. Create one to get started.
      </div>
    );
  }

  const sorted = [...jobs].sort(
    (a, b) =>
      new Date(b.created_at).getTime() - new Date(a.created_at).getTime()
  );

  return (
    <div className="rounded-lg border">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>Type</TableHead>
            <TableHead>Status</TableHead>
            <TableHead className="hidden md:table-cell">Files</TableHead>
            <TableHead className="hidden md:table-cell">Progress</TableHead>
            <TableHead className="hidden sm:table-cell">Created</TableHead>
            <TableHead />
          </TableRow>
        </TableHeader>
        <TableBody>
          {sorted.map((job) => {
            const p = job.progress;
            const done = p.completed + p.failed + p.skipped;
            return (
              <TableRow key={job.id}>
                <TableCell>
                  <Badge variant="secondary" className="font-mono capitalize">
                    {job.type === "custom_pipeline" && job.pipeline_name ? job.pipeline_name : job.type}
                  </Badge>
                </TableCell>
                <TableCell>
                  <StatusBadge status={job.status} />
                </TableCell>
                <TableCell className="hidden md:table-cell text-muted-foreground">
                  {job.files?.length ?? 0} file{(job.files?.length ?? 0) !== 1 ? "s" : ""}
                </TableCell>
                <TableCell className="hidden md:table-cell font-mono text-sm text-muted-foreground">
                  {p.total > 0
                    ? `${Math.round((done / p.total) * 100)}% (${done}/${p.total})`
                    : "—"}
                </TableCell>
                <TableCell className="hidden sm:table-cell text-sm text-muted-foreground">
                  {formatTime(job.created_at)}
                </TableCell>
                <TableCell className="text-right pl-2">
                  <div className="flex items-center justify-end gap-3">
                    <Link
                      href={`/jobs/${job.id}`}
                      className="text-sm text-blue-400 hover:underline"
                    >
                      View
                    </Link>
                    <Button
                      variant="ghost"
                      size="icon"
                      aria-label="Remove job"
                      title="Remove job"
                      onClick={() => handleRemove(job)}
                      disabled={removingId === job.id}
                    >
                      <Trash2 className="h-4 w-4 text-red-400" />
                    </Button>
                  </div>
                </TableCell>
              </TableRow>
            );
          })}
        </TableBody>
      </Table>
    </div>
  );
}
