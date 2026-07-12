import type { Meta, StoryObj } from "@storybook/nextjs-vite";
import { PipelineBuilder } from "./pipeline-builder";
import { samplePipeline } from "./__fixtures__/pipelines";

const meta: Meta<typeof PipelineBuilder> = {
  title: "Feature/PipelineBuilder",
  component: PipelineBuilder,
  parameters: {
    layout: "padded",
    nextjs: { appDirectory: true, navigation: { pathname: "/pipelines/new" } },
  },
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <div className="w-full max-w-4xl">
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof PipelineBuilder>;

export const NewPipeline: Story = {};

export const EditingExisting: Story = {
  args: { pipeline: samplePipeline },
};
