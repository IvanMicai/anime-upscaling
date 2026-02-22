"use client";

import { useEffect, useState } from "react";
import { Checkbox } from "@/components/ui/checkbox";
import { Label } from "@/components/ui/label";
import { ScrollArea } from "@/components/ui/scroll-area";
import { getFiles } from "@/lib/api";
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
}

export function FilePicker({ selected, onChange }: FilePickerProps) {
  const [files, setFiles] = useState<VideoFile[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    getFiles("input")
      .then((res) => setFiles(res.files ?? []))
      .catch(() => setFiles([]))
      .finally(() => setLoading(false));
  }, []);

  const allSelected = files.length > 0 && selected.length === files.length;
  const names = files.map((f) => f.name);

  const selectedTotal = files
    .filter((f) => selected.includes(f.name))
    .reduce((sum, f) => sum + f.size, 0);

  function toggleAll() {
    onChange(allSelected ? [] : [...names]);
  }

  function toggle(name: string) {
    onChange(
      selected.includes(name)
        ? selected.filter((f) => f !== name)
        : [...selected, name]
    );
  }

  if (loading) {
    return <p className="text-sm text-muted-foreground">Loading files...</p>;
  }

  if (files.length === 0) {
    return <p className="text-sm text-muted-foreground">No input files found.</p>;
  }

  return (
    <div className="space-y-2">
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
      <ScrollArea className="h-64 rounded-md border p-2">
        <div className="space-y-1.5">
          {files.map((file) => (
            <div key={file.name} className="flex items-center gap-2">
              <Checkbox
                id={file.name}
                checked={selected.includes(file.name)}
                onCheckedChange={() => toggle(file.name)}
              />
              <Label htmlFor={file.name} className="font-mono text-sm truncate">
                {file.name}
              </Label>
              <span className="text-xs text-muted-foreground ml-auto shrink-0">
                {formatBytes(file.size)}
              </span>
            </div>
          ))}
        </div>
      </ScrollArea>
    </div>
  );
}
