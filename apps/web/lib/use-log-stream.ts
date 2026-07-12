"use client";

import { useEffect, useRef, useState } from "react";
import type { LogEntry } from "./types";

interface LogsResponse {
  entries: LogEntry[];
  total: number;
  running: boolean;
}

const POLL_INTERVAL_MS = 1500;
const STALE_THRESHOLD_MS = 4000;

export function useLogStream(jobId: string) {
  const [logs, setLogs] = useState<LogEntry[]>([]);
  const [connected, setConnected] = useState(false);
  const cursorRef = useRef(0);
  const lastSuccessRef = useRef(0);

  // Reset stream state when the job changes, during render (not in the effect)
  // to avoid an extra commit. See react.dev "Adjusting some state when a prop
  // changes".
  const [prevJobId, setPrevJobId] = useState(jobId);
  if (prevJobId !== jobId) {
    setPrevJobId(jobId);
    setLogs([]);
    setConnected(false);
  }

  useEffect(() => {
    let cancelled = false;
    let timer: ReturnType<typeof setTimeout> | null = null;

    cursorRef.current = 0;
    lastSuccessRef.current = 0;

    async function poll() {
      try {
        const res = await fetch(
          `/api/jobs/${jobId}/logs?since=${cursorRef.current}`,
          { cache: "no-store" },
        );
        if (!res.ok) throw new Error(`HTTP ${res.status}`);
        const body: LogsResponse = await res.json();
        if (cancelled) return;

        if (body.entries.length > 0) {
          setLogs((prev) => [...prev, ...body.entries]);
        }
        cursorRef.current = body.total;
        lastSuccessRef.current = Date.now();
        setConnected(true);

        if (body.running) {
          timer = setTimeout(poll, POLL_INTERVAL_MS);
        }
      } catch {
        if (cancelled) return;
        const stale = Date.now() - lastSuccessRef.current > STALE_THRESHOLD_MS;
        if (stale) setConnected(false);
        timer = setTimeout(poll, POLL_INTERVAL_MS);
      }
    }

    poll();

    return () => {
      cancelled = true;
      if (timer) clearTimeout(timer);
    };
  }, [jobId]);

  return { logs, connected };
}
