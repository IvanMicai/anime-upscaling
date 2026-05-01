import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/nextjs-vite";
import { fn } from "storybook/test";
import { FilePicker } from "./file-picker";

const meta: Meta<typeof FilePicker> = {
  title: "Feature/FilePicker",
  component: FilePicker,
  parameters: {
    layout: "padded",
    nextjs: { appDirectory: true },
  },
  tags: ["autodocs"],
  args: { onChange: fn() },
  decorators: [
    (Story) => (
      <div className="w-full max-w-5xl">
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof FilePicker>;

function ControlledPicker(props: React.ComponentProps<typeof FilePicker>) {
  const [selected, setSelected] = useState<string[]>(props.selected);
  return (
    <FilePicker
      {...props}
      selected={selected}
      onChange={(s) => {
        setSelected(s);
        props.onChange(s);
      }}
    />
  );
}

export const Empty: Story = {
  args: { selected: [], dir: "input" },
  render: (args) => <ControlledPicker {...args} />,
};

export const WithSelection: Story = {
  args: { selected: ["ep01.mkv", "ep02.mkv"], dir: "input" },
  render: (args) => <ControlledPicker {...args} />,
};

export const OutputFolder: Story = {
  args: { selected: [], dir: "output" },
  render: (args) => <ControlledPicker {...args} />,
};

export const NestedPath: Story = {
  args: { selected: [], dir: "input", path: "season-01" },
  render: (args) => <ControlledPicker {...args} />,
};

export const WithScroll: Story = {
  args: { selected: [], dir: "input", path: "season-long" },
  parameters: {
    docs: {
      description: {
        story:
          "36 files plus subfolders inside a fixed-height container so the table actually scrolls. Use this to verify that the column headers and the Total footer remain visible while scrolling.",
      },
    },
  },
  decorators: [
    (Story) => (
      <div className="h-[520px] w-full max-w-5xl">
        <Story />
      </div>
    ),
  ],
  render: (args) => <ControlledPicker {...args} />,
};
