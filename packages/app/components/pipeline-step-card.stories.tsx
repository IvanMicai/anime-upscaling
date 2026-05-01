import type { Meta, StoryObj } from "@storybook/nextjs-vite";
import { fn } from "storybook/test";
import { PipelineStepCard } from "./pipeline-step-card";
import {
  upscaleStep,
  upscale4xStep,
  interpolateStep,
  optimizeStep,
  optimizeCopyStep,
  fullPipelineSteps,
} from "./__fixtures__/pipelines";

const meta: Meta<typeof PipelineStepCard> = {
  title: "Feature/PipelineStepCard",
  component: PipelineStepCard,
  parameters: { layout: "padded" },
  tags: ["autodocs"],
  args: {
    onChange: fn(),
    onRemove: fn(),
    onMoveUp: fn(),
    onMoveDown: fn(),
  },
  argTypes: {
    gpuVendor: {
      control: "select",
      options: ["", "nvidia", "amd", "intel"],
    },
  },
  decorators: [
    (Story) => (
      <div className="w-full max-w-2xl">
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof PipelineStepCard>;

export const Upscale: Story = {
  args: {
    step: upscaleStep,
    index: 0,
    totalSteps: 1,
    allSteps: [upscaleStep],
  },
};

export const Upscale4x: Story = {
  args: {
    step: upscale4xStep,
    index: 0,
    totalSteps: 1,
    allSteps: [upscale4xStep],
  },
};

export const Interpolate: Story = {
  args: {
    step: interpolateStep,
    index: 0,
    totalSteps: 1,
    allSteps: [interpolateStep],
  },
};

export const Optimize: Story = {
  args: {
    step: optimizeStep,
    index: 0,
    totalSteps: 1,
    allSteps: [optimizeStep],
  },
};

export const OptimizeCopy: Story = {
  args: {
    step: optimizeCopyStep,
    index: 0,
    totalSteps: 1,
    allSteps: [optimizeCopyStep],
  },
};

export const MiddleStep: Story = {
  args: {
    step: fullPipelineSteps[1],
    index: 1,
    totalSteps: 3,
    allSteps: fullPipelineSteps,
  },
};

export const LastStep: Story = {
  args: {
    step: fullPipelineSteps[2],
    index: 2,
    totalSteps: 3,
    allSteps: fullPipelineSteps,
  },
};

export const NvidiaGpu: Story = {
  args: {
    step: optimizeStep,
    index: 0,
    totalSteps: 1,
    allSteps: [optimizeStep],
    gpuVendor: "nvidia",
  },
};

export const IntelGpu: Story = {
  args: {
    step: optimizeStep,
    index: 0,
    totalSteps: 1,
    allSteps: [optimizeStep],
    gpuVendor: "intel",
  },
};
