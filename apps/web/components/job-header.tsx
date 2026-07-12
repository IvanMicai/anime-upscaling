"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { Trash2, X } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { StatusBadge } from "@/components/status-badge";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { cancelJob, deleteJob } from "@/lib/api";
import { formatRelativeTime, jobTypeLabel } from "@/lib/format";
import { sectionCard } from "@/lib/section";
import type { Job } from "@/lib/types";

interface JobHeaderProps {
  job: Job;
  onCancelled: () => void;
}

export function JobHeader({ job, onCancelled }: JobHeaderProps) {
  const router = useRouter();
  const [cancelling, setCancelling] = useState(false);
  const [removing, setRemoving] = useState(false);
  const [confirmOpen, setConfirmOpen] = useState(false);
  const [cancelConfirmOpen, setCancelConfirmOpen] = useState(false);

  const active = job.status === "running" || job.status === "queued";

  async function handleCancel() {
    setCancelling(true);
    try {
      await cancelJob(job.id);
      onCancelled();
    } catch {
      // ignore — poll will catch up
    } finally {
      setCancelling(false);
      setCancelConfirmOpen(false);
    }
  }

  async function handleRemove() {
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
    <div className={sectionCard}>
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between sm:gap-4">
        <div className="flex flex-wrap items-center gap-2">
          <span className="font-mono text-sm font-semibold">{job.id}</span>
          <Badge variant="secondary" className="capitalize">
            {jobTypeLabel(job)}
          </Badge>
          <StatusBadge status={job.status} />
          <span className="text-xs text-muted-foreground">
            Created {formatRelativeTime(job.created_at)}
            {job.finished_at &&
              ` · Finished ${formatRelativeTime(job.finished_at)}`}
          </span>
        </div>
        <div className="flex items-center gap-2">
          {active && (
            <Button
              variant="destructive"
              size="sm"
              onClick={() => setCancelConfirmOpen(true)}
              disabled={cancelling}
            >
              <X className="size-4" />
              {cancelling ? "Cancelling..." : "Cancel"}
            </Button>
          )}
          <Button
            variant="outline"
            size="sm"
            onClick={() => setConfirmOpen(true)}
            disabled={removing}
          >
            <Trash2 className="size-4" />
            {removing ? "Removing..." : "Remove"}
          </Button>
        </div>
      </div>

      <ConfirmDialog
        open={cancelConfirmOpen}
        onOpenChange={setCancelConfirmOpen}
        title="Cancel job?"
        description="This job is still active. Cancelling stops processing; progress on unfinished files will be lost."
        confirmLabel="Cancel job"
        cancelLabel="Keep running"
        destructive
        loading={cancelling}
        onConfirm={handleCancel}
      />

      <ConfirmDialog
        open={confirmOpen}
        onOpenChange={setConfirmOpen}
        title={active ? "Cancel and remove job?" : "Remove job?"}
        description={
          active
            ? "This job is still active. It will be cancelled and removed."
            : "This will remove the job from the list."
        }
        confirmLabel={active ? "Cancel & remove" : "Remove"}
        destructive
        loading={removing}
        onConfirm={handleRemove}
      />
    </div>
  );
}
