import type { Job } from "@/lib/types";

// The app has two areas: Merge (dual-audio jobs, /merge) and Upscaling (every
// other job type, /). Their settings have nothing in common, so each area has
// its own job list, creation page and job detail URL.

export function isMergeJob(job: Pick<Job, "type">): boolean {
  return job.type === "merge";
}

/** Job detail URL, inside the area the job belongs to. */
export function jobHref(job: Pick<Job, "id" | "type">): string {
  return isMergeJob(job) ? `/merge/${job.id}` : `/jobs/${job.id}`;
}

/** The job list of the area the job belongs to. */
export function jobAreaHref(job: Pick<Job, "type">): string {
  return isMergeJob(job) ? "/merge" : "/";
}
