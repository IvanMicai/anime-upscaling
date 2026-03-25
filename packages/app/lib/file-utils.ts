import type { VideoFile } from "./types";

export const FOLDER_COLORS = {
  input:        { badge: "bg-yellow-500/20 text-yellow-400 border-yellow-500/30 hover:bg-yellow-500/20", text: "text-yellow-400", label: "Input" },
  output:       { badge: "bg-blue-500/20 text-blue-400 border-blue-500/30 hover:bg-blue-500/20", text: "text-blue-400", label: "Upscaled" },
  optimized:    { badge: "bg-green-500/20 text-green-400 border-green-500/30 hover:bg-green-500/20", text: "text-green-400", label: "Optimized" },
  interpolated: { badge: "bg-purple-500/20 text-purple-400 border-purple-500/30 hover:bg-purple-500/20", text: "text-purple-400", label: "Interpolated" },
} as const;

export const FOLDER_FILTER_KEY = { input: "input", output: "upscaled", optimized: "optimized", interpolated: "interpolated" } as const;
export type FolderKey = keyof typeof FOLDER_COLORS;

export function getFolderData(file: VideoFile, dir: string) {
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
    {
      key: "interpolated",
      exists: dir === "interpolated" ? true : !!file.has_interpolated,
      size: dir === "interpolated" ? file.size : (file.interpolated_size ?? 0),
      width: dir === "interpolated" ? file.width : file.interpolated_width,
      height: dir === "interpolated" ? file.height : file.interpolated_height,
    },
  ];
  return entries;
}

export function formatBytes(bytes: number): string {
  if (bytes === 0) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB"];
  const i = Math.floor(Math.log(bytes) / Math.log(1024));
  const value = bytes / Math.pow(1024, i);
  return `${value.toFixed(i > 0 ? 1 : 0)} ${units[i]}`;
}

export function formatCacheAge(iso: string): string {
  const diff = Math.floor((Date.now() - new Date(iso).getTime()) / 1000);
  if (diff < 10) return "just now";
  if (diff < 60) return `${diff}s ago`;
  const mins = Math.floor(diff / 60);
  return `${mins}m ago`;
}
