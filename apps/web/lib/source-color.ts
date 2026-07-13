// Static Tailwind class strings must appear verbatim so JIT picks them up.

type ColorSet = {
  badge: string; // bg + text + border classes for Badge
  text: string;  // standalone text color for progress labels
  bar: string;   // solid fill for the gauge progress bar
};

// Flat palette of 16 visually-distinct hues. Ordered so neighbouring indices
// stay easy to tell apart. Single source of truth: log badge, gauge title and
// gauge progress bar all read from here.
const PALETTE: ColorSet[] = [
  { badge: "bg-blue-500/20 text-blue-300 border-blue-500/30",       text: "text-blue-400",    bar: "bg-blue-500" },
  { badge: "bg-emerald-500/20 text-emerald-300 border-emerald-500/30", text: "text-emerald-400", bar: "bg-emerald-500" },
  { badge: "bg-amber-500/20 text-amber-300 border-amber-500/30",    text: "text-amber-400",   bar: "bg-amber-500" },
  { badge: "bg-fuchsia-500/20 text-fuchsia-300 border-fuchsia-500/30", text: "text-fuchsia-400", bar: "bg-fuchsia-500" },
  { badge: "bg-cyan-500/20 text-cyan-300 border-cyan-500/30",       text: "text-cyan-400",    bar: "bg-cyan-500" },
  { badge: "bg-lime-500/20 text-lime-300 border-lime-500/30",       text: "text-lime-400",    bar: "bg-lime-500" },
  { badge: "bg-rose-500/20 text-rose-300 border-rose-500/30",       text: "text-rose-400",    bar: "bg-rose-500" },
  { badge: "bg-violet-500/20 text-violet-300 border-violet-500/30", text: "text-violet-400",  bar: "bg-violet-500" },
  { badge: "bg-orange-500/20 text-orange-300 border-orange-500/30", text: "text-orange-400",  bar: "bg-orange-500" },
  { badge: "bg-teal-500/20 text-teal-300 border-teal-500/30",       text: "text-teal-400",    bar: "bg-teal-500" },
  { badge: "bg-pink-500/20 text-pink-300 border-pink-500/30",       text: "text-pink-400",    bar: "bg-pink-500" },
  { badge: "bg-sky-500/20 text-sky-300 border-sky-500/30",          text: "text-sky-400",     bar: "bg-sky-500" },
  { badge: "bg-red-500/20 text-red-300 border-red-500/30",          text: "text-red-400",     bar: "bg-red-500" },
  { badge: "bg-green-500/20 text-green-300 border-green-500/30",    text: "text-green-400",   bar: "bg-green-500" },
  { badge: "bg-indigo-500/20 text-indigo-300 border-indigo-500/30", text: "text-indigo-400",  bar: "bg-indigo-500" },
  { badge: "bg-yellow-500/20 text-yellow-300 border-yellow-500/30", text: "text-yellow-400",  bar: "bg-yellow-500" },
];

const FALLBACK: ColorSet = {
  badge: "",
  text:  "text-muted-foreground",
  bar:   "bg-primary",
};

// Stable hash for unexpected sources so they still get a consistent colour.
function hashIndex(s: string): number {
  let h = 0;
  for (let i = 0; i < s.length; i++) h = (h * 31 + s.charCodeAt(i)) | 0;
  return Math.abs(h) % PALETTE.length;
}

// Deterministic palette index per source. Tuned so the realistic worker set
// (1–2 GPUs with streams + up to 6 FFMPEG + PIPELINE) gets fully distinct
// colours. 16 is the budget: with >16 simultaneous workers some hues repeat.
function paletteIndex(source: string): number {
  // "GPU N" or "GPU N·S" — each GPU owns a block of 4 consecutive colours.
  const gpuMatch = source.match(/^GPU (\d+)(?:·(\d+))?$/);
  if (gpuMatch) {
    const gpu = parseInt(gpuMatch[1], 10);
    const stream = gpuMatch[2] ? parseInt(gpuMatch[2], 10) - 1 : 0;
    return (gpu * 4 + stream) % PALETTE.length;
  }
  // "FFMPEG" or "FFMPEG N" — separate band starting at index 8 (orange).
  const ffMatch = source.match(/^FFMPEG(?: (\d+))?$/);
  if (ffMatch) {
    const slot = ffMatch[1] ? parseInt(ffMatch[1], 10) - 1 : 0;
    return (8 + slot) % PALETTE.length;
  }
  // Reserved slot outside the GPU/FFMPEG bands for the realistic case.
  if (source === "PIPELINE") return 6;
  return -1;
}

export function sourceColorSet(source: string): ColorSet {
  const idx = paletteIndex(source);
  if (idx >= 0) return PALETTE[idx];
  if (source) return PALETTE[hashIndex(source)];
  return FALLBACK;
}
