"use client";

import { useParams } from "next/navigation";
import { useCallback } from "react";
import Link from "next/link";
import { ArrowLeft } from "lucide-react";
import { usePoll } from "@/lib/use-poll";
import { useLogStream } from "@/lib/use-log-stream";
import { getJob } from "@/lib/api";
import { JobHeader } from "@/components/job-header";
import { ProgressBar } from "@/components/progress-bar";
import { WorkerGauge } from "@/components/worker-gauge";
import { LogViewer } from "@/components/log-viewer";

export default function JobDetailPage() {
  const { id } = useParams<{ id: string }>();
  const fetcher = useCallback(() => getJob(id), [id]);
  const { data: job, error, refresh } = usePoll(fetcher, 2000);
  const { logs, connected } = useLogStream(id);

  if (error) {
    return (
      <div className="space-y-4">
        <Link
          href="/"
          className="inline-flex items-center gap-1 text-sm text-muted-foreground hover:text-foreground"
        >
          <ArrowLeft className="size-4" />
          Back to Jobs
        </Link>
        <p className="text-red-400">Error: {error}</p>
      </div>
    );
  }

  if (!job) {
    return <p className="text-muted-foreground">Loading...</p>;
  }

  const workers = Object.entries(job.progress.containers ?? {}).filter(
    ([, c]) => c && (c.frame > 0 || c.phase || c.elapsed),
  );

  return (
    <div className="space-y-4">
      <Link
        href="/"
        className="inline-flex items-center gap-1 text-sm text-muted-foreground hover:text-foreground"
      >
        <ArrowLeft className="size-4" />
        Back to Jobs
      </Link>
      <JobHeader job={job} onCancelled={refresh} />
      <ProgressBar progress={job.progress} />
      {workers.length > 0 && (
        <div className="divide-y divide-border sm:grid sm:grid-cols-2 sm:gap-3 sm:divide-y-0">
          {workers.map(([source, c]) => (
            <WorkerGauge key={source} source={source} c={c!} />
          ))}
        </div>
      )}
      <LogViewer logs={logs} connected={connected} />
    </div>
  );
}
