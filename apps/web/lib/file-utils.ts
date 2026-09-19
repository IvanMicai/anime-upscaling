import type { VideoFile, AudioTrack, SubtitleTrack, SyncInfo } from "./types";

export const FOLDER_COLORS = {
  input:        { badge: "bg-yellow-500/20 text-yellow-400 border-yellow-500/30 hover:bg-yellow-500/20", text: "text-yellow-400", label: "Input" },
  merged:       { badge: "bg-pink-500/20 text-pink-400 border-pink-500/30 hover:bg-pink-500/20", text: "text-pink-400", label: "Merged" },
  output:       { badge: "bg-blue-500/20 text-blue-400 border-blue-500/30 hover:bg-blue-500/20", text: "text-blue-400", label: "Upscaling" },
  optimized:    { badge: "bg-green-500/20 text-green-400 border-green-500/30 hover:bg-green-500/20", text: "text-green-400", label: "Optimized" },
  interpolated: { badge: "bg-purple-500/20 text-purple-400 border-purple-500/30 hover:bg-purple-500/20", text: "text-purple-400", label: "Interpolated" },
} as const;

export const FOLDER_FILTER_KEY = { input: "input", merged: "merged", output: "upscaled", optimized: "optimized", interpolated: "interpolated" } as const;
export type FolderKey = keyof typeof FOLDER_COLORS;

export const COLUMN_ORDER: FolderKey[] = ["input", "merged", "output", "interpolated", "optimized"];

export const FOLDER_OPTIONS: { value: FolderKey; label: string }[] = [
  { value: "input", label: "Input" },
  { value: "merged", label: "Merged (dual audio)" },
  { value: "output", label: "Output (upscaled)" },
  { value: "interpolated", label: "Interpolated" },
  { value: "optimized", label: "Optimized" },
];

export interface FolderEntry {
  key: FolderKey;
  exists: boolean;
  size: number;
  width?: number;
  height?: number;
  frameRate?: number;
  audio?: AudioTrack[];
  subtitles?: SubtitleTrack[];
  sync?: SyncInfo;
}

export function computeColumnTotals(
  files: VideoFile[],
  dir: string,
): Record<FolderKey, number> {
  const totals: Record<FolderKey, number> = { input: 0, merged: 0, output: 0, optimized: 0, interpolated: 0 };
  for (const f of files) {
    for (const entry of getFolderData(f, dir)) {
      if (entry.exists) totals[entry.key] += entry.size;
    }
  }
  return totals;
}

export function getFolderData(file: VideoFile, dir: string): FolderEntry[] {
  const entries: FolderEntry[] = [
    {
      key: "input",
      exists: !!file.has_input,
      size: dir === "input" ? file.size : (file.input_size ?? 0),
      width: dir === "input" ? file.width : file.input_width,
      height: dir === "input" ? file.height : file.input_height,
      frameRate: dir === "input" ? file.frame_rate : file.input_frame_rate,
      audio: dir === "input" ? file.audio : file.input_audio,
      subtitles: dir === "input" ? file.subtitles : file.input_subtitles,
    },
    {
      key: "merged",
      exists: !!file.has_merged,
      size: dir === "merged" ? file.size : (file.merged_size ?? 0),
      width: dir === "merged" ? file.width : file.merged_width,
      height: dir === "merged" ? file.height : file.merged_height,
      frameRate: dir === "merged" ? file.frame_rate : file.merged_frame_rate,
      audio: dir === "merged" ? file.audio : file.merged_audio,
      subtitles: dir === "merged" ? file.subtitles : file.merged_subtitles,
      sync: dir === "merged" ? file.sync : file.merged_sync,
    },
    {
      key: "output",
      exists: !!file.has_upscaled,
      size: dir === "output" ? file.size : (file.upscaled_size ?? 0),
      width: dir === "output" ? file.width : file.upscaled_width,
      height: dir === "output" ? file.height : file.upscaled_height,
      frameRate: dir === "output" ? file.frame_rate : file.upscaled_frame_rate,
      audio: dir === "output" ? file.audio : file.upscaled_audio,
      subtitles: dir === "output" ? file.subtitles : file.upscaled_subtitles,
    },
    {
      key: "interpolated",
      exists: !!file.has_interpolated,
      size: dir === "interpolated" ? file.size : (file.interpolated_size ?? 0),
      width: dir === "interpolated" ? file.width : file.interpolated_width,
      height: dir === "interpolated" ? file.height : file.interpolated_height,
      frameRate: dir === "interpolated" ? file.frame_rate : file.interpolated_frame_rate,
      audio: dir === "interpolated" ? file.audio : file.interpolated_audio,
      subtitles: dir === "interpolated" ? file.subtitles : file.interpolated_subtitles,
    },
    {
      key: "optimized",
      exists: !!file.has_optimized,
      size: dir === "optimized" ? file.size : (file.optimized_size ?? 0),
      width: dir === "optimized" ? file.width : file.optimized_width,
      height: dir === "optimized" ? file.height : file.optimized_height,
      frameRate: dir === "optimized" ? file.frame_rate : file.optimized_frame_rate,
      audio: dir === "optimized" ? file.audio : file.optimized_audio,
      subtitles: dir === "optimized" ? file.subtitles : file.optimized_subtitles,
    },
  ];
  return entries;
}

