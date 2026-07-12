import type { Meta, StoryObj } from "@storybook/nextjs-vite";
import {
  Table,
  TableBody,
  TableCaption,
  TableCell,
  TableFooter,
  TableHead,
  TableHeader,
  TableRow,
} from "./table";

const meta: Meta<typeof Table> = {
  title: "UI/Table",
  component: Table,
  parameters: { layout: "padded" },
  tags: ["autodocs"],
};

export default meta;
type Story = StoryObj<typeof Table>;

export const Default: Story = {
  render: () => (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>File</TableHead>
          <TableHead>Resolution</TableHead>
          <TableHead className="text-right">Size</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        <TableRow>
          <TableCell className="font-mono">ep01.mkv</TableCell>
          <TableCell>1920×1080</TableCell>
          <TableCell className="text-right">1.7 GB</TableCell>
        </TableRow>
        <TableRow>
          <TableCell className="font-mono">ep02.mkv</TableCell>
          <TableCell>1920×1080</TableCell>
          <TableCell className="text-right">1.8 GB</TableCell>
        </TableRow>
        <TableRow>
          <TableCell className="font-mono">ep03.mkv</TableCell>
          <TableCell>1920×1080</TableCell>
          <TableCell className="text-right">1.6 GB</TableCell>
        </TableRow>
      </TableBody>
    </Table>
  ),
};

export const WithFooterAndCaption: Story = {
  render: () => (
    <Table>
      <TableCaption>Season 01 — input folder</TableCaption>
      <TableHeader>
        <TableRow>
          <TableHead>File</TableHead>
          <TableHead className="text-right">Size</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        <TableRow>
          <TableCell>ep01.mkv</TableCell>
          <TableCell className="text-right">1.7 GB</TableCell>
        </TableRow>
        <TableRow>
          <TableCell>ep02.mkv</TableCell>
          <TableCell className="text-right">1.8 GB</TableCell>
        </TableRow>
      </TableBody>
      <TableFooter>
        <TableRow>
          <TableCell>Total</TableCell>
          <TableCell className="text-right">3.5 GB</TableCell>
        </TableRow>
      </TableFooter>
    </Table>
  ),
};

export const Empty: Story = {
  render: () => (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>File</TableHead>
          <TableHead>Size</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        <TableRow>
          <TableCell colSpan={2} className="text-center text-muted-foreground">
            No files in this folder.
          </TableCell>
        </TableRow>
      </TableBody>
    </Table>
  ),
};
