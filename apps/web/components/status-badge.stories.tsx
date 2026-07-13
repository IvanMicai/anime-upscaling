import type { Meta, StoryObj } from "@storybook/nextjs-vite";
import { StatusBadge } from "./status-badge";

const meta: Meta<typeof StatusBadge> = {
  title: "Feature/StatusBadge",
  component: StatusBadge,
  parameters: { layout: "centered" },
  tags: ["autodocs"],
  argTypes: {
    status: {
      control: "select",
      options: ["queued", "running", "completed", "failed", "cancelled"],
    },
  },
};

export default meta;
type Story = StoryObj<typeof StatusBadge>;

export const Queued: Story = { args: { status: "queued" } };
export const Running: Story = { args: { status: "running" } };
export const Completed: Story = { args: { status: "completed" } };
export const Failed: Story = { args: { status: "failed" } };
export const Cancelled: Story = { args: { status: "cancelled" } };

export const All: Story = {
  render: () => (
    <div className="flex flex-wrap gap-2">
      <StatusBadge status="queued" />
      <StatusBadge status="running" />
      <StatusBadge status="completed" />
      <StatusBadge status="failed" />
      <StatusBadge status="cancelled" />
    </div>
  ),
};
