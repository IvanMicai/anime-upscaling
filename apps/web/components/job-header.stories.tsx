import type { Meta, StoryObj } from "@storybook/nextjs-vite";
import { fn } from "storybook/test";
import { JobHeader } from "./job-header";
import {
  queuedJob,
  runningJob,
  completedJob,
  failedJob,
  cancelledJob,
  customPipelineJob,
} from "./__fixtures__/jobs";

const meta: Meta<typeof JobHeader> = {
  title: "Feature/JobHeader",
  component: JobHeader,
  parameters: {
    layout: "padded",
    nextjs: { appDirectory: true, navigation: { pathname: "/jobs/job_r8a3bc" } },
  },
  tags: ["autodocs"],
  args: { onCancelled: fn() },
  decorators: [
    (Story) => (
      <div className="w-full max-w-3xl">
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof JobHeader>;

export const Queued: Story = { args: { job: queuedJob } };
export const Running: Story = { args: { job: runningJob } };
export const Completed: Story = { args: { job: completedJob } };
export const Failed: Story = { args: { job: failedJob } };
export const Cancelled: Story = { args: { job: cancelledJob } };
export const CustomPipeline: Story = { args: { job: customPipelineJob } };
