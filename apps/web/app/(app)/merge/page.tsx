"use client";

import { JobsDashboard } from "@/components/jobs-dashboard";
import { isMergeJob } from "@/lib/job-routes";

export default function MergeJobsPage() {
  return (
    <JobsDashboard
      title="Merge"
      include={isMergeJob}
      newHref="/merge/new"
      newLabel="New Merge"
    />
  );
}
