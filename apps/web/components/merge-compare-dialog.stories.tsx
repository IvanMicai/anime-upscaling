import type { Meta, StoryObj } from "@storybook/nextjs-vite";
import { expect, fn, userEvent, waitFor, within } from "storybook/test";
import { MergeCompareDialog } from "./merge-compare-dialog";

// Stand-in frames: a sharp and a soft rendering of the same "scene", so the
// divider has something to show.
function fakeFrame(_source: string, file: string, t: number): string {
  const soft = file.includes("pt-br");
  const svg = `<svg xmlns="http://www.w3.org/2000/svg" width="640" height="480">
    <defs><filter id="b"><feGaussianBlur stdDeviation="${soft ? 2.2 : 0}"/></filter></defs>
    <rect width="640" height="480" fill="#1b2a4a"/>
    <g filter="url(#b)">
      <circle cx="320" cy="240" r="150" fill="#f5c542"/>
      <path d="M170 240h300M320 90v300" stroke="#1b2a4a" stroke-width="6"/>
      <text x="320" y="252" font-size="34" font-family="monospace" text-anchor="middle" fill="#1b2a4a">t=${Math.round(t)}s</text>
    </g></svg>`;
  return `data:image/svg+xml;utf8,${encodeURIComponent(svg)}`;
}

const pair = {
  a: "Show/EN/ep01.en.mkv", b: "Show/PT/ep01.pt-br.mp4", output: "Show/ep01.mkv",
  lang_a: { tag: "en", iso3: "eng", title: "English" },
  lang_b: { tag: "pt-br", iso3: "por", title: "Português (BR)" },
};

const meta: Meta<typeof MergeCompareDialog> = {
  title: "Feature/MergeCompareDialog",
  component: MergeCompareDialog,
  parameters: { layout: "fullscreen" },
  args: { open: true, source: "input", pair, onOpenChange: fn(), onPick: fn(), frameUrl: fakeFrame },
};

export default meta;
type Story = StoryObj<typeof MergeCompareDialog>;

// The second frame is taken where the same moment falls in the other file —
// here 10.5 s earlier — not at the same timestamp.
export const SameMomentFound: Story = {
  play: async ({ args }) => {
    const body = within(document.body);
    await waitFor(() => expect(body.getByText(/\[en\] 0:30 ↔ \[pt-br\] 0:20 \(-10\.5 s\)/)).toBeVisible());
    await expect(body.getByText(/640×480 · 1\.2 Mbps/)).toBeVisible();
    await expect(body.getByText(/1280×960 · 1\.9 Mbps/)).toBeVisible();
    // Seek points appear once the duration is known.
    await userEvent.click(await body.findByRole("button", { name: "11:11" }));
    await waitFor(() => expect(body.getByText(/\[en\] 11:11 ↔ \[pt-br\] 11:01/)).toBeVisible());
    await userEvent.click(body.getByRole("button", { name: "Usar o vídeo de [en]" }));
    await expect(args.onPick).toHaveBeenCalledWith("en");
    await expect(args.onOpenChange).toHaveBeenCalledWith(false);
  },
};

// Dialogue-only stretch, or content the other file lacks: say so instead of
// showing two different scenes as if they were the same.
export const MomentNotFound: Story = {
  play: async () => {
    const body = within(document.body);
    await userEvent.click(await body.findByRole("button", { name: "20:08" }));
    await waitFor(() => expect(body.getByText(/Não consegui achar este momento/)).toBeVisible());
  },
};
