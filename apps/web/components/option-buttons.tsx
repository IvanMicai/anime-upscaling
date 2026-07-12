"use client";

import { cn } from "@/lib/utils";

export interface Option<T extends string | number> {
  value: T;
  label: string;
  desc?: string;
}

/**
 * Segmented button selector — a friendlier replacement for a <Select> when the
 * option set is small (scale, noise, source folder, job type). Renders a grid
 * of toggle buttons; the active one is highlighted with the primary accent.
 */
export function OptionButtons<T extends string | number>({
  value,
  options,
  onChange,
  columns = 2,
  className,
}: {
  value: T;
  options: readonly Option<T>[];
  onChange: (value: T) => void;
  columns?: number;
  className?: string;
}) {
  return (
    <div
      className={cn("grid gap-2", className)}
      style={{ gridTemplateColumns: `repeat(${columns}, minmax(0, 1fr))` }}
    >
      {options.map((opt) => {
        const active = opt.value === value;
        return (
          <button
            key={opt.value}
            type="button"
            onClick={() => onChange(opt.value)}
            aria-pressed={active}
            className={cn(
              "flex flex-col items-start gap-0.5 rounded-md border px-3 py-2 text-left text-sm transition-colors",
              active
                ? "border-primary/60 bg-primary/10 text-foreground"
                : "border-border text-muted-foreground hover:bg-secondary/50 hover:text-foreground",
            )}
          >
            <span className="font-medium">{opt.label}</span>
            {opt.desc && (
              <span className="text-xs text-muted-foreground">{opt.desc}</span>
            )}
          </button>
        );
      })}
    </div>
  );
}
