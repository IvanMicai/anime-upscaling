import type { Meta, StoryObj } from "@storybook/nextjs-vite";
import { Separator } from "./separator";

const meta: Meta<typeof Separator> = {
  title: "UI/Separator",
  component: Separator,
  parameters: { layout: "centered" },
  tags: ["autodocs"],
  argTypes: {
    orientation: { control: "radio", options: ["horizontal", "vertical"] },
  },
};

export default meta;
type Story = StoryObj<typeof Separator>;

export const Horizontal: Story = {
  render: () => (
    <div className="w-72">
      <p className="text-sm">Above the separator</p>
      <Separator className="my-3" />
      <p className="text-sm">Below the separator</p>
    </div>
  ),
};

export const Vertical: Story = {
  render: () => (
    <div className="flex h-12 items-center text-sm">
      <span>Left</span>
      <Separator orientation="vertical" className="mx-3" />
      <span>Middle</span>
      <Separator orientation="vertical" className="mx-3" />
      <span>Right</span>
    </div>
  ),
};
