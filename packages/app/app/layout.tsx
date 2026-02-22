import type { Metadata } from "next";
import Link from "next/link";
import { Geist, Geist_Mono } from "next/font/google";
import { LogoutButton } from "@/components/logout-button";
import "./globals.css";

const geistSans = Geist({
  variable: "--font-geist-sans",
  subsets: ["latin"],
});

const geistMono = Geist_Mono({
  variable: "--font-geist-mono",
  subsets: ["latin"],
});

export const metadata: Metadata = {
  title: "AnimeUp",
  description: "Video Processing Dashboard",
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html lang="en" className="dark">
      <body
        className={`${geistSans.variable} ${geistMono.variable} antialiased`}
      >
        <div className="mx-auto max-w-5xl px-4 py-8">
          <header className="mb-8 flex items-center justify-between">
            <div>
              <h1 className="text-2xl font-bold">AnimeUp</h1>
              <p className="text-sm text-muted-foreground">
                Video Processing Dashboard
              </p>
            </div>
            <div className="flex items-center gap-4">
              <Link
                href="/sources"
                className="text-sm text-muted-foreground hover:text-foreground transition-colors"
              >
                Sources
              </Link>
              <LogoutButton />
            </div>
          </header>
          {children}
        </div>
      </body>
    </html>
  );
}
