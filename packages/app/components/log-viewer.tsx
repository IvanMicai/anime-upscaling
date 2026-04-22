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

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [filtered.length]);

  return (
    <div className="space-y-2">
      <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
        <Tabs
          value={filter}
          onValueChange={(v) => setFilter(v as Filter)}
        >
          <TabsList>
            <TabsTrigger value={ALL_FILTER}>All</TabsTrigger>
            {sources.map((s) => (
              <TabsTrigger key={s} value={s}>
                {s}
              </TabsTrigger>
            ))}
          </TabsList>
        </Tabs>
        <span className="text-xs text-muted-foreground">
          {connected ? "streaming" : "disconnected"} &middot; {filtered.length}{" "}
          lines
        </span>
      </div>

      <ScrollArea className="h-[300px] sm:h-[500px] rounded-md border bg-black/40 p-2 sm:p-3" ref={scrollRef}>
        <div className="space-y-0.5 font-mono text-xs">
          {filtered.map((entry, i) => (
            <div key={i} className="flex flex-wrap items-start gap-x-2 gap-y-0.5">
              <span className="shrink-0 text-muted-foreground/60 hidden sm:inline">
                {formatTimestamp(entry.time)}
              </span>
              <Badge
                variant="outline"
                className={`shrink-0 px-1.5 py-0 text-[10px] ${sourceColorSet(entry.source).badge}`}
              >
                {entry.source}
              </Badge>
              <span
                className={`shrink-0 w-8 text-center ${levelColor[entry.level] ?? ""}`}
              >
                {entry.level}
              </span>
              <span className="break-all">{entry.message}</span>
            </div>
          ))}
          <div ref={bottomRef} />
        </div>
      </ScrollArea>
    </div>
  );
}
