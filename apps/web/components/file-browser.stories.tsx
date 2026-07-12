import type { Meta, StoryObj } from "@storybook/nextjs-vite";
import { FileBrowser } from "./file-browser";

const meta: Meta<typeof FileBrowser> = {
  title: "Feature/FileBrowser",
  component: FileBrowser,
  parameters: {
    layout: "padded",
    nextjs: { appDirectory: true, navigation: { pathname: "/files" } },
  },
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <div className="w-full max-w-6xl">
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof FileBrowser>;

export const Default: Story = {};
