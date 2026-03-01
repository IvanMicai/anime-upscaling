"use client";

import { useEffect, useState } from "react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Checkbox } from "@/components/ui/checkbox";
import { Label } from "@/components/ui/label";
import { ScrollArea } from "@/components/ui/scroll-area";
import { cn } from "@/lib/utils";
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

function formatCacheAge(iso: string): string {
  const diff = Math.floor((Date.now() - new Date(iso).getTime()) / 1000);
  if (diff < 10) return "just now";
  if (diff < 60) return `${diff}s ago`;
  const mins = Math.floor(diff / 60);
  return `${mins}m ago`;
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
  const [filters, setFilters] = useState<Set<string>>(new Set());
  const { handleToggle, resetLastClicked } = useShiftSelect(selected, setSelected);
  const [importing, setImporting] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [message, setMessage] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [cachedAt, setCachedAt] = useState<string | null>(null);
  const [refreshing, setRefreshing] = useState(false);

  useEffect(() => {
    if (!expanded) return;
    setLoading(true);
    setError(null);
    resetLastClicked();
    setFilters(new Set());
    getSourceFiles(source.id)
      .then((res) => {
        setFiles(res.files ?? []);
        setCachedAt(res.cached_at ?? null);
      })
      .catch((err) => setError(err instanceof Error ? err.message : "Failed to load files"))
      .finally(() => setLoading(false));
  }, [expanded, source.id]);

  function handleRefresh() {
    setRefreshing(true);
    getSourceFiles(source.id, true)
      .then((res) => {
        setFiles(res.files ?? []);
        setCachedAt(res.cached_at ?? null);
      })
      .catch(() => {})
      .finally(() => setRefreshing(false));
  }

  function toggleFilter(key: string) {
    setFilters((prev) => {
      const next = new Set(prev);
      if (next.has(key)) next.delete(key);
      else next.add(key);
      return next;
    });
    setSelected([]);
    resetLastClicked();
  }

  function matchesFilter(file: SourceFile): boolean {
    if (filters.size === 0) return true;
    if (filters.has("imported") && file.in_input) return true;
    if (filters.has("upscaled") && file.in_output) return true;
    if (filters.has("optimized") && file.in_optimized) return true;
    return false;
  }

  const sorted = [...files].sort((a, b) => a.name.localeCompare(b.name));
  const filtered = sorted.filter(matchesFilter);
  const filteredNames = filtered.map((f) => f.name);

  const allSelected = filtered.length > 0 && filtered.every((f) => selected.includes(f.name));

  function toggleAll() {
    setSelected(allSelected ? [] : [...filteredNames]);
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

  const hasStatus = files.some((f) => f.in_input || f.in_output || f.in_optimized);

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
                  Select All ({filtered.length} files)
                </Label>
                {cachedAt && (
                  <span className="ml-auto flex items-center gap-1.5 text-xs text-muted-foreground">
                    Cached {formatCacheAge(cachedAt)}
                    <button
                      type="button"
                      onClick={handleRefresh}
                      disabled={refreshing}
                      className="hover:text-foreground transition-colors disabled:opacity-50"
                    >
                      {refreshing ? "..." : "Refresh"}
                    </button>
                  </span>
                )}
              </div>

              {hasStatus && (
                <div className="flex items-center gap-2">
                  <button
                    type="button"
                    onClick={() => toggleFilter("imported")}
                    className={cn(
                      "px-2.5 py-0.5 rounded-full text-xs font-medium border transition-colors",
                      filters.has("imported")
                        ? "bg-yellow-500/20 text-yellow-400 border-yellow-500/30"
                        : "bg-transparent text-muted-foreground border-border"
                    )}
                  >
                    Imported
                  </button>
                  <button
                    type="button"
                    onClick={() => toggleFilter("upscaled")}
                    className={cn(
                      "px-2.5 py-0.5 rounded-full text-xs font-medium border transition-colors",
                      filters.has("upscaled")
                        ? "bg-blue-500/20 text-blue-400 border-blue-500/30"
                        : "bg-transparent text-muted-foreground border-border"
                    )}
                  >
                    Upscaled
                  </button>
                  <button
                    type="button"
                    onClick={() => toggleFilter("optimized")}
                    className={cn(
                      "px-2.5 py-0.5 rounded-full text-xs font-medium border transition-colors",
                      filters.has("optimized")
                        ? "bg-green-500/20 text-green-400 border-green-500/30"
                        : "bg-transparent text-muted-foreground border-border"
                    )}
                  >
                    Optimized
                  </button>
                </div>
              )}

              <ScrollArea className="rounded-md border p-2">
                <div className="space-y-1.5">
                  {filtered.map((file, index) => (
                    <div
                      key={file.name}
                      className="flex items-center gap-2 cursor-pointer select-none"
                      onClick={(e) => handleToggle(index, filteredNames, e.shiftKey)}
                    >
                      <Checkbox
                        checked={selected.includes(file.name)}
                        tabIndex={-1}
                        className="pointer-events-none"
                      />
                      <span className="font-mono text-sm truncate">
                        {file.name}
                      </span>
                      <div className="flex items-center gap-1.5 ml-auto shrink-0">
                        {file.in_input && (
                          <Badge className="bg-yellow-500/20 text-yellow-400 border-yellow-500/30 hover:bg-yellow-500/20">
                            {file.input_width && file.input_height
                              ? `${file.input_width}x${file.input_height} @ `
                              : ""}{formatBytes(file.input_size ?? 0)}
                          </Badge>
                        )}
                        {file.in_output && (
                          <Badge className="bg-blue-500/20 text-blue-400 border-blue-500/30 hover:bg-blue-500/20">
                            {file.upscaled_width && file.upscaled_height
                              ? `${file.upscaled_width}x${file.upscaled_height} @ `
                              : ""}{formatBytes(file.output_size ?? 0)}
                          </Badge>
                        )}
                        {file.in_optimized && (
                          <Badge className="bg-green-500/20 text-green-400 border-green-500/30 hover:bg-green-500/20">
                            {file.optimized_width && file.optimized_height
                              ? `${file.optimized_width}x${file.optimized_height} @ `
                              : ""}{formatBytes(file.optimized_size ?? 0)}
                          </Badge>
                        )}
                        <span className="text-xs text-muted-foreground">
                          {file.width && file.height
                            ? `${file.width}x${file.height} `
                            : ""}{formatBytes(file.size)}
                        </span>
                      </div>
                    </div>
                  ))}
                </div>
              </ScrollArea>

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
