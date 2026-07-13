import type { Meta, StoryObj } from "@storybook/nextjs-vite";
import { fn } from "storybook/test";
import { Breadcrumbs } from "./breadcrumbs";

const meta: Meta<typeof Breadcrumbs> = {
  title: "Feature/Breadcrumbs",
  component: Breadcrumbs,
  parameters: { layout: "padded" },
  tags: ["autodocs"],
  args: { onNavigate: fn() },
};

export default meta;
type Story = StoryObj<typeof Breadcrumbs>;

export const Root: Story = { args: { path: "" } };
export const OneLevel: Story = { args: { path: "season-01" } };
export const Nested: Story = { args: { path: "season-01/specials/extras" } };
export const Deep: Story = { args: { path: "anime/season-01/episodes/specials/raw" } };
