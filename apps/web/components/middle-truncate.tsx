import { cn } from "@/lib/utils";

/**
 * Truncates text in the middle (start…end), keeping the last `tail` characters
 * visible so a file extension survives. CSS-only ellipsis truncates at the end,
 * so we split into a shrinkable head and a fixed tail inside a flex row. The
 * head shrinks first, placing the ellipsis just before the tail.
 */
export function MiddleTruncate({
  text,
  tail = 12,
  className,
}: {
  text: string;
  tail?: number;
  className?: string;
}) {
  const cut = Math.max(0, text.length - tail);
  const head = text.slice(0, cut);
  const end = text.slice(cut);
  return (
    <span className={cn("flex min-w-0", className)} title={text}>
      <span className="min-w-0 truncate">{head}</span>
      <span className="shrink-0 whitespace-pre">{end}</span>
    </span>
  );
}
