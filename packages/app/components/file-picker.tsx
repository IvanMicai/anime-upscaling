"use client";

import { useEffect, useState } from "react";
import { Checkbox } from "@/components/ui/checkbox";
import { Label } from "@/components/ui/label";
import { ScrollArea } from "@/components/ui/scroll-area";
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
  const { handleToggle, resetLastClicked } = useShiftSelect(selected, onChange);

  useEffect(() => {
    setLoading(true);
    onChange([]);
    resetLastClicked();
    getFiles(dir)
      .then((res) => setFiles(res.files ?? []))
      .catch(() => setFiles([]))
      .finally(() => setLoading(false));
  }, [dir]);

  const allSelected = files.length > 0 && selected.length === files.length;
  const names = files.map((f) => f.name);

  const selectedTotal = files
    .filter((f) => selected.includes(f.name))
    .reduce((sum, f) => sum + f.size, 0);

  function toggleAll() {
    onChange(allSelected ? [] : [...names]);
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
          Select All ({files.length} files{selected.length > 0 ? `, ${formatBytes(selectedTotal)}` : ""})
        </Label>
      </div>
      <ScrollArea className="flex-1 min-h-0 rounded-md border p-2">
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
              {dir === "input" && file.has_upscaled && (
                <span className="w-2.5 h-2.5 rounded-full bg-blue-500 shrink-0" />
              )}
              {dir === "input" && file.has_optimized && (
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
      {hasStatus && (
        <div className="flex items-center gap-4 text-xs text-muted-foreground">
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
    </div>
  );
}
