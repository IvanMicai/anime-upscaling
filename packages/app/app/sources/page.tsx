"use client";

import { useCallback } from "react";
import Link from "next/link";
import { usePoll } from "@/lib/use-poll";
import { getSources } from "@/lib/api";
import { SourceCard } from "@/components/source-card";
import { AddSourceDialog } from "@/components/add-source-dialog";

export default function SourcesPage() {
  const { data: sources, error, refresh } = usePoll(getSources, 5000);

  const handleRefresh = useCallback(() => {
    refresh();
  }, [refresh]);

  return (
    <div className="space-y-6">
      <Link href="/" className="text-sm text-blue-400 hover:underline">
        &larr; Back to Jobs
      </Link>
      <div className="flex items-center justify-between">
        <h2 className="text-lg font-semibold">Sources</h2>
        <AddSourceDialog onAdded={handleRefresh} />
      </div>

      {error && (
        <p className="text-sm text-red-400">Failed to load sources: {error}</p>
      )}

      {sources && sources.length === 0 && (
        <p className="text-sm text-muted-foreground">
          No sources configured. Add a source to browse and import files from host directories.
        </p>
      )}

      <div className="space-y-4">
        {(sources ?? []).map((source) => (
          <SourceCard
            key={source.id}
            source={source}
            onDeleted={handleRefresh}
          />
        ))}
      </div>
    </div>
  );
}
