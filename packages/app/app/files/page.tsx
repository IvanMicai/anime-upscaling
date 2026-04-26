"use client";

import Link from "next/link";
import { FileBrowser } from "@/components/file-browser";

export default function FilesPage() {
  return (
    <div className="flex flex-col h-[calc(100vh-8rem)] sm:h-[calc(100vh-12rem)]">
      <Link href="/" className="text-sm text-blue-400 hover:underline">
        &larr; Back to Jobs
      </Link>
      <div className="flex flex-col flex-1 min-h-0 mt-6">
        <FileBrowser />
      </div>
    </div>
  );
}
