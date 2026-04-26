"use client";

import Link from "next/link";
import { usePoll } from "@/lib/use-poll";
import { getJobs } from "@/lib/api";
import { JobList } from "@/components/job-list";
import { Button } from "@/components/ui/button";

export default function DashboardPage() {
  const { data: jobs, error, refresh } = usePoll(getJobs, 3000);

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h2 className="text-lg font-semibold">Jobs</h2>
        <Link href="/jobs/new">
          <Button>New Job</Button>
        </Link>
      </div>
      {error && (
        <p className="text-sm text-red-400">
          Failed to load jobs: {error}
        </p>
      )}
      <JobList jobs={jobs ?? []} onRemove={refresh} />
    </div>
  );
}
