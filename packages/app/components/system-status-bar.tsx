"use client";

import { Activity, Layers, Server } from "lucide-react";
import { usePoll } from "@/lib/use-poll";
import { getSystemStatus } from "@/lib/api";
import { formatUptime } from "@/lib/format";
import { cn } from "@/lib/utils";
import type { GPUMetric } from "@/lib/types";

function loadBarColor(pct: number): string {
  if (pct >= 85) return "bg-red-500";
  if (pct >= 60) return "bg-amber-500";
  return "bg-emerald-500";
}

function tempColor(c: number): string {
  if (c >= 83) return "text-red-400";
  if (c >= 75) return "text-amber-400";
  return "text-muted-foreground";
}

function Divider() {
  return <span className="hidden h-7 w-px shrink-0 bg-border sm:block" />;
}

function GpuStat({ gpu }: { gpu: GPUMetric }) {
  return (
    <div className="flex items-center gap-2">
      <span className="font-mono text-xs font-medium text-muted-foreground">
        GPU {gpu.index}
      </span>
      <span className="h-1.5 w-16 overflow-hidden rounded-full bg-muted">
        <span
          className={cn("block h-full rounded-full", loadBarColor(gpu.utilization))}
          style={{ width: `${Math.min(100, Math.max(0, gpu.utilization))}%` }}
        />
      </span>
      <span className="font-mono text-xs tabular-nums">
        {gpu.utilization}%{" "}
        <span className={tempColor(gpu.temperature)}>· {gpu.temperature}°C</span>
      </span>
    </div>
  );
}

export function SystemStatusBar() {
  const { data, error } = usePoll(getSystemStatus, 3000);

  const online = !error && (data?.online ?? false);
  const dotColor = error
    ? "bg-red-500"
    : data?.gpu_healthy === false
      ? "bg-amber-500"
      : online
        ? "bg-emerald-500"
        : "bg-muted-foreground";

  const host = data?.hostname || "server";
  const fps = data?.throughput.fps ?? 0;
  const gpus = data?.gpus ?? [];

  return (
    <div className="mb-3 overflow-x-auto rounded-xl border bg-card/50 px-3 py-2 text-sm">
      {/* Desktop / tablet */}
      <div className="hidden items-center gap-4 sm:flex">
        <div className="flex items-center gap-2">
          <span className={cn("size-2 shrink-0 rounded-full", dotColor)} />
          <Server className="size-4 text-muted-foreground" />
          <span className="font-mono font-medium">{host}</span>
          <span className="text-xs text-muted-foreground">
            {error
              ? "offline"
              : `online · ${formatUptime(data?.uptime_seconds ?? 0)}`}
          </span>
        </div>

        {gpus.map((gpu) => (
          <div key={gpu.index} className="flex items-center gap-4">
            <Divider />
            <GpuStat gpu={gpu} />
          </div>
        ))}

        <Divider />
        <div className="flex items-center gap-2 text-muted-foreground">
          <Layers className="size-4" />
          <span className="font-mono text-xs">
            Queue <span className="text-foreground">{data?.queue.queued ?? 0}</span>{" "}
            · Run <span className="text-foreground">{data?.queue.running ?? 0}</span>
          </span>
        </div>

        <Divider />
        <div className="flex items-center gap-2 text-muted-foreground">
          <Activity className="size-4" />
          <span className="font-mono text-xs tabular-nums">
            <span className="text-foreground">{fps.toFixed(1)}</span> fps
          </span>
        </div>
      </div>

      {/* Mobile compact strip */}
      <div className="flex items-center gap-3 whitespace-nowrap font-mono text-xs sm:hidden">
        <span className="flex items-center gap-1.5">
          <span className={cn("size-2 rounded-full", dotColor)} />
          {error ? "offline" : "online"}
        </span>
        {gpus.map((gpu) => (
          <span key={gpu.index} className="text-muted-foreground">
            G{gpu.index} <span className="text-foreground">{gpu.utilization}%</span>
          </span>
        ))}
        <span className="text-muted-foreground">
          Q<span className="text-foreground">{data?.queue.queued ?? 0}</span>·R
          <span className="text-foreground">{data?.queue.running ?? 0}</span>
        </span>
        <span className="tabular-nums">
          <span className="text-foreground">{fps.toFixed(1)}</span> fps
        </span>
      </div>
    </div>
  );
}
