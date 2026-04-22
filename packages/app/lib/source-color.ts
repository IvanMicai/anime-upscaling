// Static Tailwind class strings must appear verbatim so JIT picks them up.

type ColorSet = {
  badge: string; // bg + text + border classes for Badge
  text: string;  // standalone text color for progress labels
};

// Palette per GPU (mod 4). Stream index (1-based) picks a variant within the family.
const GPU_PALETTES: ColorSet[][] = [
  [
    { badge: "bg-blue-500/20 text-blue-400 border-blue-500/30",   text: "text-blue-400" },
    { badge: "bg-sky-500/20 text-sky-300 border-sky-500/30",      text: "text-sky-300" },
    { badge: "bg-indigo-500/20 text-indigo-300 border-indigo-500/30", text: "text-indigo-300" },
  ],
  [
    { badge: "bg-purple-500/20 text-purple-400 border-purple-500/30",    text: "text-purple-400" },
    { badge: "bg-fuchsia-500/20 text-fuchsia-300 border-fuchsia-500/30", text: "text-fuchsia-300" },
    { badge: "bg-violet-500/20 text-violet-300 border-violet-500/30",    text: "text-violet-300" },
  ],
  [
    { badge: "bg-emerald-500/20 text-emerald-400 border-emerald-500/30", text: "text-emerald-400" },
    { badge: "bg-teal-500/20 text-teal-300 border-teal-500/30",          text: "text-teal-300" },
    { badge: "bg-lime-500/20 text-lime-300 border-lime-500/30",          text: "text-lime-300" },
  ],
  [
    { badge: "bg-amber-500/20 text-amber-400 border-amber-500/30",   text: "text-amber-400" },
    { badge: "bg-orange-500/20 text-orange-300 border-orange-500/30", text: "text-orange-300" },
    { badge: "bg-yellow-500/20 text-yellow-300 border-yellow-500/30", text: "text-yellow-300" },
  ],
];

const FFMPEG_PALETTE: ColorSet[] = [
  { badge: "bg-cyan-500/20 text-cyan-400 border-cyan-500/30",       text: "text-cyan-400" },
  { badge: "bg-teal-500/20 text-teal-300 border-teal-500/30",       text: "text-teal-300" },
  { badge: "bg-sky-500/20 text-sky-300 border-sky-500/30",          text: "text-sky-300" },
];

const PIPELINE_COLOR: ColorSet = {
  badge: "bg-green-500/20 text-green-400 border-green-500/30",
  text:  "text-green-400",
};

const FALLBACK: ColorSet = {
  badge: "",
  text:  "text-muted-foreground",
};

function pick<T>(arr: T[], idx: number): T {
  if (arr.length === 0) return FALLBACK as unknown as T;
  return arr[Math.min(Math.max(idx, 0), arr.length - 1)];
}

export function sourceColorSet(source: string): ColorSet {
  // "GPU N" or "GPU N·S"
  const gpuMatch = source.match(/^GPU (\d+)(?:·(\d+))?$/);
  if (gpuMatch) {
    const gpu = parseInt(gpuMatch[1], 10);
    const stream = gpuMatch[2] ? parseInt(gpuMatch[2], 10) - 1 : 0;
    return pick(GPU_PALETTES[gpu % GPU_PALETTES.length], stream);
  }
  // "FFMPEG" or "FFMPEG N"
  const ffMatch = source.match(/^FFMPEG(?: (\d+))?$/);
  if (ffMatch) {
    const slot = ffMatch[1] ? parseInt(ffMatch[1], 10) - 1 : 0;
    return pick(FFMPEG_PALETTE, slot);
  }
  if (source === "PIPELINE") return PIPELINE_COLOR;
  return FALLBACK;
}
