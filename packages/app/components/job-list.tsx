import Link from "next/link";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { StatusBadge } from "@/components/status-badge";
import { Badge } from "@/components/ui/badge";
import type { Job } from "@/lib/types";

function formatTime(iso: string) {
  return new Date(iso).toLocaleString();
}

export function JobList({ jobs }: { jobs: Job[] }) {
  if (jobs.length === 0) {
    return (
      <div className="flex h-40 items-center justify-center rounded-lg border border-dashed text-muted-foreground">
        No jobs yet. Create one to get started.
      </div>
    );
  }

  const sorted = [...jobs].sort(
    (a, b) =>
      new Date(b.created_at).getTime() - new Date(a.created_at).getTime()
  );

  return (
    <div className="rounded-lg border">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>Type</TableHead>
            <TableHead>Status</TableHead>
            <TableHead className="hidden md:table-cell">Files</TableHead>
            <TableHead className="hidden md:table-cell">Progress</TableHead>
            <TableHead className="hidden sm:table-cell">Created</TableHead>
            <TableHead />
          </TableRow>
        </TableHeader>
        <TableBody>
          {sorted.map((job) => {
            const p = job.progress;
            const done = p.completed + p.failed + p.skipped;
            return (
              <TableRow key={job.id}>
                <TableCell>
                  <Badge variant="secondary" className="font-mono capitalize">
                    {job.type}
                  </Badge>
                </TableCell>
                <TableCell>
                  <StatusBadge status={job.status} />
                </TableCell>
                <TableCell className="hidden md:table-cell text-muted-foreground">
                  {job.files?.length ?? 0} file{(job.files?.length ?? 0) !== 1 ? "s" : ""}
                </TableCell>
                <TableCell className="hidden md:table-cell font-mono text-sm text-muted-foreground">
                  {done}/{p.total}
                </TableCell>
                <TableCell className="hidden sm:table-cell text-sm text-muted-foreground">
                  {formatTime(job.created_at)}
                </TableCell>
                <TableCell className="text-right">
                  <Link
                    href={`/jobs/${job.id}`}
                    className="text-sm text-blue-400 hover:underline"
                  >
                    View
                  </Link>
                </TableCell>
              </TableRow>
            );
          })}
        </TableBody>
      </Table>
    </div>
  );
}
