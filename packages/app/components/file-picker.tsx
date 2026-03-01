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

const FOLDER_COLORS = {
  input:     { badge: "bg-yellow-500/20 text-yellow-400 border-yellow-500/30 hover:bg-yellow-500/20", label: "Input" },
  output:    { badge: "bg-blue-500/20 text-blue-400 border-blue-500/30 hover:bg-blue-500/20",   label: "Upscaled" },
  optimized: { badge: "bg-green-500/20 text-green-400 border-green-500/30 hover:bg-green-500/20", label: "Optimized" },
} as const;

const FOLDER_FILTER_KEY = { input: "input", output: "upscaled", optimized: "optimized" } as const;
type FolderKey = keyof typeof FOLDER_COLORS;

function getFolderData(file: VideoFile, dir: string) {
  const entries: { key: FolderKey; exists: boolean; size: number; width?: number; height?: number }[] = [
    {
      key: "input",
      exists: dir === "input" ? true : !!file.has_input,
      size: dir === "input" ? file.size : (file.input_size ?? 0),
      width: dir === "input" ? file.width : file.input_width,
      height: dir === "input" ? file.height : file.input_height,
    },
    {
      key: "output",
      exists: dir === "output" ? true : !!file.has_upscaled,
      size: dir === "output" ? file.size : (file.upscaled_size ?? 0),
      width: dir === "output" ? file.width : file.upscaled_width,
      height: dir === "output" ? file.height : file.upscaled_height,
    },
    {
      key: "optimized",
      exists: dir === "optimized" ? true : !!file.has_optimized,
      size: dir === "optimized" ? file.size : (file.optimized_size ?? 0),
      width: dir === "optimized" ? file.width : file.optimized_width,
      height: dir === "optimized" ? file.height : file.optimized_height,
    },
  ];
  return entries;
}

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
    if (filters.has("input") && (dir === "input" || file.has_input)) return true;
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

  return (
    <div className="flex flex-col h-full gap-2">
      <div className="flex items-center gap-2">
        <span className="text-xs text-muted-foreground">Legend:</span>
        {(Object.keys(FOLDER_COLORS) as FolderKey[]).map((key) => {
          const filterKey = FOLDER_FILTER_KEY[key];
          const active = filters.has(filterKey);
          return (
            <button
              key={key}
              type="button"
              onClick={() => toggleFilter(filterKey)}
              className={cn(
                "px-2.5 py-0.5 rounded-full text-xs font-medium border transition-colors",
                FOLDER_COLORS[key].badge,
                active && "ring-2 ring-white/30"
              )}
            >
              {FOLDER_COLORS[key].label}
            </button>
          );
        })}
      </div>
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
                {getFolderData(file, dir).map((entry) => (
                  <Badge key={entry.key} className={FOLDER_COLORS[entry.key].badge}>
                    {entry.exists
                      ? `${entry.width && entry.height ? `${entry.width}x${entry.height} @ ` : ""}${formatBytes(entry.size)}`
                      : "\u2014"}
                  </Badge>
                ))}
              </div>
            </div>
          ))}
        </div>
      </ScrollArea>
    </div>
  );
}
