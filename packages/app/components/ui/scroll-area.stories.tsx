import type { Meta, StoryObj } from "@storybook/nextjs-vite";
import { ScrollArea } from "./scroll-area";

const meta: Meta<typeof ScrollArea> = {
  title: "UI/ScrollArea",
  component: ScrollArea,
  parameters: { layout: "centered" },
  tags: ["autodocs"],
};

export default meta;
type Story = StoryObj<typeof ScrollArea>;

const lines = Array.from({ length: 60 }, (_, i) => `Frame ${i + 1} — INFO log line content`);

export const Vertical: Story = {
  render: () => (
    <ScrollArea className="h-60 w-72 rounded-md border p-3 font-mono text-xs">
      {lines.map((l) => (
        <div key={l}>{l}</div>
      ))}
    </ScrollArea>
  ),
};

export const Horizontal: Story = {
  render: () => (
    <ScrollArea className="w-72 whitespace-nowrap rounded-md border p-3">
      <div className="flex gap-2">
        {Array.from({ length: 30 }).map((_, i) => (
          <div key={i} className="h-20 w-20 shrink-0 rounded bg-muted" />
        ))}
      </div>
    </ScrollArea>
  ),
};

export const ShortContent: Story = {
  render: () => (
    <ScrollArea className="h-60 w-72 rounded-md border p-3">
      <p className="text-sm">
        Content fits without scrolling — scrollbar should not appear.
      </p>
    </ScrollArea>
  ),
};
