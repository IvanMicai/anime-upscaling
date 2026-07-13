import type { Meta, StoryObj } from "@storybook/nextjs-vite";
import { fn } from "storybook/test";
import { JobList } from "./job-list";
import { sampleJobs } from "./__fixtures__/jobs";

const meta: Meta<typeof JobList> = {
  title: "Feature/JobList",
  component: JobList,
  parameters: {
    layout: "padded",
    nextjs: { appDirectory: true, navigation: { pathname: "/jobs" } },
  },
  tags: ["autodocs"],
  args: { onRemove: fn() },
};

export default meta;
type Story = StoryObj<typeof JobList>;

export const Empty: Story = { args: { jobs: [] } };

export const Mixed: Story = { args: { jobs: sampleJobs } };

export const RunningOnly: Story = {
  args: { jobs: sampleJobs.filter((j) => j.status === "running") },
};

export const ManyJobs: Story = {
  args: {
    jobs: Array.from({ length: 15 }).flatMap((_, i) =>
      sampleJobs.map((j) => ({ ...j, id: `${j.id}_${i}` })),
    ),
  },
};
