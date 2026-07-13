import { createReadStream } from "node:fs";
import { stat } from "node:fs/promises";
import { join } from "node:path";
import { Readable } from "node:stream";
import { cookies } from "next/headers";
import { NextRequest, NextResponse } from "next/server";
import { COOKIE_NAME, isValidSession } from "@/lib/auth";

const API_URL = process.env.API_URL || "http://localhost:4751";
const PROCESS_DIR = process.env.PROCESS_DIR || "/data";

async function proxy(req: NextRequest) {
  const cookieStore = await cookies();
  const session = cookieStore.get(COOKIE_NAME)?.value;
  if (!(await isValidSession(session))) {
    return NextResponse.json({ error: "unauthorized" }, { status: 401 });
  }

  const path = req.nextUrl.pathname; // e.g. /api/jobs

  // Binary download: serve from local filesystem to avoid proxying multi-GB
  // bodies through undici+Next's standalone server (which buffers and OOMs the
  // 1g app container). The /data volume must be mounted into the app container.
  if (req.method === "GET" && path === "/api/files/download") {
    return handleDownload(req);
  }

  const search = req.nextUrl.search; // e.g. ?dir=input
  const url = `${API_URL}${path}${search}`;

  const headers = new Headers();
  headers.set("Content-Type", req.headers.get("Content-Type") || "application/json");

  const upstream = await fetch(url, {
    method: req.method,
    headers,
    body: req.body,
    // @ts-expect-error -- Node fetch supports duplex for streaming request bodies
    duplex: "half",
  });

  // SSE: stream through without buffering
  const ct = upstream.headers.get("content-type") || "";
  if (ct.includes("text/event-stream")) {
    return new Response(upstream.body, {
      status: upstream.status,
      headers: {
        "Content-Type": "text/event-stream",
        "Cache-Control": "no-cache",
        Connection: "keep-alive",
      },
    });
  }

  // Regular JSON response
  // 101/204/205/304 são "null body status codes" — Response() lança se receber body
  if (upstream.status === 204 || upstream.status === 205 || upstream.status === 304) {
    return new Response(null, { status: upstream.status });
  }
  const body = await upstream.text();
  return new Response(body, {
    status: upstream.status,
    headers: { "Content-Type": ct },
  });
}

export const GET = proxy;
export const POST = proxy;
export const PUT = proxy;
export const DELETE = proxy;

export async function OPTIONS() {
  return new Response(null, { status: 204 });
}

const DIR_MAP: Record<string, string> = {
  input: "input",
  output: "output",
  optimized: "optimized",
  interpolated: "interpolated",
};

const VIDEO_EXTS = [".mkv", ".mp4", ".avi"];

function safeRelDir(rel: string): boolean {
  if (rel === "") return true;
  if (rel.includes("\\") || rel.startsWith("/")) return false;
  for (const seg of rel.split("/")) {
    if (seg === "" || seg === "." || seg === "..") return false;
  }
  return true;
}

function safeVideoFilename(name: string): boolean {
  if (!name || name.includes("/") || name.includes("\\") || name.includes("..")) {
    return false;
  }
  const lower = name.toLowerCase();
  const dot = lower.lastIndexOf(".");
  if (dot < 0) return false;
  return VIDEO_EXTS.includes(lower.slice(dot));
}

async function handleDownload(req: NextRequest): Promise<Response> {
  const params = req.nextUrl.searchParams;
  const dir = params.get("dir") || "";
  const name = params.get("name") || "";
  const subPath = params.get("path") || "";

  const subDir = DIR_MAP[dir];
  if (!subDir) {
    return NextResponse.json({ error: "invalid dir" }, { status: 400 });
  }
  if (!safeVideoFilename(name)) {
    return NextResponse.json({ error: "invalid filename" }, { status: 400 });
  }
  if (!safeRelDir(subPath)) {
    return NextResponse.json({ error: "invalid path" }, { status: 400 });
  }

  const fullPath = join(PROCESS_DIR, subDir, subPath, name);

  let size: number;
  try {
    const info = await stat(fullPath);
    if (!info.isFile()) {
      return NextResponse.json({ error: "file not found" }, { status: 404 });
    }
    size = info.size;
  } catch {
    return NextResponse.json({ error: "file not found" }, { status: 404 });
  }

  const nodeStream = createReadStream(fullPath);
  const webStream = Readable.toWeb(nodeStream) as ReadableStream<Uint8Array>;

  return new Response(webStream, {
    headers: {
      "Content-Type": "application/octet-stream",
      "Content-Disposition": `attachment; filename="${name}"`,
      "Content-Length": String(size),
    },
  });
}
