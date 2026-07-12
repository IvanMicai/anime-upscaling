"use client";

import { FileBrowser } from "@/components/file-browser";

export default function FilesPage() {
  return (
    <div className="flex h-[calc(100vh-10rem)] flex-col">
      <h2 className="mb-3 text-xl font-bold">Files</h2>
      <div className="flex min-h-0 flex-1 flex-col">
        <FileBrowser />
      </div>
    </div>
  );
}
