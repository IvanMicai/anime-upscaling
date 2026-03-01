"use client";

import { useEffect, useState } from "react";
import { Badge } from "@/components/ui/badge";
import { Checkbox } from "@/components/ui/checkbox";
import { Label } from "@/components/ui/label";
import { ScrollArea } from "@/components/ui/scroll-area";
import { cn } from "@/lib/utils";
import { getFiles } from "@/lib/api";
import { useShiftSelect } from "@/lib/use-shift-select";
import type { VideoFile } from "@/lib/types";

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

interface FilePickerProps {
  selected: string[];
  onChange: (files: string[]) => void;
  dir?: string;
}

export function FilePicker({ selected, onChange, dir = "input" }: FilePickerProps) {
  const [files, setFiles] = useState<VideoFile[]>([]);
  const [loading, setLoading] = useState(true);
  const [filters, setFilters] = useState<Set<string>>(new Set());
  const [cachedAt, setCachedAt] = useState<string | null>(null);
  const [refreshing, setRefreshing] = useState(false);
  const { handleToggle, resetLastClicked } = useShiftSelect(selected, onChange);

  useEffect(() => {
    setLoading(true);
    onChange([]);
    resetLastClicked();
    setFilters(new Set());
    getFiles(dir)
      .then((res) => {
        setFiles(res.files ?? []);
        setCachedAt(res.cached_at ?? null);
      })
      .catch(() => setFiles([]))
      .finally(() => setLoading(false));
  }, [dir]);

  function handleRefresh() {
    setRefreshing(true);
    getFiles(dir, true)
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
    onChange([]);
    resetLastClicked();
  }

  function matchesFilter(file: VideoFile): boolean {
    if (filters.size === 0) return true;
    if (filters.has("upscaled") && file.has_upscaled) return true;
    if (filters.has("optimized") && file.has_optimized) return true;
    if (filters.has("input") && file.has_input) return true;
    return false;
  }

  const sorted = [...files].sort((a, b) => a.name.localeCompare(b.name));
  const filtered = sorted.filter(matchesFilter);
  const filteredNames = filtered.map((f) => f.name);

  const allSelected = filtered.length > 0 && filtered.every((f) => selected.includes(f.name));

  const selectedTotal = files
    .filter((f) => selected.includes(f.name))
    .reduce((sum, f) => sum + f.size, 0);

  function toggleAll() {
    onChange(allSelected ? [] : [...filteredNames]);
  }

  if (loading) {
    return <p className="text-sm text-muted-foreground">Loading files...</p>;
  }

  if (files.length === 0) {
    return <p className="text-sm text-muted-foreground">No files found in {dir}/.</p>;
  }

  const hasStatus = files.some((f) => f.has_upscaled || f.has_optimized || f.has_input);

  return (
    <div className="flex flex-col h-full gap-2">
      <div className="flex items-center gap-2">
        <Checkbox
          id="select-all"
          checked={allSelected}
          onCheckedChange={toggleAll}
        />
        <Label htmlFor="select-all" className="text-sm font-medium">
          Select All ({filtered.length} files{selected.length > 0 ? `, ${formatBytes(selectedTotal)}` : ""})
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
          {(dir === "input" || dir === "optimized") && (
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
          )}
          {(dir === "input" || dir === "output") && (
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
          )}
          {dir === "optimized" && (
            <button
              type="button"
              onClick={() => toggleFilter("input")}
              className={cn(
                "px-2.5 py-0.5 rounded-full text-xs font-medium border transition-colors",
                filters.has("input")
                  ? "bg-purple-500/20 text-purple-400 border-purple-500/30"
                  : "bg-transparent text-muted-foreground border-border"
              )}
            >
              Has Input
            </button>
          )}
        </div>
      )}
      <ScrollArea className="flex-1 min-h-0 rounded-md border p-2">
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
                {dir === "optimized" && file.has_input && (
                  <Badge className="bg-purple-500/20 text-purple-400 border-purple-500/30 hover:bg-purple-500/20">
                    {file.input_width && file.input_height
                      ? `${file.input_width}x${file.input_height} @ `
                      : ""}{formatBytes(file.input_size ?? 0)}
                  </Badge>
                )}
                {(dir === "input" || dir === "optimized") && file.has_upscaled && (
                  <Badge className="bg-blue-500/20 text-blue-400 border-blue-500/30 hover:bg-blue-500/20">
                    {file.upscaled_width && file.upscaled_height
                      ? `${file.upscaled_width}x${file.upscaled_height} @ `
                      : ""}{formatBytes(file.upscaled_size ?? 0)}
                  </Badge>
                )}
                {(dir === "input" || dir === "output") && file.has_optimized && (
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
    </div>
  );
}
