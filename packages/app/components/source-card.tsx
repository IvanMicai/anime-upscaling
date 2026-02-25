"use client";

import { useEffect, useState } from "react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Label } from "@/components/ui/label";
import { ScrollArea } from "@/components/ui/scroll-area";
import { deleteSource, getSourceFiles, importFiles } from "@/lib/api";
import { useShiftSelect } from "@/lib/use-shift-select";
import type { Source, SourceFile } from "@/lib/types";

function formatBytes(bytes: number): string {
  if (bytes === 0) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB"];
  const i = Math.floor(Math.log(bytes) / Math.log(1024));
  const value = bytes / Math.pow(1024, i);
  return `${value.toFixed(i > 0 ? 1 : 0)} ${units[i]}`;
}

interface SourceCardProps {
  source: Source;
  onDeleted: () => void;
}

export function SourceCard({ source, onDeleted }: SourceCardProps) {
  const [expanded, setExpanded] = useState(false);
  const [files, setFiles] = useState<SourceFile[]>([]);
  const [loading, setLoading] = useState(false);
  const [selected, setSelected] = useState<string[]>([]);
  const { handleToggle, resetLastClicked } = useShiftSelect(selected, setSelected);
  const [importing, setImporting] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [message, setMessage] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!expanded) return;
    setLoading(true);
    setError(null);
    resetLastClicked();
    getSourceFiles(source.id)
      .then((res) => setFiles(res.files ?? []))
      .catch((err) => setError(err instanceof Error ? err.message : "Failed to load files"))
      .finally(() => setLoading(false));
  }, [expanded, source.id]);

  const allSelected = files.length > 0 && selected.length === files.length;

  function toggleAll() {
    setSelected(allSelected ? [] : files.map((f) => f.name));
  }

  async function handleImport() {
    setImporting(true);
    setMessage(null);
    setError(null);
    try {
      const res = await importFiles(source.id, selected);
      setMessage(`Imported ${res.copied} file${res.copied !== 1 ? "s" : ""} to input/`);
      setSelected([]);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Import failed");
    } finally {
      setImporting(false);
    }
  }

  async function handleDelete() {
    setDeleting(true);
    try {
      await deleteSource(source.id);
      onDeleted();
    } catch {
      setDeleting(false);
    }
  }

  return (
    <Card>
      <CardHeader
        className="flex flex-row items-center justify-between gap-4 cursor-pointer"
        onClick={() => setExpanded(!expanded)}
      >
        <div className="min-w-0">
          <CardTitle className="text-sm flex items-center gap-2">
            <span>{expanded ? "\u25BC" : "\u25B6"}</span>
            {source.name}
          </CardTitle>
          <p className="text-xs text-muted-foreground font-mono truncate mt-1">
            {source.path}
          </p>
        </div>
        <Button
          variant="ghost"
          size="sm"
          className="text-red-400 hover:text-red-300 shrink-0"
          onClick={(e) => {
            e.stopPropagation();
            handleDelete();
          }}
          disabled={deleting}
        >
          {deleting ? "..." : "Remove"}
        </Button>
      </CardHeader>

      {expanded && (
        <CardContent className="space-y-3">
          {loading && (
            <p className="text-sm text-muted-foreground">Loading files...</p>
          )}

          {error && <p className="text-sm text-red-400">{error}</p>}

          {!loading && !error && files.length === 0 && (
            <p className="text-sm text-muted-foreground">No video files found.</p>
          )}

          {!loading && files.length > 0 && (
            <>
              <div className="flex items-center gap-2">
                <Checkbox
                  id={`select-all-${source.id}`}
                  checked={allSelected}
                  onCheckedChange={toggleAll}
                />
                <Label
                  htmlFor={`select-all-${source.id}`}
                  className="text-sm font-medium"
                >
                  Select All ({files.length} files)
                </Label>
              </div>

              <ScrollArea className="rounded-md border p-2">
                <div className="space-y-1.5">
                  {(() => {
                    const sorted = [...files].sort((a, b) => a.name.localeCompare(b.name));
                    const sortedNames = sorted.map((f) => f.name);
                    return sorted.map((file, index) => (
                    <div
                      key={file.name}
                      className="flex items-center gap-2 cursor-pointer select-none"
                      onClick={(e) => handleToggle(index, sortedNames, e.shiftKey)}
                    >
                      <Checkbox
                        checked={selected.includes(file.name)}
                        tabIndex={-1}
                        className="pointer-events-none"
                      />
                      <span className="font-mono text-sm truncate">
                        {file.name}
                      </span>
                      {file.in_input && (
                        <span className="w-2.5 h-2.5 rounded-full bg-yellow-500 shrink-0" />
                      )}
                      {file.in_output && (
                        <span className="w-2.5 h-2.5 rounded-full bg-blue-500 shrink-0" />
                      )}
                      {file.in_optimized && (
                        <span className="w-2.5 h-2.5 rounded-full bg-green-500 shrink-0" />
                      )}
                      <span className="text-xs text-muted-foreground ml-auto shrink-0">
                        {formatBytes(file.size)}
                      </span>
                    </div>
                  ));
                  })()}
                </div>
              </ScrollArea>

              {files.some((f) => f.in_input || f.in_output || f.in_optimized) && (
                <div className="flex items-center gap-4 text-xs text-muted-foreground">
                  <span className="flex items-center gap-1.5">
                    <span className="w-2.5 h-2.5 rounded-full bg-yellow-500" />
                    Imported
                  </span>
                  <span className="flex items-center gap-1.5">
                    <span className="w-2.5 h-2.5 rounded-full bg-blue-500" />
                    Upscaled
                  </span>
                  <span className="flex items-center gap-1.5">
                    <span className="w-2.5 h-2.5 rounded-full bg-green-500" />
                    Optimized
                  </span>
                </div>
              )}

              <div className="flex items-center gap-3">
                <Button
                  size="sm"
                  onClick={handleImport}
                  disabled={importing || selected.length === 0}
                >
                  {importing
                    ? "Importing..."
                    : `Import Selected (${selected.length})`}
                </Button>
                {message && (
                  <span className="text-sm text-green-400">{message}</span>
                )}
              </div>
            </>
          )}
        </CardContent>
      )}
    </Card>
  );
}
