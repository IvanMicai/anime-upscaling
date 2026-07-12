"use client";

import Link from "next/link";
import { Plus } from "lucide-react";
import { usePoll } from "@/lib/use-poll";
import { getJobs } from "@/lib/api";
import { JobList } from "@/components/job-list";
import { Button } from "@/components/ui/button";

export default function DashboardPage() {
  const { data: jobs, error, refresh } = usePoll(getJobs, 3000);
  const list = jobs ?? [];

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h2 className="text-xl font-bold">
          Jobs{" "}
          <span className="ml-1 text-sm font-normal text-muted-foreground">
            {list.length} total
          </span>
        </h2>
        <Link href="/jobs/new">
          <Button>
            <Plus className="size-4" />
            New Job
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
