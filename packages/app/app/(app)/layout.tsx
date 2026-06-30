import Link from "next/link";
import Image from "next/image";
import { LogoutButton } from "@/components/logout-button";
import { NavLink } from "@/components/nav-link";
import { SystemStatusBar } from "@/components/system-status-bar";

export default function AppLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <div className="mx-auto max-w-6xl px-4 py-4 sm:py-8">
      <header className="mb-3 flex flex-col gap-3 border-b border-border pb-3 sm:flex-row sm:items-center sm:justify-between sm:rounded-xl sm:border sm:border-border sm:bg-card/50 sm:px-3 sm:py-2.5 sm:pb-2.5">
        <Link href="/" className="flex items-center gap-3">
          <Image
            src="/logo.png"
            alt="AnimeUp"
            width={600}
            height={394}
            priority
            unoptimized
            className="h-9 w-auto shrink-0"
          />
          <span className="leading-tight">
            <span className="block text-base font-bold">AnimeUp</span>
            <span className="block text-xs text-muted-foreground">
              Video Processing
            </span>
          </span>
        </Link>
        <nav className="flex items-center gap-1">
          <NavLink href="/" matchPrefixes={["/jobs"]}>
            Jobs
          </NavLink>
          <NavLink href="/pipelines">Pipelines</NavLink>
          <NavLink href="/files">Files</NavLink>
          <NavLink href="/settings">Settings</NavLink>
          <LogoutButton />
        </nav>
      </header>
      <SystemStatusBar />
      <main>{children}</main>
    </div>
  );
}
