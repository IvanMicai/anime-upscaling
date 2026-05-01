import type { Meta, StoryObj } from "@storybook/nextjs-vite";
import { Search, Trash2, Plus } from "lucide-react";
import { Button } from "./button";

const meta: Meta<typeof Button> = {
  title: "UI/Button",
  component: Button,
  parameters: { layout: "centered" },
  tags: ["autodocs"],
  argTypes: {
    variant: {
      control: "select",
      options: ["default", "destructive", "outline", "secondary", "ghost", "link"],
    },
    size: {
      control: "select",
      options: ["default", "xs", "sm", "lg", "icon", "icon-xs", "icon-sm", "icon-lg"],
    },
    disabled: { control: "boolean" },
    asChild: { control: "boolean" },
  },
  args: {
    children: "Button",
    variant: "default",
    size: "default",
  },
};

export default meta;
type Story = StoryObj<typeof Button>;

export const Default: Story = {};

export const Destructive: Story = { args: { variant: "destructive", children: "Delete" } };
export const Outline: Story = { args: { variant: "outline" } };
export const Secondary: Story = { args: { variant: "secondary" } };
export const Ghost: Story = { args: { variant: "ghost" } };
export const Link: Story = { args: { variant: "link", children: "Read more" } };

export const AllVariants: Story = {
  render: () => (
    <div className="flex flex-wrap gap-3">
      <Button>Default</Button>
      <Button variant="destructive">Destructive</Button>
      <Button variant="outline">Outline</Button>
      <Button variant="secondary">Secondary</Button>
      <Button variant="ghost">Ghost</Button>
      <Button variant="link">Link</Button>
    </div>
  ),
};

export const AllSizes: Story = {
  render: () => (
    <div className="flex flex-wrap items-center gap-3">
      <Button size="xs">XS</Button>
      <Button size="sm">Small</Button>
      <Button size="default">Default</Button>
      <Button size="lg">Large</Button>
    </div>
  ),
};

export const IconSizes: Story = {
  render: () => (
    <div className="flex flex-wrap items-center gap-3">
      <Button size="icon-xs" aria-label="Add"><Plus /></Button>
      <Button size="icon-sm" aria-label="Add"><Plus /></Button>
      <Button size="icon" aria-label="Add"><Plus /></Button>
      <Button size="icon-lg" aria-label="Add"><Plus /></Button>
    </div>
  ),
};

export const WithIcon: Story = {
  render: () => (
    <div className="flex flex-wrap items-center gap-3">
      <Button><Search /> Search</Button>
      <Button variant="destructive"><Trash2 /> Delete</Button>
      <Button variant="outline" size="sm"><Plus /> Add step</Button>
    </div>
  ),
};

export const Disabled: Story = { args: { disabled: true } };

export const AsChild: Story = {
  args: { asChild: true },
  render: (args) => (
    <Button {...args}>
      <a href="#">Anchor styled as button</a>
    </Button>
  ),
};
