import type { Meta, StoryObj } from "@storybook/nextjs-vite";
import { WorkerGauge } from "./worker-gauge";
import type { ContainerProgress } from "@/lib/types";

const gpu0: ContainerProgress = {
  frame: 1240,
  fps: 19.2,
  total_frames: 34_280,
  elapsed: "00:01:07",
  percent: 3.6,
  filename: "ep09.mkv",
  phase: "upscale · frame",
};

const gpu1: ContainerProgress = {
  frame: 24_180,
  fps: 22.6,
  total_frames: 34_010,
  elapsed: "00:14:51",
  percent: 71.1,
  filename: "ep10.mkv",
  phase: "upscale · frame",
};

const meta: Meta<typeof WorkerGauge> = {
  title: "Feature/WorkerGauge",
  component: WorkerGauge,
  parameters: { layout: "padded" },
};

export default meta;
type Story = StoryObj<typeof WorkerGauge>;

export const Single: Story = { args: { source: "GPU 0", c: gpu0 } };

export const Pair: Story = {
  render: () => (
    <div className="grid max-w-3xl gap-3 sm:grid-cols-2">
      <WorkerGauge source="GPU 0" c={gpu0} />
      <WorkerGauge source="GPU 1" c={gpu1} />
    </div>
  ),
};
