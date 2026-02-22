import { cookies } from "next/headers";
import { NextRequest, NextResponse } from "next/server";
import { COOKIE_NAME, isValidSession } from "@/lib/auth";

const API_URL = process.env.API_URL || "http://localhost:4751";

async function proxy(req: NextRequest) {
  const cookieStore = await cookies();
  const session = cookieStore.get(COOKIE_NAME)?.value;
  if (!(await isValidSession(session))) {
    return NextResponse.json({ error: "unauthorized" }, { status: 401 });
  }

  const path = req.nextUrl.pathname; // e.g. /api/jobs
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
  const body = await upstream.text();
  return new Response(body, {
    status: upstream.status,
    headers: { "Content-Type": ct },
  });
}

export const GET = proxy;
export const POST = proxy;
export const DELETE = proxy;

export async function OPTIONS() {
  return new Response(null, { status: 204 });
}
