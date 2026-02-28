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

interface FilePickerProps {
  selected: string[];
  onChange: (files: string[]) => void;
  dir?: string;
}

export function FilePicker({ selected, onChange, dir = "input" }: FilePickerProps) {
  const [files, setFiles] = useState<VideoFile[]>([]);
  const [loading, setLoading] = useState(true);
  const [filters, setFilters] = useState<Set<string>>(new Set());
  const { handleToggle, resetLastClicked } = useShiftSelect(selected, onChange);

  useEffect(() => {
    setLoading(true);
    onChange([]);
    resetLastClicked();
    setFilters(new Set());
    getFiles(dir)
      .then((res) => setFiles(res.files ?? []))
      .catch(() => setFiles([]))
      .finally(() => setLoading(false));
  }, [dir]);

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

  const hasStatus = dir === "input" && files.some((f) => f.has_upscaled || f.has_optimized);

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
      </div>
      {hasStatus && (
        <div className="flex items-center gap-2">
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
                {dir === "input" && file.has_upscaled && (
                  <Badge className="bg-blue-500/20 text-blue-400 border-blue-500/30 hover:bg-blue-500/20">
                    {formatBytes(file.upscaled_size ?? 0)}
                  </Badge>
                )}
                {dir === "input" && file.has_optimized && (
                  <Badge className="bg-green-500/20 text-green-400 border-green-500/30 hover:bg-green-500/20">
                    {formatBytes(file.optimized_size ?? 0)}
                  </Badge>
                )}
                <span className="text-xs text-muted-foreground">
                  {formatBytes(file.size)}
                </span>
              </div>
            </div>
          ))}
        </div>
      </ScrollArea>
    </div>
  );
}
