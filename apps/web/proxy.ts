import { NextRequest, NextResponse } from "next/server";
import { COOKIE_NAME, isValidSession } from "@/lib/auth";

export async function proxy(req: NextRequest) {
  const session = req.cookies.get(COOKIE_NAME)?.value;
  if (!(await isValidSession(session))) {
    return NextResponse.redirect(new URL("/login", req.url));
  }
  return NextResponse.next();
}

export const config = {
  matcher: [
    /*
     * Match all paths except:
     * - /login
     * - /api/login
     * - /api/logout
     * - /_next (static assets, HMR)
     * - /favicon.ico
     */
    "/((?!login|api/login|api/logout|_next|favicon\\.ico).*)",
  ],
};
