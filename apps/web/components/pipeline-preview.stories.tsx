import type { Meta, StoryObj } from "@storybook/nextjs-vite";
import { PipelinePreview } from "./pipeline-preview";
import {
  upscaleStep,
  upscale4xStep,
  interpolateStep,
  optimizeStep,
  optimizeCopyStep,
  fullPipelineSteps,
} from "./__fixtures__/pipelines";

const meta: Meta<typeof PipelinePreview> = {
  title: "Feature/PipelinePreview",
  component: PipelinePreview,
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
type Story = StoryObj<typeof PipelinePreview>;

export const Empty: Story = { args: { steps: [] } };

export const SingleUpscale: Story = { args: { steps: [upscaleStep] } };

export const Upscale4x: Story = { args: { steps: [upscale4xStep] } };

export const InterpolateOnly: Story = { args: { steps: [interpolateStep] } };

export const OptimizeOnly: Story = { args: { steps: [optimizeStep] } };

export const OptimizeCopy: Story = { args: { steps: [optimizeCopyStep] } };

export const FullPipeline: Story = { args: { steps: fullPipelineSteps } };

export const HeavyPipeline: Story = {
  args: {
    steps: [upscale4xStep, interpolateStep, { ...optimizeStep, quality: "ultra" }],
  },
};
