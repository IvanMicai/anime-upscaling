import type { Meta, StoryObj } from "@storybook/nextjs-vite";
import { ProgressBar } from "./progress-bar";
import {
  queuedProgress,
  runningProgress,
  completedProgress,
  failedProgress,
} from "./__fixtures__/jobs";

const meta: Meta<typeof ProgressBar> = {
  title: "Feature/ProgressBar",
  component: ProgressBar,
  parameters: { layout: "padded" },
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <div className="w-full max-w-2xl">
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof ProgressBar>;

export const Queued: Story = { args: { progress: queuedProgress } };
export const Running: Story = { args: { progress: runningProgress } };
export const Completed: Story = { args: { progress: completedProgress } };
export const Failed: Story = { args: { progress: failedProgress } };

export const MultiContainer: Story = {
  args: {
    progress: {
      total: 10,
      completed: 3,
      failed: 1,
      skipped: 0,
      current: "ep05.mkv",
      containers: {
        "GPU 0·1": {
          frame: 4200,
          fps: 22.5,
          total_frames: 34_280,
          elapsed: "00:03:07",
          percent: 12.2,
          filename: "ep05.mkv",
          phase: "upscale",
        },
        "GPU 0·2": {
          frame: 1800,
          fps: 19.1,
          total_frames: 34_280,
          elapsed: "00:01:34",
          percent: 5.2,
          filename: "ep06.mkv",
          phase: "upscale",
        },
        "FFMPEG 1": {
          frame: 0,
          fps: 0,
          phase: "encoding",
          elapsed: "00:00:42",
        },
      },
    },
  },
};

export const ZeroTotal: Story = {
  args: {
    progress: {
      total: 0,
      completed: 0,
      failed: 0,
      skipped: 0,
      current: "",
    },
  },
  parameters: {
    docs: {
      description: { story: "Returns null when total is 0 — nothing should render." },
    },
  },
};
