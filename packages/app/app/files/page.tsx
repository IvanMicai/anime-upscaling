"use client";

import Link from "next/link";
import { FileBrowser } from "@/components/file-browser";

export default function FilesPage() {
  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-4">
          <Link href="/" className="text-sm text-blue-400 hover:underline">
            &larr; Back to Jobs
          </Link>
        </div>
      </div>
      <FileBrowser />
    </div>
  );
}
