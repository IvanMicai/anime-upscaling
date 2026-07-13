import type { Meta, StoryObj } from "@storybook/nextjs-vite";
import { NavLink } from "./nav-link";

const meta: Meta<typeof NavLink> = {
  title: "Feature/NavLink",
  component: NavLink,
  parameters: {
    layout: "padded",
    nextjs: { appDirectory: true, navigation: { pathname: "/jobs" } },
  },
  tags: ["autodocs"],
};

export default meta;
type Story = StoryObj<typeof NavLink>;

export const Inactive: Story = {
  parameters: { nextjs: { navigation: { pathname: "/" } } },
  args: { href: "/jobs", children: "Jobs" },
};

export const Active: Story = {
  parameters: { nextjs: { navigation: { pathname: "/jobs" } } },
  args: { href: "/jobs", children: "Jobs" },
};

export const ActiveByPrefix: Story = {
  parameters: { nextjs: { navigation: { pathname: "/jobs/abc-123" } } },
  args: { href: "/jobs", children: "Jobs", matchPrefixes: ["/jobs"] },
};

export const NavRow: Story = {
  parameters: { nextjs: { navigation: { pathname: "/pipelines" } } },
  render: () => (
    <nav className="flex items-center gap-4">
      <NavLink href="/">Home</NavLink>
      <NavLink href="/jobs">Jobs</NavLink>
      <NavLink href="/pipelines">Pipelines</NavLink>
      <NavLink href="/files">Files</NavLink>
      <NavLink href="/settings">Settings</NavLink>
    </nav>
  ),
};
