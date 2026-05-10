import type { Metadata } from "next";
import Link from "next/link";
import { LogoutButton } from "@/components/logout-button";
import { NavLink } from "@/components/nav-link";
import "./globals.css";

export const metadata: Metadata = {
  title: "AnimeUp",
  description: "Video Processing Dashboard",
  icons: {
    icon: [
      { url: "/favicon/favicon-96x96.png", type: "image/png", sizes: "96x96" },
      { url: "/favicon/favicon.svg", type: "image/svg+xml" },
    ],
    shortcut: "/favicon/favicon.ico",
    apple: { url: "/favicon/apple-touch-icon.png", sizes: "180x180" },
  },
  manifest: "/favicon/site.webmanifest",
  appleWebApp: {
    title: "Anime UP",
  },
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html lang="en" className="dark">
      <body className="antialiased">
        <div className="mx-auto max-w-5xl px-4 py-4 sm:py-8">
          <header className="mb-6 sm:mb-8 flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
            <div>
              <Link href="/">
                <h1 className="text-2xl font-bold">AnimeUp</h1>
              </Link>
              <p className="text-sm text-muted-foreground">
                Video Processing Dashboard
              </p>
            </div>
            <div className="flex items-center gap-3 sm:gap-4">
              <NavLink href="/" matchPrefixes={["/jobs"]}>
                Jobs
              </NavLink>
              <NavLink href="/pipelines">Pipelines</NavLink>
              <NavLink href="/files">Files</NavLink>
              <NavLink href="/settings">Settings</NavLink>
              <LogoutButton />
            </div>
          </header>
          {children}
        </div>
      </body>
    </html>
  );
}
