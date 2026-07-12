package files

import "sort"

// NaturalLess reports whether a should sort before b in natural order:
// runs of ASCII digits are compared as numbers (so "Episode 2" < "Episode 10"),
// and other bytes are compared case-insensitively for ASCII letters.
//
// Total ties on the natural comparison fall back to raw byte order, so the
// result is total and stable (e.g. "ep01" sorts before "ep1").
//
// Trade-off: only ASCII letters are case-folded. Filenames with accented
// characters fall back to byte-wise comparison, which matches the frontend's
// `localeCompare(b, undefined, { sensitivity: "base" })` for ASCII and is
// close enough for the anime-naming conventions we see in practice. If that
// causes a regression, swap the byte loop for a rune loop using unicode.ToLower.
func NaturalLess(a, b string) bool {
	return naturalCompare(a, b) < 0
}

// SortNatural sorts s in-place using NaturalLess.
func SortNatural(s []string) {
	sort.Slice(s, func(i, j int) bool { return NaturalLess(s[i], s[j]) })
}

func naturalCompare(a, b string) int {
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		ca, cb := a[i], b[j]
		if isDigit(ca) && isDigit(cb) {
			ai := skipDigits(a, i)
			bj := skipDigits(b, j)
			numA := stripLeadingZeros(a[i:ai])
			numB := stripLeadingZeros(b[j:bj])
			// Compare as integers via length, then lex — avoids strconv
			// overflow on absurdly long digit runs.
			if len(numA) != len(numB) {
				if len(numA) < len(numB) {
					return -1
				}
				return 1
			}
			if numA != numB {
				if numA < numB {
					return -1
				}
				return 1
			}
			i, j = ai, bj
			continue
		}
		la, lb := lowerASCII(ca), lowerASCII(cb)
		if la != lb {
			if la < lb {
				return -1
			}
			return 1
		}
		i++
		j++
	}
	if remA, remB := len(a)-i, len(b)-j; remA != remB {
		if remA < remB {
			return -1
		}
		return 1
	}
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

func lowerASCII(c byte) byte {
	if c >= 'A' && c <= 'Z' {
		return c + ('a' - 'A')
	}
	return c
}

func skipDigits(s string, i int) int {
	for i < len(s) && isDigit(s[i]) {
		i++
	}
	return i
}

func stripLeadingZeros(s string) string {
	i := 0
	for i < len(s)-1 && s[i] == '0' {
		i++
	}
	return s[i:]
}
