"use client";

import Link from "next/link";
import { Plus } from "lucide-react";
import { usePoll } from "@/lib/use-poll";
import { getJobs } from "@/lib/api";
import { JobList } from "@/components/job-list";
import { Button } from "@/components/ui/button";
import type { Job } from "@/lib/types";

/** The job list of one area (Upscaling or Merge), with its "new job" button. */
export function JobsDashboard({
  title,
  include,
  newHref,
  newLabel,
}: {
  title: string;
  include: (job: Job) => boolean;
  newHref: string;
  newLabel: string;
}) {
  const { data: jobs, error, refresh } = usePoll(getJobs, 3000);
  const list = (jobs ?? []).filter(include);

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h2 className="text-xl font-bold">
          {title}{" "}
          <span className="ml-1 text-sm font-normal text-muted-foreground">
            {list.length} total
          </span>
        </h2>
        <Link href={newHref}>
          <Button>
            <Plus className="size-4" />
            {newLabel}
          </Button>
        </Link>
      </div>
      {error && (
        <p className="text-sm text-red-400">Failed to load jobs: {error}</p>
      )}
      <JobList jobs={list} onRemove={refresh} />
    </div>
  );
}
