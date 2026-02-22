"use client";

import { usePoll } from "@/lib/use-poll";
import { getJobs } from "@/lib/api";
import { JobList } from "@/components/job-list";
import { NewJobDialog } from "@/components/new-job-dialog";

export default function DashboardPage() {
  const { data: jobs, error, refresh } = usePoll(getJobs, 3000);

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h2 className="text-lg font-semibold">Jobs</h2>
        <NewJobDialog onCreated={refresh} />
      </div>
      {error && (
        <p className="text-sm text-red-400">
          Failed to load jobs: {error}
        </p>
      )}
      <JobList jobs={jobs ?? []} />
    </div>
  );
}
