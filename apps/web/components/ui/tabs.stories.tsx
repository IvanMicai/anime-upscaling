import type { Meta, StoryObj } from "@storybook/nextjs-vite";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "./tabs";

const meta: Meta<typeof Tabs> = {
  title: "UI/Tabs",
  component: Tabs,
  parameters: { layout: "padded" },
  tags: ["autodocs"],
};

export default meta;
type Story = StoryObj<typeof Tabs>;

export const Default: Story = {
  render: () => (
    <Tabs defaultValue="input" className="w-96">
      <TabsList>
        <TabsTrigger value="input">Input</TabsTrigger>
        <TabsTrigger value="output">Output</TabsTrigger>
        <TabsTrigger value="optimized">Optimized</TabsTrigger>
      </TabsList>
      <TabsContent value="input">Files in the input folder.</TabsContent>
      <TabsContent value="output">Upscaled videos.</TabsContent>
      <TabsContent value="optimized">Re-encoded videos.</TabsContent>
    </Tabs>
  ),
};

export const LineVariant: Story = {
  render: () => (
    <Tabs defaultValue="all" className="w-96">
      <TabsList variant="line">
        <TabsTrigger value="all">All</TabsTrigger>
        <TabsTrigger value="gpu">GPU 0</TabsTrigger>
        <TabsTrigger value="ffmpeg">FFMPEG</TabsTrigger>
      </TabsList>
      <TabsContent value="all">Showing all log streams.</TabsContent>
      <TabsContent value="gpu">GPU stream.</TabsContent>
      <TabsContent value="ffmpeg">FFMPEG stream.</TabsContent>
    </Tabs>
  ),
};

export const Vertical: Story = {
  render: () => (
    <Tabs defaultValue="general" orientation="vertical" className="h-48">
      <TabsList>
        <TabsTrigger value="general">General</TabsTrigger>
        <TabsTrigger value="gpu">GPU</TabsTrigger>
        <TabsTrigger value="ffmpeg">FFMPEG</TabsTrigger>
      </TabsList>
      <TabsContent value="general">General settings.</TabsContent>
      <TabsContent value="gpu">GPU settings.</TabsContent>
      <TabsContent value="ffmpeg">FFMPEG settings.</TabsContent>
    </Tabs>
  ),
};
