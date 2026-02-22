"use client";

import { useState, useEffect, useRef } from "react";
import type { LogEntry } from "./types";

export function useLogStream(jobId: string) {
  const [logs, setLogs] = useState<LogEntry[]>([]);
  const [connected, setConnected] = useState(false);
  const eventSourceRef = useRef<EventSource | null>(null);

  useEffect(() => {
    const es = new EventSource(`/api/jobs/${jobId}/logs`);
    eventSourceRef.current = es;

    es.onopen = () => setConnected(true);

    es.onmessage = (event) => {
      try {
        const entry: LogEntry = JSON.parse(event.data);
        setLogs((prev) => [...prev, entry]);
      } catch {
        // ignore non-JSON messages
      }
    };

    es.onerror = () => {
      setConnected(false);
      es.close();
    };

    return () => {
      es.close();
      eventSourceRef.current = null;
    };
  }, [jobId]);

  return { logs, connected };
}
