// Static Tailwind class strings must appear verbatim so the JIT picks them up.
//
// On mobile we drop the card "chrome" (border/background/rounding and horizontal
// padding) to save space; sections only get it back from the `sm:` breakpoint.
// Stacked sections are separated by a thin divider instead of stacked cards.

/**
 * A stacked page section: full card on >= sm; on mobile borderless and
 * edge-to-edge, with a bottom divider separating it from the next section.
 * Pair with the page container's vertical spacing.
 */
export const sectionCard =
  "border-b border-border pb-4 sm:rounded-lg sm:border sm:border-border sm:bg-card/50 sm:p-4";

/**
 * A standalone/last section, or a row inside a `divide-y` list: card on >= sm,
 * borderless on mobile, no divider of its own (the list container draws those).
 * Add `py-*` for mobile breathing room when used as a list row.
 */
export const sectionCardPlain =
  "sm:rounded-lg sm:border sm:border-border sm:bg-card/50 sm:p-4";
