import type { Meta, StoryObj } from "@storybook/nextjs-vite";
import { LogViewer } from "./log-viewer";
import { sampleLogs } from "./__fixtures__/jobs";
import type { LogEntry, LogLevel, LogSource } from "@/lib/types";

const meta: Meta<typeof LogViewer> = {
  title: "Feature/LogViewer",
  component: LogViewer,
  parameters: { layout: "padded" },
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <div className="w-full max-w-4xl">
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof LogViewer>;

export const Empty: Story = { args: { logs: [], connected: true } };

export const Streaming: Story = { args: { logs: sampleLogs, connected: true } };

export const Disconnected: Story = { args: { logs: sampleLogs, connected: false } };

const sources: LogSource[] = ["PIPELINE", "GPU 0", "GPU 1", "FFMPEG"];
const levels: LogLevel[] = ["INFO", "OK", "WARN", "ERRO", "SKIP", "STEP"];

const longLogs: LogEntry[] = Array.from({ length: 200 }, (_, i) => ({
  source: sources[i % sources.length],
  level: levels[i % levels.length],
  index: i,
  message: `Log line ${i}: processing frame ${i * 10}/100000 at ${(15 + (i % 10)).toFixed(1)} fps`,
  time: new Date(Date.now() - (200 - i) * 1000).toISOString(),
}));

export const LongStream: Story = { args: { logs: longLogs, connected: true } };

export const SingleSource: Story = {
  args: {
    logs: sampleLogs.filter((l) => l.source === "GPU 0"),
    connected: true,
  },
};
