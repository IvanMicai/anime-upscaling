import type { Metadata } from "next";
import { Geist, Geist_Mono } from "next/font/google";
import Link from "next/link";
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
              <Link href="/pipelines" className="text-sm text-muted-foreground hover:text-foreground transition-colors">
                Pipelines
              </Link>
              <Link href="/files" className="text-sm text-muted-foreground hover:text-foreground transition-colors">
                Files
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
