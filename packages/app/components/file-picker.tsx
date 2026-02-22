"use client";

import { useEffect, useState } from "react";
import { Checkbox } from "@/components/ui/checkbox";
import { Label } from "@/components/ui/label";
import { ScrollArea } from "@/components/ui/scroll-area";
import { getFiles } from "@/lib/api";

interface FilePickerProps {
  selected: string[];
  onChange: (files: string[]) => void;
}

export function FilePicker({ selected, onChange }: FilePickerProps) {
  const [files, setFiles] = useState<string[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    getFiles("input")
      .then((res) => setFiles(res.files ?? []))
      .catch(() => setFiles([]))
      .finally(() => setLoading(false));
  }, []);

  const allSelected = files.length > 0 && selected.length === files.length;

  function toggleAll() {
    onChange(allSelected ? [] : [...files]);
  }

  function toggle(file: string) {
    onChange(
      selected.includes(file)
        ? selected.filter((f) => f !== file)
        : [...selected, file]
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
          Select All ({files.length})
        </Label>
      </div>
      <ScrollArea className="h-48 rounded-md border p-2">
        <div className="space-y-1.5">
          {files.map((file) => (
            <div key={file} className="flex items-center gap-2">
              <Checkbox
                id={file}
                checked={selected.includes(file)}
                onCheckedChange={() => toggle(file)}
              />
              <Label htmlFor={file} className="font-mono text-sm truncate">
                {file}
              </Label>
            </div>
          ))}
        </div>
      </ScrollArea>
    </div>
  );
}
