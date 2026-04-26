"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";

interface NavLinkProps {
  href: string;
  matchPrefixes?: string[];
  children: React.ReactNode;
}

function isUnder(pathname: string, prefix: string) {
  if (prefix === "/") return pathname === "/";
  return pathname === prefix || pathname.startsWith(`${prefix}/`);
}

export function NavLink({ href, matchPrefixes, children }: NavLinkProps) {
  const pathname = usePathname();
  const prefixes = matchPrefixes ?? [];
  const isActive =
    isUnder(pathname, href) || prefixes.some((p) => isUnder(pathname, p));

  return (
    <Link
      href={href}
      aria-current={isActive ? "page" : undefined}
      className={
        isActive
          ? "text-sm font-medium text-foreground"
          : "text-sm text-muted-foreground hover:text-foreground transition-colors"
      }
    >
      {children}
    </Link>
  );
}
