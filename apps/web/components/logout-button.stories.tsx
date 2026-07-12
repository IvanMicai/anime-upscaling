import type { Meta, StoryObj } from "@storybook/nextjs-vite";
import { LogoutButton } from "./logout-button";

const meta: Meta<typeof LogoutButton> = {
  title: "Feature/LogoutButton",
  component: LogoutButton,
  parameters: {
    layout: "centered",
    nextjs: { appDirectory: true },
  },
  tags: ["autodocs"],
};

export default meta;
type Story = StoryObj<typeof LogoutButton>;

export const Default: Story = {};
