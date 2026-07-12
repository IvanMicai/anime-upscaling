export const COOKIE_NAME = "animeup_session";

export async function computeToken(): Promise<string> {
  const password = process.env.AUTH_PASSWORD ?? "";
  const secret = process.env.AUTH_SECRET ?? "";
  const data = new TextEncoder().encode(password + secret);
  const hash = await crypto.subtle.digest("SHA-256", data);
  return Array.from(new Uint8Array(hash))
    .map((b) => b.toString(16).padStart(2, "0"))
    .join("");
}

export async function isValidSession(
  cookieValue: string | undefined,
): Promise<boolean> {
  if (!process.env.AUTH_PASSWORD) return true;
  if (!cookieValue) return false;
  const expected = await computeToken();
  return cookieValue === expected;
}
