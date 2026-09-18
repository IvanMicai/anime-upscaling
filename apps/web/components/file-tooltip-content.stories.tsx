import type { Meta, StoryObj } from "@storybook/nextjs-vite";
import { expect, within } from "storybook/test";
import { FileTooltipContent } from "./file-tooltip-content";
import type { FolderEntry } from "@/lib/file-utils";

const merged: FolderEntry = {
  key: "merged",
  exists: true,
  size: 131_072_000,
  width: 640,
  height: 480,
  frameRate: 23.976,
  audio: [
    { index: 1, language: "eng", title: "English", codec: "aac", channels: 2 },
    { index: 2, language: "por", title: "Português (BR)", codec: "aac", channels: 2 },
  ],
  subtitles: [{ index: 3, language: "eng", title: "English", codec: "subrip" }],
  sync: {
    v: 1, status: "ok", residual_ms: 1, p95_ms: 10, in_place: 238, checked: 245,
    off_s: 0, coverage: 0.954, segments: 9, gaps: 4, tick_s: 5, video: "en",
  },
};

const meta: Meta<typeof FileTooltipContent> = {
  title: "Feature/FileTooltipContent",
  component: FileTooltipContent,
  parameters: { layout: "centered" },
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <div className="max-w-xs rounded-md border bg-popover p-3 text-popover-foreground">
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof FileTooltipContent>;

// A plain stage file: no verdict, the tooltip reads as it always did.
export const PlainFile: Story = {
  args: { entry: { ...merged, key: "input", sync: undefined } },
  play: async ({ canvasElement }) => {
    await expect(within(canvasElement).queryByText(/Sincronia/)).toBeNull();
  },
};

export const SyncGuaranteed: Story = {
  args: { entry: merged },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByText("Sincronia garantida")).toBeVisible();
    await expect(canvas.getByText(/238\/245 verificações \(97%\), a cada 5 s/)).toBeVisible();
  },
};

// Written with "gravar mesmo sem sincronia garantida": the file says so itself.
export const SyncNotGuaranteed: Story = {
  args: {
    entry: {
      ...merged,
      sync: {
        ...merged.sync!, status: "review", in_place: 155, checked: 240, off_s: 20, residual_ms: 3,
        notes: ["20 s playing at a consistently wrong offset", "only 155/240 validation windows in place"],
      },
    },
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByText("Sincronia NÃO garantida")).toBeVisible();
    await expect(canvas.getByText("20 s em offset errado")).toBeVisible();
  },
};
