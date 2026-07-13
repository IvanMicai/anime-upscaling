import { NextRequest, NextResponse } from "next/server";
import { COOKIE_NAME, computeToken } from "@/lib/auth";

export async function POST(req: NextRequest) {
  const { password } = await req.json();
  const expected = process.env.AUTH_PASSWORD;

  if (!expected || password !== expected) {
    return NextResponse.json({ error: "wrong password" }, { status: 401 });
  }

  const token = await computeToken();
  const res = NextResponse.json({ ok: true });
  res.cookies.set(COOKIE_NAME, token, {
    httpOnly: true,
    sameSite: "lax",
    path: "/",
    maxAge: 60 * 60 * 24 * 30, // 30 days
  });
  return res;
}
