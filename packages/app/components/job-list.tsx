"use client";

import Link from "next/link";
import { useMemo, useState } from "react";
import { Eye, Search, Trash2 } from "lucide-react";
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
import { JobCard } from "@/components/job-card";
import { JobMiniProgress } from "@/components/job-progress";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { deleteJob } from "@/lib/api";
import {
  formatEta,
  formatRelativeTime,
  jobProgressLabel,
  jobTypeLabel,
} from "@/lib/format";
import { cn } from "@/lib/utils";
import type { Job, JobStatus } from "@/lib/types";

type FilterKey = "all" | "running" | "queued" | "completed" | "failed";

const FILTERS: { key: FilterKey; label: string; active: string }[] = [
  { key: "all", label: "All", active: "bg-secondary text-foreground border-transparent" },
  { key: "running", label: "Running", active: "bg-blue-500/20 text-blue-400 border-blue-500/40" },
  { key: "queued", label: "Queued", active: "bg-purple-500/20 text-purple-400 border-purple-500/40" },
  { key: "completed", label: "Completed", active: "bg-green-500/20 text-green-400 border-green-500/40" },
  { key: "failed", label: "Failed", active: "bg-red-500/20 text-red-400 border-red-500/40" },
];

export function JobList({
  jobs,
  onRemove,
}: {
  jobs: Job[];
  onRemove?: () => void;
}) {
  const [filter, setFilter] = useState<FilterKey>("all");
  const [query, setQuery] = useState("");
  const [pending, setPending] = useState<Job | null>(null);
  const [removingId, setRemovingId] = useState<string | null>(null);

  const counts = useMemo(() => {
    const c: Record<JobStatus, number> = {
      queued: 0,
      running: 0,
      completed: 0,
      failed: 0,
      cancelled: 0,
    };
    for (const j of jobs) c[j.status]++;
    return c;
  }, [jobs]);

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase();
    return [...jobs]
      .filter((j) => filter === "all" || j.status === filter)
      .filter((j) => {
        if (!q) return true;
        const hay = [
          jobTypeLabel(j),
          j.type,
          j.pipeline_name ?? "",
          ...(j.files ?? []),
        ]
          .join(" ")
          .toLowerCase();
        return hay.includes(q);
      })
      .sort(
        (a, b) =>
          new Date(b.created_at).getTime() - new Date(a.created_at).getTime(),
      );
  }, [jobs, filter, query]);

  async function confirmRemove() {
    if (!pending) return;
    setRemovingId(pending.id);
    try {
      await deleteJob(pending.id);
      onRemove?.();
      setPending(null);
    } catch (err) {
      window.alert(
        `Failed to remove job: ${err instanceof Error ? err.message : "unknown error"}`,
      );
    } finally {
      setRemovingId(null);
    }
  }

  const pendingActive =
    pending?.status === "running" || pending?.status === "queued";

  return (
    <div className="space-y-4">
      {/* Search */}
      <div className="relative">
        <Search className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground" />
        <input
          type="text"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          placeholder="Search by type, pipeline or file…"
          className="w-full rounded-md border bg-transparent py-2 pr-3 pl-9 text-sm outline-none focus:ring-2 focus:ring-ring"
        />
      </div>

      {/* Filter pills */}
      <div className="flex flex-wrap gap-2">
        {FILTERS.map((f) => {
          const count = f.key === "all" ? jobs.length : counts[f.key as JobStatus];
          const isActive = filter === f.key;
          return (
            <button
              key={f.key}
              type="button"
              onClick={() => setFilter(f.key)}
              className={cn(
                "rounded-full border px-3 py-1 text-sm transition-colors",
                isActive
                  ? f.active
                  : "border-border text-muted-foreground hover:text-foreground",
              )}
            >
              {f.label}
              {f.key !== "all" && count > 0 && (
                <span className="ml-1.5 tabular-nums opacity-80">{count}</span>
              )}
            </button>
          );
        })}
      </div>

      {filtered.length === 0 ? (
        <div className="flex h-40 items-center justify-center rounded-lg border border-dashed text-muted-foreground">
          {jobs.length === 0
            ? "No jobs yet. Create one to get started."
            : "No jobs match the current filter."}
        </div>
      ) : (
        <>
          {/* Desktop table */}
          <div className="hidden rounded-lg border md:block">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Type</TableHead>
                  <TableHead>Status</TableHead>
                  <TableHead>Files</TableHead>
                  <TableHead>Progress</TableHead>
                  <TableHead className="hidden lg:table-cell">ETA</TableHead>
                  <TableHead className="hidden sm:table-cell">Created</TableHead>
                  <TableHead />
                </TableRow>
              </TableHeader>
              <TableBody>
                {filtered.map((job) => {
                  const p = job.progress;
                  return (
                    <TableRow key={job.id}>
                      <TableCell>
                        <Badge
                          variant="secondary"
                          className="font-mono capitalize"
                        >
                          {jobTypeLabel(job)}
                        </Badge>
                      </TableCell>
                      <TableCell>
                        <StatusBadge status={job.status} />
                      </TableCell>
                      <TableCell className="text-muted-foreground">
                        {job.files?.length ?? 0}
                      </TableCell>
                      <TableCell>
                        <div className="flex items-center gap-3">
                          <JobMiniProgress p={p} className="w-28" />
                          <span className="whitespace-nowrap font-mono text-xs text-muted-foreground tabular-nums">
                            {jobProgressLabel(p)}
                          </span>
                        </div>
                      </TableCell>
                      <TableCell className="hidden lg:table-cell font-mono text-sm text-muted-foreground tabular-nums">
                        {formatEta(job)}
                      </TableCell>
                      <TableCell className="hidden sm:table-cell text-sm text-muted-foreground">
                        {formatRelativeTime(job.created_at)}
                      </TableCell>
                      <TableCell className="pl-2 text-right">
                        <div className="flex items-center justify-end gap-1">
                          <Button
                            asChild
                            variant="ghost"
                            size="icon-sm"
                            aria-label="View job"
                            title="View job"
                          >
                            <Link href={`/jobs/${job.id}`}>
                              <Eye className="size-4" />
                            </Link>
                          </Button>
                          <Button
                            variant="ghost"
                            size="icon-sm"
                            aria-label="Remove job"
                            title="Remove job"
                            onClick={() => setPending(job)}
                            disabled={removingId === job.id}
                          >
                            <Trash2 className="size-4 text-red-400" />
                          </Button>
                        </div>
                      </TableCell>
                    </TableRow>
                  );
                })}
              </TableBody>
            </Table>
          </div>

          {/* Mobile cards */}
          <div className="grid gap-3 md:hidden">
            {filtered.map((job) => (
              <JobCard
                key={job.id}
                job={job}
                onRequestRemove={setPending}
                removing={removingId === job.id}
              />
            ))}
          </div>
        </>
      )}

      <ConfirmDialog
        open={pending !== null}
        onOpenChange={(open) => !open && setPending(null)}
        title={pendingActive ? "Cancel and remove job?" : "Remove job?"}
        description={
          pendingActive
            ? "This job is still active. It will be cancelled and removed."
            : "This will remove the job from the list."
        }
        confirmLabel={pendingActive ? "Cancel & remove" : "Remove"}
        destructive
        loading={removingId !== null}
        onConfirm={confirmRemove}
      />
    </div>
  );
}
