"use client";

import { useEffect, useRef, useState } from "react";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Badge } from "@/components/ui/badge";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import type { LogEntry, LogSource, LogLevel } from "@/lib/types";
import { sourceColorSet } from "@/lib/source-color";

const levelColor: Record<LogLevel, string> = {
  INFO: "text-muted-foreground",
  OK: "text-green-400",
  ERRO: "text-red-400",
  SKIP: "text-yellow-400",
  WARN: "text-yellow-400",
  STEP: "text-green-400",
};

function formatTimestamp(iso: string) {
  return new Date(iso).toLocaleTimeString();
}

const STEP_PREFIX = /^\[(\d+)\/(\d+)\]\s+/;

function parseStep(message: string): { step: string | null; text: string } {
  const m = message.match(STEP_PREFIX);
  if (!m) return { step: null, text: message };
  return { step: `${m[1]}/${m[2]}`, text: message.slice(m[0].length) };
}

const ALL_FILTER = "ALL";
type Filter = typeof ALL_FILTER | LogSource;

export function LogViewer({
  logs,
  connected,
}: {
  logs: LogEntry[];
  connected: boolean;
}) {
  const [filter, setFilter] = useState<Filter>(ALL_FILTER);
  const bottomRef = useRef<HTMLDivElement>(null);
  const scrollRef = useRef<HTMLDivElement>(null);

  const filtered =
    filter === ALL_FILTER ? logs : logs.filter((l) => l.source === filter);

  // Determine which sources exist for tabs
  const sources = Array.from(new Set(logs.map((l) => l.source)));

  // Only show the step column when at least one entry carries a [N/M] prefix.
  const hasAnyStep = filtered.some((e) => STEP_PREFIX.test(e.message));
  const gridCols = hasAnyStep
    ? "grid-cols-[96px_44px_44px_minmax(0,1fr)] sm:grid-cols-[88px_104px_44px_44px_minmax(0,1fr)]"
    : "grid-cols-[96px_44px_minmax(0,1fr)] sm:grid-cols-[88px_104px_44px_minmax(0,1fr)]";

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [filtered.length]);

  return (
    <div className="space-y-2">
      <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
        <div className="min-w-0 max-w-full overflow-x-auto overflow-y-hidden scrollbar-dark">
          <Tabs
            value={filter}
            onValueChange={(v) => setFilter(v as Filter)}
          >
            <TabsList className="w-max">
              <TabsTrigger value={ALL_FILTER}>All</TabsTrigger>
              {sources.map((s) => (
                <TabsTrigger key={s} value={s}>
                  {s}
                </TabsTrigger>
              ))}
            </TabsList>
          </Tabs>
        </div>
        <span className="shrink-0 text-xs text-muted-foreground">
          {connected ? "streaming" : "disconnected"} &middot; {filtered.length}{" "}
          lines
        </span>
      </div>

      <ScrollArea className="h-[300px] sm:h-[500px] rounded-md border bg-black/40 p-2 sm:p-3" ref={scrollRef}>
        <div className="font-mono text-xs">
          {filtered.map((entry, i) => {
            const { step, text } = parseStep(entry.message);
            return (
              <div
                key={i}
                className={`grid items-start gap-x-2 gap-y-0.5 px-1 py-0.5 hover:bg-white/5 ${gridCols}`}
              >
                <span className="hidden sm:block whitespace-nowrap text-muted-foreground/60 tabular-nums">
                  {formatTimestamp(entry.time)}
                </span>
                <Badge
                  variant="outline"
                  className={`justify-start px-1.5 py-0 text-[10px] truncate ${sourceColorSet(entry.source).badge}`}
                >
                  {entry.source}
                </Badge>
                <span
                  className={`text-center ${levelColor[entry.level] ?? ""}`}
                >
                  {entry.level}
                </span>
                {hasAnyStep && (
                  <span className="text-center text-muted-foreground/70 tabular-nums">
                    {step ?? ""}
                  </span>
                )}
                <span className="break-all">{text}</span>
              </div>
            );
          })}
          <div ref={bottomRef} />
        </div>
      </ScrollArea>
    </div>
  );
}
