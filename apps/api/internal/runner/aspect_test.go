package runner

import "testing"

func TestParseAspectRatio(t *testing.T) {
	cases := []struct {
		in       string
		num, den int
	}{
		{"32:27", 32, 27},
		{"8:9", 8, 9},
		{"1:1", 1, 1},
		{"0:1", 0, 0},     // undefined SAR
		{"N/A", 0, 0},     // ffprobe unknown
		{"", 0, 0},        // missing field
		{"16", 0, 0},      // malformed (no colon)
		{"a:b", 0, 0},     // non-numeric
		{"-1:2", 0, 0},    // negative
		{" 4 : 3 ", 4, 3}, // whitespace tolerant
	}
	for _, c := range cases {
		n, d := parseAspectRatio(c.in)
		if n != c.num || d != c.den {
			t.Errorf("parseAspectRatio(%q) = %d:%d, want %d:%d", c.in, n, d, c.num, c.den)
		}
	}
}

func TestAspectInfoAnamorphic(t *testing.T) {
	cases := []struct {
		name string
		info AspectInfo
		want bool
	}{
		{"anamorphic 16:9 DVD", AspectInfo{Width: 720, Height: 480, SarNum: 32, SarDen: 27}, true},
		{"anamorphic 4:3 DVD", AspectInfo{Width: 720, Height: 480, SarNum: 8, SarDen: 9}, true},
		{"square pixels", AspectInfo{Width: 640, Height: 480, SarNum: 1, SarDen: 1}, false},
		{"unknown SAR", AspectInfo{Width: 1920, Height: 1080, SarNum: 0, SarDen: 0}, false},
	}
	for _, c := range cases {
		if got := c.info.Anamorphic(); got != c.want {
			t.Errorf("%s: Anamorphic() = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestBuildNormalizeFilter(t *testing.T) {
	cases := []struct {
		name        string
		deinterlace bool
		want        string
	}{
		{"square only", false, "scale='round(iw*sar/2)*2':ih:flags=lanczos,setsar=1"},
		{"with deinterlace", true, "bwdif,scale='round(iw*sar/2)*2':ih:flags=lanczos,setsar=1"},
	}
	for _, c := range cases {
		if got := buildNormalizeFilter(c.deinterlace); got != c.want {
			t.Errorf("%s: buildNormalizeFilter(%v) = %q, want %q", c.name, c.deinterlace, got, c.want)
		}
	}
}
