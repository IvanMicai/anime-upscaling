package files

import (
	"reflect"
	"sort"
	"testing"
)

func TestNaturalLess_BasicPairs(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"Episode 2", "Episode 10", true},
		{"Episode 10", "Episode 2", false},
		{"Episode 2.mkv", "Episode 10.mkv", true},
		{"S01E01", "S01E02", true},
		{"S01E10", "S02E01", true},
		{"S2E1", "S10E1", true},
		{"a", "B", true},
		{"B", "a", false},
		{"abc", "abd", true},
		{"abc", "abc", false},
		{"ep1", "ep1a", true},
		{"ep1a", "ep1", false},
		{"ep01", "ep1", true},
		{"ep1", "ep01", false},
		{"season1/ep2.mkv", "season1/ep10.mkv", true},
		{"season1/ep10.mkv", "season1/ep2.mkv", false},
		{"season10/ep1.mkv", "season2/ep1.mkv", false},
		{"season2/ep1.mkv", "season10/ep1.mkv", true},
	}
	for _, c := range cases {
		got := NaturalLess(c.a, c.b)
		if got != c.want {
			t.Errorf("NaturalLess(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

func TestSortNatural_FullList(t *testing.T) {
	in := []string{
		"Episode 10.mkv",
		"Episode 2.mkv",
		"Episode 1.mkv",
		"Episode 11.mkv",
		"Episode 9.mkv",
	}
	want := []string{
		"Episode 1.mkv",
		"Episode 2.mkv",
		"Episode 9.mkv",
		"Episode 10.mkv",
		"Episode 11.mkv",
	}
	SortNatural(in)
	if !reflect.DeepEqual(in, want) {
		t.Errorf("SortNatural mismatch\n got: %v\nwant: %v", in, want)
	}
}

func TestSortNatural_Idempotent(t *testing.T) {
	in := []string{"Episode 1.mkv", "Episode 2.mkv", "Episode 10.mkv"}
	SortNatural(in)
	first := append([]string(nil), in...)
	SortNatural(in)
	if !reflect.DeepEqual(in, first) {
		t.Errorf("SortNatural not idempotent: %v vs %v", first, in)
	}
}

func TestNaturalLess_Total(t *testing.T) {
	// Property: trichotomy for a tiny corpus.
	corpus := []string{"a", "A", "ep1", "EP01", "ep10", "ep2", "ep2a", "season1/ep1.mkv", "season1/ep2.mkv"}
	for _, x := range corpus {
		for _, y := range corpus {
			lxy := NaturalLess(x, y)
			lyx := NaturalLess(y, x)
			if x == y {
				if lxy || lyx {
					t.Errorf("equal pair (%q) should be neither less", x)
				}
				continue
			}
			if lxy == lyx {
				t.Errorf("trichotomy broken on (%q, %q): both directions = %v", x, y, lxy)
			}
		}
	}
}

func TestSortNatural_AgreesWithSortPackage(t *testing.T) {
	// Sanity: applying NaturalLess via sort.Slice gives the same result as
	// applying SortNatural directly.
	in := []string{"b10", "b2", "a", "B", "b1"}
	manual := append([]string(nil), in...)
	sort.Slice(manual, func(i, j int) bool { return NaturalLess(manual[i], manual[j]) })
	via := append([]string(nil), in...)
	SortNatural(via)
	if !reflect.DeepEqual(manual, via) {
		t.Errorf("SortNatural diverges from sort.Slice(NaturalLess): %v vs %v", via, manual)
	}
}
