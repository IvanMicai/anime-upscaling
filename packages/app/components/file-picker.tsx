"use client";

import { useEffect, useState } from "react";
import { Checkbox } from "@/components/ui/checkbox";
import { Label } from "@/components/ui/label";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Button } from "@/components/ui/button";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";
import { getFiles, deleteFiles } from "@/lib/api";
import { useShiftSelect } from "@/lib/use-shift-select";
import {
  FOLDER_COLORS,
  FOLDER_FILTER_KEY,
  COLUMN_ORDER,
  getFolderData,
  formatBytes,
  formatBytesCompact,
  formatResolutionLabel,
  formatCacheAge,
  type FolderKey,
  type FolderEntry,
} from "@/lib/file-utils";
import type { VideoFile } from "@/lib/types";

function FileTooltipContent({ entry }: { entry: FolderEntry }) {
  return (
    <div className="space-y-1 text-xs">
      <div>Size: {formatBytes(entry.size)}</div>
      {entry.width && entry.height && (
        <div>Resolution: {entry.width}x{entry.height}</div>
      )}
      {entry.audio && entry.audio.length > 0 && (
        <div>
          <div className="font-medium">Audio ({entry.audio.length}):</div>
          {entry.audio.map((a, i) => (
            <div key={i} className="ml-2 text-muted-foreground">
              {[a.title, a.language, a.codec, a.channels ? `${a.channels}ch` : null]
                .filter(Boolean).join(" · ") || `Track ${a.index}`}
            </div>
          ))}
        </div>
      )}
      {entry.subtitles && entry.subtitles.length > 0 && (
        <div>
          <div className="font-medium">Subtitles ({entry.subtitles.length}):</div>
          {entry.subtitles.map((s, i) => (
            <div key={i} className="ml-2 text-muted-foreground">
              {[s.title, s.language, s.codec].filter(Boolean).join(" · ") || `Track ${s.index}`}
            </div>
          ))}
        </div>
      )}
    </div>
  );
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

  // Delete mode state
  const [deleteMode, setDeleteMode] = useState(false);
  const [deleteSelections, setDeleteSelections] = useState<Map<string, Set<FolderKey>>>(new Map());
  const [confirmOpen, setConfirmOpen] = useState(false);
  const [deleting, setDeleting] = useState(false);

  useEffect(() => {
    setLoading(true);
    onChange([]);
    resetLastClicked();
    setFilters(new Set());
    setDeleteMode(false);
    setDeleteSelections(new Map());
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
    if (filters.has("interpolated") && file.has_interpolated) return true;
    return false;
  }

  // Delete mode helpers
  function toggleDeleteCell(fileName: string, folder: FolderKey) {
    setDeleteSelections((prev) => {
      const next = new Map(prev);
      const folders = new Set(next.get(fileName) ?? []);
      if (folders.has(folder)) {
        folders.delete(folder);
        if (folders.size === 0) next.delete(fileName);
        else next.set(fileName, folders);
      } else {
        folders.add(folder);
        next.set(fileName, folders);
      }
      return next;
    });
  }

  function clearDeleteSelections() {
    setDeleteSelections(new Map());
  }

  function getDeleteSummary() {
    const counts: Record<FolderKey, number> = { input: 0, output: 0, optimized: 0, interpolated: 0 };
    for (const folders of deleteSelections.values()) {
      for (const f of folders) counts[f]++;
    }
    return counts;
  }

  function getDeleteTotal() {
    let total = 0;
    for (const folders of deleteSelections.values()) total += folders.size;
    return total;
  }

  async function handleDeleteConfirm() {
    const items: { name: string; folders: string[] }[] = [];
    for (const [name, folders] of deleteSelections) {
      items.push({ name, folders: [...folders] });
    }
    setDeleting(true);
    try {
      await deleteFiles({ items });
      setDeleteSelections(new Map());
      setConfirmOpen(false);
      // Refresh file list
      const res = await getFiles(dir, true);
      setFiles(res.files ?? []);
      setCachedAt(res.cached_at ?? null);
    } catch {
      // keep dialog open on error
    } finally {
      setDeleting(false);
    }
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

  const deleteSummary = getDeleteSummary();
  const deleteTotal = getDeleteTotal();

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
        <div className="ml-auto">
          <Button
            variant={deleteMode ? "destructive" : "outline"}
            size="xs"
            onClick={() => {
              setDeleteMode(!deleteMode);
              if (deleteMode) clearDeleteSelections();
            }}
          >
            {deleteMode ? "Exit Delete" : "Delete Mode"}
          </Button>
        </div>
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

      {/* Delete summary bar */}
      {deleteMode && (
        <div className="flex items-center gap-2 rounded-md border border-red-500/30 bg-red-500/10 px-3 py-1.5 text-sm">
          <span className="text-red-400">
            {deleteTotal > 0
              ? [
                  deleteSummary.input > 0 && `${deleteSummary.input} input`,
                  deleteSummary.output > 0 && `${deleteSummary.output} upscaled`,
                  deleteSummary.optimized > 0 && `${deleteSummary.optimized} optimized`,
                  deleteSummary.interpolated > 0 && `${deleteSummary.interpolated} interpolated`,
                ].filter(Boolean).join(", ")
              : "Select files to delete"}
          </span>
          {deleteTotal > 0 && (
            <div className="ml-auto flex gap-1.5">
              <Button variant="ghost" size="xs" onClick={clearDeleteSelections}>
                Clear
              </Button>
              <Button variant="destructive" size="xs" onClick={() => setConfirmOpen(true)}>
                Delete selected
              </Button>
            </div>
          )}
        </div>
      )}

      <TooltipProvider>
        <ScrollArea className="flex-1 min-h-0 rounded-md border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead className="w-8" />
                <TableHead>Filename</TableHead>
                {COLUMN_ORDER.map((d) => (
                  <TableHead key={d} className={cn("text-right", FOLDER_COLORS[d].text)}>
                    {FOLDER_COLORS[d].label}
                  </TableHead>
                ))}
              </TableRow>
            </TableHeader>
            <TableBody>
              {filtered.map((file, index) => {
                const folders = getFolderData(file, dir);
                const fileDeleteFolders = deleteSelections.get(file.name);
                return (
                  <TableRow
                    key={file.name}
                    className="cursor-pointer"
                    onClick={(e) => {
                      if (!deleteMode) handleToggle(index, filteredNames, e.shiftKey);
                    }}
                  >
                    <TableCell className="w-8">
                      <Checkbox
                        checked={selected.includes(file.name)}
                        tabIndex={-1}
                        className="pointer-events-none"
                      />
                    </TableCell>
                    <TableCell className="font-mono text-sm truncate max-w-[300px]">
                      {file.name}
                    </TableCell>
                    {folders.map((entry) => {
                      const isMarked = fileDeleteFolders?.has(entry.key) ?? false;
                      const canClick = deleteMode && entry.exists;
                      return (
                        <TableCell
                          key={entry.key}
                          className={cn(
                            "text-right text-sm",
                            entry.exists ? FOLDER_COLORS[entry.key].text : "text-muted-foreground",
                            canClick && "cursor-pointer hover:bg-muted/50",
                            isMarked && "ring-2 ring-inset ring-red-500 bg-red-500/10"
                          )}
                          onClick={(e) => {
                            if (canClick) {
                              e.stopPropagation();
                              toggleDeleteCell(file.name, entry.key);
                            }
                          }}
                        >
                          {entry.exists ? (
                            deleteMode ? (
                              <span>
                                {formatBytesCompact(entry.size)}
                                {entry.width && entry.height ? ` | ${formatResolutionLabel(entry.height)}` : ""}
                              </span>
                            ) : (
                              <Tooltip>
                                <TooltipTrigger asChild>
                                  <span className="cursor-default">
                                    {formatBytesCompact(entry.size)}
                                    {entry.width && entry.height ? ` | ${formatResolutionLabel(entry.height)}` : ""}
                                  </span>
                                </TooltipTrigger>
                                <TooltipContent side="top" className="max-w-xs">
                                  <FileTooltipContent entry={entry} />
                                </TooltipContent>
                              </Tooltip>
                            )
                          ) : (
                            "\u2014"
                          )}
                        </TableCell>
                      );
                    })}
                  </TableRow>
                );
              })}
            </TableBody>
          </Table>
        </ScrollArea>
      </TooltipProvider>

      {/* Delete confirmation dialog */}
      <Dialog open={confirmOpen} onOpenChange={setConfirmOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Confirm Deletion</DialogTitle>
            <DialogDescription>
              The following files will be permanently deleted:
            </DialogDescription>
          </DialogHeader>
          <div className="max-h-60 overflow-y-auto space-y-2 text-sm">
            {COLUMN_ORDER.map((folder) => {
              const names: string[] = [];
              for (const [name, folders] of deleteSelections) {
                if (folders.has(folder)) names.push(name);
              }
              if (names.length === 0) return null;
              return (
                <div key={folder}>
                  <p className={cn("font-medium", FOLDER_COLORS[folder].text)}>
                    {FOLDER_COLORS[folder].label} ({names.length})
                  </p>
                  <ul className="ml-4 list-disc text-muted-foreground">
                    {names.map((n) => <li key={n} className="font-mono text-xs">{n}</li>)}
                  </ul>
                </div>
              );
            })}
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setConfirmOpen(false)} disabled={deleting}>
              Cancel
            </Button>
            <Button variant="destructive" onClick={handleDeleteConfirm} disabled={deleting}>
              {deleting ? "Deleting..." : `Delete ${deleteTotal} files`}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
