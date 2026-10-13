import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/nextjs-vite";
import { expect, fn, userEvent, within } from "storybook/test";
import { MergeFields, MERGE_DEFAULTS } from "./merge-fields";
import type { MergeConfig } from "@/lib/types";

const meta: Meta<typeof MergeFields> = {
  title: "Feature/MergeFields",
  component: MergeFields,
  parameters: { layout: "padded" },
  tags: ["autodocs"],
  args: { onChange: fn(), config: MERGE_DEFAULTS, languages: [] },
  decorators: [
    (Story) => (
      <div className="w-full max-w-2xl">
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof MergeFields>;

function Controlled(props: React.ComponentProps<typeof MergeFields>) {
  const [config, setConfig] = useState<MergeConfig>(props.config);
  return (
    <MergeFields
      {...props}
      config={config}
      onChange={(patch) => {
        setConfig((prev) => ({ ...prev, ...patch }));
        props.onChange(patch);
      }}
    />
  );
}

// Before any file is picked the languages are unknown: only "Automático".
export const NoSelectionYet: Story = {
  render: (args) => <Controlled {...args} />,
};

// Once the selection is known, the picture can be chosen by language name —
// the way out when the higher-resolution file is an upscale that looks worse.
export const WithLanguages: Story = {
  args: { languages: ["en", "pt-br"] },
  render: (args) => <Controlled {...args} />,
  play: async ({ canvasElement, args }) => {
    const canvas = within(canvasElement);
    await userEvent.click(canvas.getAllByRole("button", { name: /\[en\]/ })[0]);
    await expect(args.onChange).toHaveBeenCalledWith({ video: "en" });
    await userEvent.click(canvas.getByRole("button", { name: /2 s/ }));
    await expect(args.onChange).toHaveBeenCalledWith({ tick: 2 });
  },
};

export const Forced: Story = {
  args: { languages: ["en", "pt-br"], config: { ...MERGE_DEFAULTS, force: true, tick: 1, gapFill: "silence" } },
  render: (args) => <Controlled {...args} />,
};
