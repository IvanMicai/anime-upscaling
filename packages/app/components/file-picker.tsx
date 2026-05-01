"use client";

import { useEffect, useState } from "react";
import { Checkbox } from "@/components/ui/checkbox";
import { Label } from "@/components/ui/label";
import { Button } from "@/components/ui/button";
import {
  TableBody,
  TableCell,
  TableFooter,
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
  computeColumnTotals,
  getFolderData,
  formatBytes,
  formatBytesCompact,
  formatResolutionLabel,
  formatFrameRate,
  formatCacheAge,
  joinPath,
  type FolderKey,
  type FolderEntry,
} from "@/lib/file-utils";
import { Breadcrumbs } from "@/components/breadcrumbs";
import type { VideoFile, DirectorySizes } from "@/lib/types";

function FileTooltipContent({ entry }: { entry: FolderEntry }) {
  return (
    <div className="space-y-1 text-xs">
      <div>Size: {formatBytes(entry.size)}</div>
      {entry.width && entry.height && (
        <div>Resolution: {entry.width}x{entry.height}</div>
      )}
      {entry.frameRate ? (
        <div>Framerate: {formatFrameRate(entry.frameRate)}</div>
      ) : null}
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
  path?: string;
  onPathChange?: (path: string) => void;
}

export function FilePicker({ selected, onChange, dir = "input", path: pathProp, onPathChange }: FilePickerProps) {
  const [internalPath, setInternalPath] = useState<string>("");
  const path = pathProp ?? internalPath;
  const setPath = (p: string) => {
    if (onPathChange) onPathChange(p);
    else setInternalPath(p);
  };

  const [files, setFiles] = useState<VideoFile[]>([]);
  const [directories, setDirectories] = useState<string[]>([]);
  const [directorySizes, setDirectorySizes] = useState<Record<string, DirectorySizes>>({});
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

  // Reset selection only when the source dir changes (not on path navigation).
  useEffect(() => {
    onChange([]);
    setInternalPath("");
    if (onPathChange) onPathChange("");
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [dir]);

  useEffect(() => {
    setLoading(true);
    resetLastClicked();
    setFilters(new Set());
    setDeleteMode(false);
    setDeleteSelections(new Map());
    getFiles(dir, path)
      .then((res) => {
        setFiles(res.files ?? []);
        setDirectories(res.directories ?? []);
        setDirectorySizes(res.directory_sizes ?? {});
        setCachedAt(res.cached_at ?? null);
      })
      .catch(() => {
        setFiles([]);
        setDirectories([]);
        setDirectorySizes({});
      })
      .finally(() => setLoading(false));
  }, [dir, path, resetLastClicked]);

  function handleRefresh() {
    setRefreshing(true);
    getFiles(dir, path, true)
      .then((res) => {
        setFiles(res.files ?? []);
        setDirectories(res.directories ?? []);
        setDirectorySizes(res.directory_sizes ?? {});
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
    const items: { name: string; path?: string; folders: string[] }[] = [];
    for (const [name, folders] of deleteSelections) {
      items.push({ name, path: path || undefined, folders: [...folders] });
    }
    setDeleting(true);
    try {
      await deleteFiles({ items });
      setDeleteSelections(new Map());
      setConfirmOpen(false);
      // Refresh file list
      const res = await getFiles(dir, path, true);
      setFiles(res.files ?? []);
      setDirectories(res.directories ?? []);
      setDirectorySizes(res.directory_sizes ?? {});
      setCachedAt(res.cached_at ?? null);
    } catch {
      // keep dialog open on error
    } finally {
      setDeleting(false);
    }
  }

  const sorted = [...files].sort((a, b) => a.name.localeCompare(b.name));
  const filtered = sorted.filter(matchesFilter);
  const fileTotals = computeColumnTotals(filtered, dir as FolderKey);
  const totals: Record<FolderKey, number> = { ...fileTotals };
  for (const sizes of Object.values(directorySizes)) {
    totals.input += sizes.input;
    totals.output += sizes.output;
    totals.optimized += sizes.optimized;
    totals.interpolated += sizes.interpolated;
  }
  // Selection identifiers are file paths relative to the source dir
  // (e.g. "season1/ep01.mkv") so picks survive subfolder navigation.
  const toRel = (name: string) => (path ? `${path}/${name}` : name);
  const filteredRelPaths = filtered.map((f) => toRel(f.name));

  const allSelected = filtered.length > 0 && filteredRelPaths.every((p) => selected.includes(p));

  const selectedTotal = files
    .filter((f) => selected.includes(toRel(f.name)))
    .reduce((sum, f) => sum + f.size, 0);

  function toggleAll() {
    if (allSelected) {
      onChange(selected.filter((s) => !filteredRelPaths.includes(s)));
    } else {
      const next = new Set(selected);
      filteredRelPaths.forEach((p) => next.add(p));
      onChange([...next]);
    }
  }

  const deleteSummary = getDeleteSummary();
  const deleteTotal = getDeleteTotal();

  return (
    <div className="flex flex-col h-full gap-2">
      <Breadcrumbs path={path} onNavigate={setPath} />
      <div className="flex flex-wrap items-center gap-1.5 sm:gap-2">
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
        <div className="ml-auto flex flex-nowrap items-center gap-2 min-h-[28px]">
          <div
            aria-hidden={!deleteMode}
            className={cn(
              "flex flex-nowrap items-center gap-2 rounded-md border px-2 py-1 text-xs transition-opacity",
              deleteMode
                ? "border-red-500/40 bg-red-500/5"
                : "pointer-events-none invisible border-transparent",
            )}
          >
            <span className="flex flex-nowrap items-center gap-x-2 whitespace-nowrap">
              {deleteTotal > 0 ? (
                COLUMN_ORDER.map((key) => {
                  const count = deleteSummary[key];
                  if (count <= 0) return null;
                  return (
                    <span key={key} className={FOLDER_COLORS[key].text}>
                      {count} {FOLDER_COLORS[key].label}
                    </span>
                  );
                })
              ) : (
                <span className="text-red-400">Select files to delete</span>
              )}
            </span>
            <Button
              variant="ghost"
              size="xs"
              onClick={clearDeleteSelections}
              className={cn(deleteTotal === 0 && "invisible pointer-events-none")}
            >
              Clear
            </Button>
            <Button
              variant="destructive"
              size="xs"
              onClick={() => setConfirmOpen(true)}
              className={cn(deleteTotal === 0 && "invisible pointer-events-none")}
            >
              Delete selected
            </Button>
          </div>
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
      <div className="flex flex-wrap items-center gap-2">
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

      <TooltipProvider>
        <div className="scrollbar-dark mb-3 flex-1 min-h-0 overflow-auto rounded-md border">
          <table className="w-full caption-bottom border-collapse text-sm">
            <TableHeader>
              <TableRow className="border-b-0">
                <TableHead className="sticky top-0 z-10 w-8 bg-[oklch(0.32_0_0)] shadow-[inset_0_-1px_0_var(--border)]" />
                <TableHead className="sticky top-0 z-10 bg-[oklch(0.32_0_0)] shadow-[inset_0_-1px_0_var(--border)]">
                  Filename
                </TableHead>
                {COLUMN_ORDER.map((d, i, arr) => (
                  <TableHead
                    key={d}
                    className={cn(
                      "sticky top-0 z-10 hidden bg-[oklch(0.32_0_0)] text-right shadow-[inset_0_-1px_0_var(--border)] md:table-cell",
                      FOLDER_COLORS[d].text,
                      i === arr.length - 1 && "pr-5",
                    )}
                  >
                    {FOLDER_COLORS[d].label}
                  </TableHead>
                ))}
              </TableRow>
            </TableHeader>
            <TableBody>
              {loading && (
                <TableRow>
                  <TableCell
                    className="py-8 text-center text-sm text-muted-foreground"
                    colSpan={2 + COLUMN_ORDER.length}
                  >
                    Loading files...
                  </TableCell>
                </TableRow>
              )}
              {!loading && directories.map((name) => {
                const sizes = directorySizes[name];
                return (
                  <TableRow
                    key={`dir:${name}`}
                    className="cursor-pointer hover:bg-muted/50"
                    onClick={() => setPath(joinPath(path, name))}
                  >
                    <TableCell className="w-8" />
                    <TableCell className="font-mono text-sm">
                      <span className="inline-flex items-center gap-2">
                        <svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" className="text-muted-foreground">
                          <path d="M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z" />
                        </svg>
                        {name}/
                      </span>
                    </TableCell>
                    {COLUMN_ORDER.map((d, i, arr) => {
                      const value = sizes?.[d] ?? 0;
                      return (
                        <TableCell
                          key={d}
                          className={cn(
                            "text-right text-sm hidden md:table-cell tabular-nums",
                            value > 0 ? FOLDER_COLORS[d].text : "text-muted-foreground",
                            i === arr.length - 1 && "pr-5",
                          )}
                        >
                          {value > 0 ? formatBytesCompact(value) : "—"}
                        </TableCell>
                      );
                    })}
                  </TableRow>
                );
              })}
              {!loading && filtered.map((file, index) => {
                const folders = getFolderData(file, dir);
                const rel = toRel(file.name);
                const fileDeleteFolders = deleteSelections.get(file.name);
                return (
                  <TableRow
                    key={rel}
                    className="cursor-pointer"
                    onClick={(e) => {
                      if (!deleteMode) handleToggle(index, filteredRelPaths, e.shiftKey);
                    }}
                  >
                    <TableCell className="w-8">
                      <Checkbox
                        checked={selected.includes(rel)}
                        tabIndex={-1}
                        className="pointer-events-none"
                      />
                    </TableCell>
                    <TableCell className="font-mono text-sm truncate max-w-[180px] sm:max-w-[300px]">
                      {file.name}
                    </TableCell>
                    {folders.map((entry, i, arr) => {
                      const isMarked = fileDeleteFolders?.has(entry.key) ?? false;
                      const canClick = deleteMode && entry.exists;
                      return (
                        <TableCell
                          key={entry.key}
                          className={cn(
                            "text-right text-sm hidden md:table-cell",
                            entry.exists ? FOLDER_COLORS[entry.key].text : "text-muted-foreground",
                            canClick && "cursor-pointer hover:bg-muted/50",
                            isMarked && "ring-2 ring-inset ring-red-500 bg-red-500/10",
                            i === arr.length - 1 && "pr-5",
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
            {filtered.length > 0 && (
              <TableFooter>
                <TableRow>
                  <TableCell className="sticky bottom-[-1px] z-10 w-8 border-t bg-muted" />
                  <TableCell className="sticky bottom-[-1px] z-10 border-t bg-muted font-medium text-sm">
                    Total
                  </TableCell>
                  {COLUMN_ORDER.map((d, i, arr) => (
                    <TableCell
                      key={d}
                      className={cn(
                        "sticky bottom-[-1px] z-10 hidden border-t bg-muted text-right font-medium text-sm md:table-cell",
                        FOLDER_COLORS[d].text,
                        i === arr.length - 1 && "pr-5",
                      )}
                    >
                      {totals[d] > 0 ? formatBytesCompact(totals[d]) : "—"}
                    </TableCell>
                  ))}
                </TableRow>
              </TableFooter>
            )}
          </table>
        </div>
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
            {(() => {
              const filesByName = new Map(files.map((f) => [f.name, f]));
              let grandTotalSize = 0;
              const sections = COLUMN_ORDER.map((folder) => {
                const items: { name: string; size: number }[] = [];
                let folderTotal = 0;
                for (const [name, folders] of deleteSelections) {
                  if (!folders.has(folder)) continue;
                  const file = filesByName.get(name);
                  const entry = file
                    ? getFolderData(file, dir).find((e) => e.key === folder)
                    : undefined;
                  const size = entry?.size ?? 0;
                  items.push({ name, size });
                  folderTotal += size;
                }
                if (items.length === 0) return null;
                grandTotalSize += folderTotal;
                return (
                  <div key={folder}>
                    <p className={cn("font-medium", FOLDER_COLORS[folder].text)}>
                      {FOLDER_COLORS[folder].label} ({items.length} files,{" "}
                      {formatBytesCompact(folderTotal)})
                    </p>
                    <ul className="ml-4 list-disc text-muted-foreground">
                      {items.map(({ name, size }) => (
                        <li key={name} className="font-mono text-xs">
                          <span className="flex justify-between gap-2">
                            <span className="truncate">{name}</span>
                            <span className="shrink-0 tabular-nums">
                              {size > 0 ? formatBytesCompact(size) : "—"}
                            </span>
                          </span>
                        </li>
                      ))}
                    </ul>
                  </div>
                );
              });
              return (
                <>
                  {sections}
                  <div className="border-t border-border pt-2 flex justify-between font-medium">
                    <span>Total</span>
                    <span className="tabular-nums">
                      {deleteTotal} files · {formatBytesCompact(grandTotalSize)}
                    </span>
                  </div>
                </>
              );
            })()}
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
