import type { Meta, StoryObj } from "@storybook/nextjs-vite";
import { expect, fn, waitFor, within } from "storybook/test";
import { MergePreviewPanel } from "./merge-preview-panel";

const meta: Meta<typeof MergePreviewPanel> = {
  title: "Feature/MergePreviewPanel",
  component: MergePreviewPanel,
  parameters: { layout: "padded" },
  tags: ["autodocs"],
  args: {
    source: "input", path: "", files: [], dirs: ["Show/EN", "Show/PT"],
    onLanguages: fn(), video: "auto", onPickVideo: fn(),
  },
  decorators: [
    (Story) => (
      <div className="w-full max-w-3xl">
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof MergePreviewPanel>;

// Two folders picked: what pairs up, what is already merged, what is left out.
export const TwoFolders: Story = {
  play: async ({ canvasElement, args }) => {
    const canvas = within(canvasElement);
    await waitFor(() => expect(canvas.getByText(/2 par\(es\)/)).toBeVisible());
    await expect(canvas.getByText(/1 já existe/)).toBeVisible();
    await expect(canvas.getByText(/1 arquivo\(s\) sem par/)).toBeVisible();
    // The languages found feed the "which file supplies the picture" choice.
    await expect(args.onLanguages).toHaveBeenCalledWith(["en", "pt-br"]);
  },
};
