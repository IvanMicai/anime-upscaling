"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Button } from "@/components/ui/button";
import { ScrollArea } from "@/components/ui/scroll-area";
import {
  Table,
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
import { compareNatural } from "@/lib/sort";
import { getFiles, getJobs, deleteFiles, downloadFile } from "@/lib/api";
import { usePoll } from "@/lib/use-poll";
import { buildProcessingMap, normalizeRelPath, resolveProcessing } from "@/lib/processing";
import { FileProgressCell } from "@/components/file-progress-cell";
import {
  FOLDER_COLORS,
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
import type { VideoFile } from "@/lib/types";

const TAB_ORDER: FolderKey[] = ["input", "output", "optimized", "interpolated"];

// "all" is a virtual pill that shows every file (the union across stages)
// without filtering by which stage it has.
type Pill = FolderKey | "all";

// The API returns the union of files across every stage, so the selected
// directory pill filters the rows down to files that actually have that stage.
const DIR_HAS: Record<FolderKey, (f: VideoFile) => boolean> = {
  input: (f) => !!f.has_input,
  output: (f) => !!f.has_upscaled,
  optimized: (f) => !!f.has_optimized,
  interpolated: (f) => !!f.has_interpolated,
};

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

export function FileBrowser() {
  const [dir, setDir] = useState<Pill>("all");
  const [path, setPath] = useState<string>("");
  const [files, setFiles] = useState<VideoFile[]>([]);
  const [directories, setDirectories] = useState<string[]>([]);
  const [loading, setLoading] = useState(true);
  const [cachedAt, setCachedAt] = useState<string | null>(null);
  const [refreshing, setRefreshing] = useState(false);

  // Delete mode state
  const [deleteMode, setDeleteMode] = useState(false);
  const [deleteSelections, setDeleteSelections] = useState<Map<string, Set<FolderKey>>>(new Map());
  const [confirmOpen, setConfirmOpen] = useState(false);
  const [deleting, setDeleting] = useState(false);

  // Live processing state, derived from the polled jobs list. Each running job
  // reports the file + percent under progress.containers, so we can show a
  // per-file progress indicator without any backend change.
  const { data: jobs } = usePoll(getJobs, 3000);
  const processing = useMemo(() => buildProcessingMap(jobs ?? []), [jobs]);

  // Reset view state when the source dir/path changes, during render (not in
  // the effect) to avoid an extra commit. See react.dev "Adjusting some state
  // when a prop changes".
  const [prevLoad, setPrevLoad] = useState({ dir, path });
  if (prevLoad.dir !== dir || prevLoad.path !== path) {
    setPrevLoad({ dir, path });
    setLoading(true);
    setDeleteMode(false);
    setDeleteSelections(new Map());
  }

  const loadFiles = useCallback(
    (refresh = false) =>
      // "all" has no folder of its own; fetch the input tree, which the API
      // returns as the union of files across every stage.
      getFiles(dir === "all" ? "input" : dir, path, refresh).then((res) => {
        setFiles(res.files ?? []);
        setDirectories(res.directories ?? []);
        setCachedAt(res.cached_at ?? null);
      }),
    [dir, path],
  );

  useEffect(() => {
    loadFiles()
      .catch(() => {
        setFiles([]);
        setDirectories([]);
      })
      .finally(() => setLoading(false));
  }, [loadFiles]);

  // Auto-refresh the list shortly after a file finishes processing, so the new
  // stage output shows up without a manual refresh. Debounced to coalesce many
  // near-simultaneous completions; skipped while deleting to avoid disruption.
  const prevActiveRef = useRef<Set<string>>(new Set());
  const refreshTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  useEffect(() => {
    const active = new Set<string>();
    for (const [key, info] of processing) {
      if (info.status === "running") active.add(key);
    }
    let finished = false;
    for (const key of prevActiveRef.current) {
      if (!active.has(key)) {
        finished = true;
        break;
      }
    }
    prevActiveRef.current = active;
    if (finished && !deleteMode) {
      if (refreshTimerRef.current) clearTimeout(refreshTimerRef.current);
      refreshTimerRef.current = setTimeout(() => {
        loadFiles(true).catch(() => {});
      }, 1500);
    }
  }, [processing, deleteMode, loadFiles]);
  useEffect(
    () => () => {
      if (refreshTimerRef.current) clearTimeout(refreshTimerRef.current);
    },
    [],
  );

  function handleDirChange(next: Pill) {
    setDir(next);
    setPath("");
  }

  function handleRefresh() {
    setRefreshing(true);
    loadFiles(true)
      .catch(() => {})
      .finally(() => setRefreshing(false));
  }

  // Delete helpers
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
      await loadFiles(true);
    } catch {
      // keep dialog open on error
    } finally {
      setDeleting(false);
    }
  }

  const filtered = [...files]
    .filter((f) => dir === "all" || DIR_HAS[dir](f))
    .sort((a, b) => compareNatural(a.name, b.name));
  const totals = computeColumnTotals(filtered, dir);

  const deleteSummary = getDeleteSummary();
  const deleteTotal = getDeleteTotal();

  return (
    <div className="flex flex-col flex-1 min-h-0 gap-3">
      {/* Directory selector (colored pills) + delete mode toggle */}
      <div className="flex flex-wrap items-center gap-1.5 sm:gap-2">
        <button
          type="button"
          onClick={() => handleDirChange("all")}
          aria-pressed={dir === "all"}
          className={cn(
            "px-2.5 py-0.5 rounded-full text-xs font-medium border transition-colors",
            "bg-secondary text-foreground border-border hover:bg-secondary/80",
            dir === "all" ? "ring-2 ring-white/30" : "opacity-50 hover:opacity-100",
          )}
        >
          All
        </button>
        {TAB_ORDER.map((key) => {
          const active = dir === key;
          return (
            <button
              key={key}
              type="button"
              onClick={() => handleDirChange(key)}
              aria-pressed={active}
              className={cn(
                "px-2.5 py-0.5 rounded-full text-xs font-medium border transition-colors",
                FOLDER_COLORS[key].badge,
                active ? "ring-2 ring-white/30" : "opacity-50 hover:opacity-100",
              )}
            >
              {FOLDER_COLORS[key].label}
            </button>
          );
        })}
        <div className="ml-auto flex flex-wrap items-center gap-2">
          {cachedAt && (
            <span className="flex items-center gap-1.5 text-xs text-muted-foreground">
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

      <Breadcrumbs path={path} onNavigate={setPath} />

      {/* Delete summary bar */}
      {deleteMode && (
        <div className="flex flex-wrap items-center gap-2 rounded-md border border-red-500/30 bg-red-500/10 px-3 py-1.5 text-sm">
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

      {/* File table */}
      {loading ? (
        <p className="text-sm text-muted-foreground">Loading files...</p>
      ) : filtered.length === 0 && directories.length === 0 ? (
        <p className="text-sm text-muted-foreground">
          No {dir === "all" ? "" : `${FOLDER_COLORS[dir].label} `}files found in /{path}.
        </p>
      ) : (
        <TooltipProvider>
          <ScrollArea className="rounded-md border flex-1 min-h-0">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead className="sticky top-0 z-10 bg-background">Filename</TableHead>
                  {COLUMN_ORDER.map((d, i, arr) => (
                    <TableHead
                      key={d}
                      className={cn(
                        "text-right hidden md:table-cell sticky top-0 z-10 bg-background",
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
                {directories.map((name) => (
                  <TableRow
                    key={`dir:${name}`}
                    className="cursor-pointer hover:bg-muted/50"
                    onClick={() => setPath(joinPath(path, name))}
                  >
                    <TableCell className="font-mono text-sm" colSpan={1 + COLUMN_ORDER.length}>
                      <span className="inline-flex items-center gap-2">
                        <svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" className="text-muted-foreground">
                          <path d="M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z" />
                        </svg>
                        {name}/
                      </span>
                    </TableCell>
                  </TableRow>
                ))}
                {filtered.map((file) => {
                  const folders = getFolderData(file, dir);
                  const fileDeleteFolders = deleteSelections.get(file.name);
                  const info = processing.get(
                    normalizeRelPath(joinPath(path, file.name)),
                  );
                  const proc = info ? resolveProcessing(info, file) : null;
                  return (
                    <TableRow key={file.name}>
                      <TableCell className="font-mono text-sm truncate max-w-[180px] sm:max-w-[300px]">
                        {file.name}
                      </TableCell>
                      {folders.map((entry, i, arr) => {
                        const isMarked = fileDeleteFolders?.has(entry.key) ?? false;
                        const canClickDelete = deleteMode && entry.exists;
                        return (
                          <TableCell
                            key={entry.key}
                            className={cn(
                              "text-right text-sm hidden md:table-cell",
                              entry.exists ? FOLDER_COLORS[entry.key].text : "text-muted-foreground",
                              canClickDelete && "cursor-pointer hover:bg-muted/50",
                              isMarked && "ring-2 ring-inset ring-red-500 bg-red-500/10",
                              i === arr.length - 1 && "pr-5",
                            )}
                            onClick={() => {
                              if (canClickDelete) toggleDeleteCell(file.name, entry.key);
                            }}
                          >
                            {!deleteMode && proc && proc.column === entry.key ? (
                              <FileProgressCell info={proc} />
                            ) : entry.exists ? (
                              deleteMode ? (
                                <span>
                                  {formatBytesCompact(entry.size)}
                                  {entry.width && entry.height ? ` | ${formatResolutionLabel(entry.height)}` : ""}
                                </span>
                              ) : (
                                <Tooltip>
                                  <TooltipTrigger asChild>
                                    <span className="inline-flex items-center gap-1.5 justify-end cursor-default">
                                      <span>
                                        {formatBytesCompact(entry.size)}
                                        {entry.width && entry.height ? ` | ${formatResolutionLabel(entry.height)}` : ""}
                                      </span>
                                      <button
                                        type="button"
                                        onClick={(e) => {
                                          e.stopPropagation();
                                          downloadFile(entry.key, file.name, path);
                                        }}
                                        className="opacity-40 hover:opacity-100 transition-opacity"
                                        title={`Download from ${FOLDER_COLORS[entry.key].label}`}
                                      >
                                        <svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                                          <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4" />
                                          <polyline points="7 10 12 15 17 10" />
                                          <line x1="12" y1="15" x2="12" y2="3" />
                                        </svg>
                                      </button>
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
                <TableFooter className="sticky bottom-0 z-10 bg-muted/95 backdrop-blur supports-[backdrop-filter]:bg-muted/80">
                  <TableRow>
                    <TableCell className="font-medium text-sm">
                      Total ({filtered.length} {filtered.length === 1 ? "file" : "files"})
                    </TableCell>
                    {COLUMN_ORDER.map((d, i, arr) => (
                      <TableCell
                        key={d}
                        className={cn(
                          "text-right font-medium text-sm hidden md:table-cell",
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
            </Table>
          </ScrollArea>
        </TooltipProvider>
      )}

      {!loading && filtered.length > 0 && !deleteMode && (
        <p className="text-xs text-muted-foreground">
          Hover a cell for exact size, resolution, framerate, audio &amp;
          subtitle tracks.
        </p>
      )}

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
