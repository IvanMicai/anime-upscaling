"use client";

import { useParams } from "next/navigation";
import { useCallback } from "react";
import { usePoll } from "@/lib/use-poll";
import { useLogStream } from "@/lib/use-log-stream";
import { getJob } from "@/lib/api";
import { JobHeader } from "@/components/job-header";
import { ProgressBar } from "@/components/progress-bar";
import { LogViewer } from "@/components/log-viewer";
import Link from "next/link";

export default function JobDetailPage() {
  const { id } = useParams<{ id: string }>();
  const fetcher = useCallback(() => getJob(id), [id]);
  const { data: job, error, refresh } = usePoll(fetcher, 2000);
  const { logs, connected } = useLogStream(id);

  if (error) {
    return (
      <div className="space-y-4">
        <Link href="/" className="text-sm text-blue-400 hover:underline">
          &larr; Back
        </Link>
        <p className="text-red-400">Error: {error}</p>
      </div>
    );
  }

  if (!job) {
    return <p className="text-muted-foreground">Loading...</p>;
  }

  return (
    <div className="space-y-6">
      <Link href="/" className="text-sm text-blue-400 hover:underline">
        &larr; Back to Jobs
      </Link>
      <JobHeader job={job} onCancelled={refresh} />
      <ProgressBar progress={job.progress} />
      <LogViewer logs={logs} connected={connected} />
    </div>
  );
}
