import type { VideoFile, FilesResponse } from "@/lib/types";

export const sampleFiles: VideoFile[] = [
  {
    name: "ep01.mkv",
    size: 1_842_390_122,
    width: 1920,
    height: 1080,
    has_input: true,
    has_upscaled: true,
    has_optimized: false,
    has_interpolated: false,
    upscaled_size: 7_421_009_882,
    upscaled_width: 3840,
    upscaled_height: 2160,
    frame_rate: 23.976,
    audio: [
      { index: 0, language: "jpn", title: "Japanese", codec: "flac", channels: 2 },
      { index: 1, language: "eng", title: "English", codec: "ac3", channels: 6 },
    ],
    subtitles: [
      { index: 0, language: "eng", title: "English", codec: "ass" },
      { index: 1, language: "por", title: "Portuguese", codec: "subrip" },
    ],
  },
  {
    name: "ep02.mkv",
    size: 1_910_002_001,
    width: 1920,
    height: 1080,
    has_input: true,
    has_upscaled: true,
    has_optimized: true,
    has_interpolated: true,
    upscaled_size: 7_801_002_001,
    optimized_size: 1_201_002_001,
    interpolated_size: 14_002_002_001,
    upscaled_width: 3840,
    upscaled_height: 2160,
    optimized_width: 3840,
    optimized_height: 2160,
    interpolated_width: 3840,
    interpolated_height: 2160,
    frame_rate: 23.976,
    interpolated_frame_rate: 47.952,
    audio: [{ index: 0, language: "jpn", codec: "flac", channels: 2 }],
    subtitles: [{ index: 0, language: "eng", codec: "ass" }],
  },
  {
    name: "ep03.mkv",
    size: 1_722_904_002,
    width: 1920,
    height: 1080,
    has_input: true,
    has_upscaled: false,
    has_optimized: false,
    has_interpolated: false,
    frame_rate: 23.976,
  },
  {
    name: "ep04.mkv",
    size: 2_004_902_310,
    width: 1280,
    height: 720,
    has_input: true,
    has_upscaled: false,
    frame_rate: 23.976,
  },
];

export const inputFilesResponse: FilesResponse = {
  dir: "input",
  path: "",
  directories: ["season-01", "season-02", "movies"],
  directory_sizes: {
    "season-01": {
      input: 24_902_004_002,
      output: 88_204_004_002,
      optimized: 4_302_004_002,
      interpolated: 0,
    },
    "season-02": {
      input: 18_704_001_011,
      output: 0,
      optimized: 0,
      interpolated: 0,
    },
    movies: {
      input: 9_802_004_002,
      output: 32_402_001_001,
      optimized: 2_102_004_002,
      interpolated: 5_402_004_002,
    },
  },
  files: sampleFiles,
  cached_at: "2026-04-29T11:55:00Z",
};

export const emptyFilesResponse: FilesResponse = {
  dir: "input",
  path: "",
  directories: [],
  files: [],
};

export const nestedFilesResponse: FilesResponse = {
  dir: "input",
  path: "season-01",
  directories: ["specials"],
  directory_sizes: {
    specials: {
      input: 3_402_001_001,
      output: 12_402_001_001,
      optimized: 0,
      interpolated: 0,
    },
  },
  files: sampleFiles.slice(0, 2),
};

const longSeriesFiles: VideoFile[] = Array.from({ length: 36 }, (_, i) => {
  const ep = String(i + 1).padStart(2, "0");
  const hasUpscaled = i % 3 !== 2;
  const hasOptimized = i % 4 === 0;
  const hasInterpolated = i % 5 === 0;
  return {
    name: `ep${ep}.mkv`,
    size: 1_700_000_000 + ((i * 137) % 400) * 1_000_000,
    width: 1920,
    height: 1080,
    has_input: true,
    has_upscaled: hasUpscaled,
    has_optimized: hasOptimized,
    has_interpolated: hasInterpolated,
    upscaled_size: hasUpscaled ? 7_200_000_000 + ((i * 211) % 600) * 1_000_000 : undefined,
    optimized_size: hasOptimized ? 1_100_000_000 + ((i * 89) % 200) * 1_000_000 : undefined,
    interpolated_size: hasInterpolated ? 13_500_000_000 + ((i * 173) % 800) * 1_000_000 : undefined,
    upscaled_width: hasUpscaled ? 3840 : undefined,
    upscaled_height: hasUpscaled ? 2160 : undefined,
    frame_rate: 23.976,
  };
});

export const manyFilesResponse: FilesResponse = {
  dir: "input",
  path: "season-long",
  directories: ["specials", "extras", "ovas"],
  directory_sizes: {
    specials: { input: 6_402_001_001, output: 24_402_001_001, optimized: 0, interpolated: 0 },
    extras: { input: 2_102_001_001, output: 0, optimized: 0, interpolated: 0 },
    ovas: { input: 4_002_001_001, output: 14_402_001_001, optimized: 1_802_001_001, interpolated: 0 },
  },
  files: longSeriesFiles,
  cached_at: "2026-04-29T11:55:00Z",
};
