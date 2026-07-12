"use client";

import { Fragment } from "react";
import { cn } from "@/lib/utils";
import { getBreadcrumbs } from "@/lib/file-utils";

interface BreadcrumbsProps {
  path: string;
  onNavigate: (path: string) => void;
  className?: string;
}

export function Breadcrumbs({ path, onNavigate, className }: BreadcrumbsProps) {
  const crumbs = getBreadcrumbs(path);
  return (
    <nav className={cn("flex flex-wrap items-center gap-1 text-sm", className)} aria-label="Breadcrumb">
      {crumbs.map((c, i) => {
        const isLast = i === crumbs.length - 1;
        return (
          <Fragment key={c.path || "root"}>
            {i > 0 && <span className="text-muted-foreground/50">/</span>}
            <button
              type="button"
              onClick={() => onNavigate(c.path)}
              disabled={isLast}
              className={cn(
                "rounded px-1.5 py-0.5 font-mono transition-colors",
                isLast
                  ? "text-foreground cursor-default"
                  : "text-muted-foreground hover:bg-muted hover:text-foreground",
              )}
            >
              {c.label}
            </button>
          </Fragment>
        );
      })}
    </nav>
  );
}
