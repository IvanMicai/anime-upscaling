"use client";

import { cn } from "@/lib/utils";
import { FOLDER_COLORS, type FolderKey } from "@/lib/file-utils";

// Shared pieces of the two job-creation wizards (Upscaling and Merge).

export type WizardStep = 1 | 2 | 3;

// Labels come from FOLDER_COLORS so the source buttons stay in sync with the
// file-picker legend tags (e.g. "output" reads "Upscaling", not "Output").
export const SOURCE_OPTS = (["input", "merged", "output", "interpolated", "optimized"] as FolderKey[]).map(
  (value) => ({ value, label: FOLDER_COLORS[value].label }),
);

export function sourceLabel(source: FolderKey): string {
  return SOURCE_OPTS.find((s) => s.value === source)?.label ?? source;
}

export function WizardStepper({
  steps,
  step,
  onStep,
}: {
  steps: { n: WizardStep; label: string }[];
  step: WizardStep;
  onStep: (n: WizardStep) => void;
}) {
  return (
    <nav className="flex items-center gap-2 text-sm" aria-label="Etapas">
      {steps.map((s, i) => {
        const active = step === s.n;
        const done = step > s.n;
        // Only allow jumping to a step that's already been reached.
        const clickable = s.n <= step;
        return (
          <div key={s.n} className="flex items-center gap-2">
            {i > 0 && <span className="h-px w-6 bg-border sm:w-8" />}
            <button
              type="button"
              onClick={() => clickable && onStep(s.n)}
              disabled={!clickable}
              aria-current={active ? "step" : undefined}
              className={cn(
                "flex items-center gap-2 rounded-full border px-3 py-1.5 transition-colors",
                active
                  ? "border-primary/60 bg-primary/10 text-foreground"
                  : done
                    ? "border-border text-muted-foreground hover:bg-secondary/50 hover:text-foreground"
                    : "border-border text-muted-foreground",
                !clickable && "cursor-default",
              )}
            >
              <span
                className={cn(
                  "flex size-5 items-center justify-center rounded-full text-xs font-semibold",
                  active || done
                    ? "bg-primary/20 text-foreground"
                    : "bg-secondary text-muted-foreground",
                )}
              >
                {s.n}
              </span>
              <span className="hidden sm:inline">{s.label}</span>
            </button>
          </div>
        );
      })}
    </nav>
  );
}

export function SourceFolderPicker({
  value,
  onChange,
  exclude = [],
}: {
  value: FolderKey;
  onChange: (source: FolderKey) => void;
  exclude?: FolderKey[];
}) {
  return (
    <div className="space-y-1.5">
      <label className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
        Pasta de Origem
      </label>
      <div className="grid grid-cols-2 gap-2 sm:grid-cols-5">
        {SOURCE_OPTS.filter((o) => !exclude.includes(o.value)).map((opt) => {
          const active = value === opt.value;
          const colors = FOLDER_COLORS[opt.value];
          return (
            <button
              key={opt.value}
              type="button"
              onClick={() => onChange(opt.value)}
              aria-pressed={active}
              className={cn(
                "rounded-md border px-3 py-2 text-sm font-medium transition-colors",
                active
                  ? cn(colors.badge, "ring-2 ring-inset ring-white/20")
                  : cn(
                      "border-border bg-transparent hover:bg-secondary/50",
                      colors.text,
                    ),
              )}
            >
              {opt.label}
            </button>
          );
        })}
      </div>
    </div>
  );
}
