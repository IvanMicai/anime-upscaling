import type { Meta, StoryObj } from "@storybook/nextjs-vite";
import { Checkbox } from "./checkbox";
import { Label } from "./label";

const meta: Meta<typeof Label> = {
  title: "UI/Label",
  component: Label,
  parameters: { layout: "centered" },
  tags: ["autodocs"],
};

export default meta;
type Story = StoryObj<typeof Label>;

export const Default: Story = {
  render: () => <Label htmlFor="name">Pipeline name</Label>,
};

export const WithCheckbox: Story = {
  render: () => (
    <Label className="flex items-center gap-2">
      <Checkbox defaultChecked /> Run automatically when files arrive
    </Label>
  ),
};

export const WithDisabledControl: Story = {
  render: () => (
    <div className="group" data-disabled="true">
      <Label className="flex items-center gap-2">
        <Checkbox disabled /> Disabled state — label dims
      </Label>
    </div>
  ),
};
