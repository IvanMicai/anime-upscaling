import { Badge } from "@/components/ui/badge";
import type { JobStatus } from "@/lib/types";

const config: Record<JobStatus, { label: string; className: string }> = {
  queued: {
    label: "Queued",
    className: "bg-purple-500/20 text-purple-400 border-purple-500/30",
  },
  running: {
    label: "Running",
    className: "bg-blue-500/20 text-blue-400 border-blue-500/30",
  },
  completed: {
    label: "Completed",
    className: "bg-green-500/20 text-green-400 border-green-500/30",
  },
  failed: {
    label: "Failed",
    className: "bg-red-500/20 text-red-400 border-red-500/30",
  },
  cancelled: {
    label: "Cancelled",
    className: "bg-yellow-500/20 text-yellow-400 border-yellow-500/30",
  },
};

export function StatusBadge({ status }: { status: JobStatus }) {
  const c = config[status];
  return (
    <Badge variant="outline" className={c.className}>
      {c.label}
    </Badge>
  );
}
