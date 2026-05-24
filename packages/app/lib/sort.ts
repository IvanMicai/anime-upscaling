// Natural-order string comparison: "Episode 2" < "Episode 10".
// `sensitivity: "base"` ignores case and accents, matching the backend's
// ASCII-fold implementation in internal/files/sort.go (NaturalLess).
// Passing `undefined` as the locale keeps the result consistent across
// browsers regardless of the user's system locale.
export const compareNatural = (a: string, b: string): number =>
  a.localeCompare(b, undefined, { numeric: true, sensitivity: "base" });
