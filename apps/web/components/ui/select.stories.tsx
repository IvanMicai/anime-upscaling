import type { Meta, StoryObj } from "@storybook/nextjs-vite";
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectLabel,
  SelectSeparator,
  SelectTrigger,
  SelectValue,
} from "./select";

const meta: Meta<typeof Select> = {
  title: "UI/Select",
  component: Select,
  parameters: { layout: "centered" },
  tags: ["autodocs"],
};

export default meta;
type Story = StoryObj<typeof Select>;

export const Default: Story = {
  render: () => (
    <Select>
      <SelectTrigger className="w-56">
        <SelectValue placeholder="Pick a processor" />
      </SelectTrigger>
      <SelectContent>
        <SelectItem value="realesrgan">RealESRGAN</SelectItem>
        <SelectItem value="libplacebo">libplacebo</SelectItem>
        <SelectItem value="realcugan">RealCUGAN</SelectItem>
      </SelectContent>
    </Select>
  ),
};

export const Grouped: Story = {
  render: () => (
    <Select defaultValue="rife-v4.6">
      <SelectTrigger className="w-56">
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        <SelectGroup>
          <SelectLabel>Recommended</SelectLabel>
          <SelectItem value="rife-v4.6">RIFE v4.6</SelectItem>
          <SelectItem value="rife-v4.26">RIFE v4.26 (experimental)</SelectItem>
        </SelectGroup>
        <SelectSeparator />
        <SelectGroup>
          <SelectLabel>Legacy</SelectLabel>
          <SelectItem value="rife-v3.1">RIFE v3.1</SelectItem>
          <SelectItem value="rife-v2.4">RIFE v2.4</SelectItem>
        </SelectGroup>
      </SelectContent>
    </Select>
  ),
};

export const Disabled: Story = {
  render: () => (
    <Select disabled>
      <SelectTrigger className="w-56">
        <SelectValue placeholder="Disabled" />
      </SelectTrigger>
      <SelectContent>
        <SelectItem value="a">Option A</SelectItem>
      </SelectContent>
    </Select>
  ),
};
