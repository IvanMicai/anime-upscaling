"use client";

import Link from "next/link";
import { Combine, Maximize2 } from "lucide-react";
import { usePoll } from "@/lib/use-poll";
import { getJobs } from "@/lib/api";
import { JobList } from "@/components/job-list";
import { Button } from "@/components/ui/button";

export default function JobsPage() {
  const { data: jobs, error, refresh } = usePoll(getJobs, 3000);
  const list = jobs ?? [];

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h2 className="text-xl font-bold">
          Jobs{" "}
          <span className="ml-1 text-sm font-normal text-muted-foreground">
            {list.length} total
          </span>
        </h2>
        <div className="flex items-center gap-2">
          <Button asChild variant="outline">
            <Link href="/merge">
              <Combine className="size-4" />
              New Merge
            </Link>
          </Button>
          <Button asChild>
            <Link href="/upscaling">
              <Maximize2 className="size-4" />
              New Upscaling
            </Link>
          </Button>
        </div>
      </div>
      {error && (
        <p className="text-sm text-red-400">Failed to load jobs: {error}</p>
      )}
      <JobList jobs={list} onRemove={refresh} />
    </div>
  );
}