const RESOLUTION_LABELS: Record<number, string> = {
  480: "480p",
  720: "720p",
  1080: "1080p",
  1440: "1440p",
  2160: "4K",
  4320: "8K",
};

export function formatResolutionLabel(height: number): string {
  return RESOLUTION_LABELS[height] ?? `${height}p`;
}

export function formatBytesCompact(bytes: number): string {
  if (bytes === 0) return "0B";
  const units = ["B", "K", "M", "G", "T"];
  const i = Math.floor(Math.log(bytes) / Math.log(1024));
  const value = bytes / Math.pow(1024, i);
  const rounded = Math.round(value * 10) / 10;
  if (rounded === Math.floor(rounded)) {
    return `${Math.floor(rounded)}${units[i]}`;
  }
  return `${rounded.toFixed(1)}${units[i]}`;
}

export function formatFrameRate(fps: number): string {
  if (!fps || fps <= 0) return "";
  const rounded = Math.round(fps * 1000) / 1000;
  return Number.isInteger(rounded) ? `${rounded} fps` : `${rounded.toFixed(3)} fps`;
}

export function formatBytes(bytes: number): string {
  if (bytes === 0) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB"];
  const i = Math.floor(Math.log(bytes) / Math.log(1024));
  const value = bytes / Math.pow(1024, i);
  return `${value.toFixed(i > 0 ? 1 : 0)} ${units[i]}`;
}

export function joinPath(base: string, segment: string): string {
  if (!base) return segment;
  if (!segment) return base;
  return `${base}/${segment}`;
}

export function parentPath(path: string): string {
  if (!path) return "";
  const idx = path.lastIndexOf("/");
  return idx === -1 ? "" : path.slice(0, idx);
}

export function getBreadcrumbs(path: string): { label: string; path: string }[] {
  const crumbs: { label: string; path: string }[] = [{ label: "/", path: "" }];
  if (!path) return crumbs;
  const segs = path.split("/");
  let acc = "";
  for (const seg of segs) {
    acc = acc ? `${acc}/${seg}` : seg;
    crumbs.push({ label: seg, path: acc });
  }
  return crumbs;
}

export function formatCacheAge(iso: string): string {
  const diff = Math.floor((Date.now() - new Date(iso).getTime()) / 1000);
  if (diff < 10) return "just now";
  if (diff < 60) return `${diff}s ago`;
  const mins = Math.floor(diff / 60);
  return `${mins}m ago`;
}

// How a sync verdict reads at a glance. "review" means the file was written
// without the sync being guaranteed (the merge was forced).
export function syncShareInPlace(sync: SyncInfo): number {
  return sync.checked > 0 ? Math.round((sync.in_place / sync.checked) * 100) : 0;
}
