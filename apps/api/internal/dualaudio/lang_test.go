package dualaudio

import "testing"

func TestSplitLanguage(t *testing.T) {
	cases := []struct {
		in, stem, iso3 string
		ok             bool
	}{
		{"name.pt-br.mp4", "name", "por", true},
		{"name.en.mp4", "name", "eng", true},
		{"Show/S01/Ep 01 - Title.PT-BR.mkv", "Show/S01/Ep 01 - Title", "por", true},
		{"ep01_ja.mkv", "ep01", "jpn", true},
		{"ep01 en.mkv", "ep01", "eng", true},
		// Short segments that are NOT languages must not be read as one.
		{"show.web.mkv", "show.web", "", false},
		{"show.x264.mkv", "show.x264", "", false},
		{"show.v2.mkv", "show.v2", "", false},
		{"plain.mkv", "plain", "", false},
		{"dir.en/plain.mkv", "dir.en/plain", "", false},
		{".en.mkv", ".en", "", false},
	}
	for _, c := range cases {
		stem, lang, ok := SplitLanguage(c.in)
		if ok != c.ok || (ok && (stem != c.stem || lang.ISO3 != c.iso3)) {
			t.Errorf("SplitLanguage(%q) = %q, %q, %v; want %q, %q, %v", c.in, stem, lang.ISO3, ok, c.stem, c.iso3, c.ok)
		}
	}
}

func TestPairByName(t *testing.T) {
	pairs, unpaired := PairByName([]string{
		"Show/EN/ep01.en.mkv", "Show/PT/ep01.pt-br.mp4", // two folders
		"Show/ep02.en.mkv", "Show/ep02.pt-br.mkv", // same folder
		"Show/EN/ep03.en.mkv",                                      // orphan
		"Show/ep04.mkv",                                            // no tag
		"Show/ep05.en.mkv", "Show/ep05.pt.mkv", "Show/ep05.ja.mkv", // three-way
		"Show/ep06.en.mkv", "Other/ep06.en.mkv", // same language twice
	})
	if len(pairs) != 2 {
		t.Fatalf("pairs = %+v, want 2", pairs)
	}
	if pairs[0].Output != "Show/ep01.mkv" || pairs[1].Output != "Show/ep02.mkv" {
		t.Errorf("outputs = %q, %q; want the common ancestor + stem", pairs[0].Output, pairs[1].Output)
	}
	if pairs[0].LangA.ISO3 != "eng" || pairs[0].LangB.ISO3 != "por" {
		t.Errorf("languages = %+v / %+v", pairs[0].LangA, pairs[0].LangB)
	}
	if len(unpaired) != 7 {
		t.Errorf("unpaired = %d (%+v), want 7", len(unpaired), unpaired)
	}
}

func TestCommonDir(t *testing.T) {
	for _, c := range [][3]string{
		{"a/b/x.mkv", "a/c/y.mkv", "a"},
		{"a/b/x.mkv", "a/b/y.mkv", "a/b"},
		{"x.mkv", "y.mkv", ""},
		{"a/x.mkv", "b/y.mkv", ""},
	} {
		if got := commonDir(c[0], c[1]); got != c[2] {
			t.Errorf("commonDir(%q, %q) = %q, want %q", c[0], c[1], got, c[2])
		}
	}
}

func TestBetterVideo(t *testing.T) {
	hd := VideoStats{Width: 1280, Height: 960, Bitrate: 1_900_000}
	sd := VideoStats{Width: 640, Height: 480, Bitrate: 2_500_000}
	if !BetterVideo(hd, sd) || BetterVideo(sd, hd) {
		t.Error("more pixels must win over more bitrate")
	}
	lean := VideoStats{Width: 640, Height: 480, Bitrate: 500_000}
	if !BetterVideo(sd, lean) || BetterVideo(lean, sd) {
		t.Error("at equal resolution the higher bitrate must win")
	}
}

func TestSyncInfoRoundTrip(t *testing.T) {
	r := &Result{Status: StatusOK, ResidualMedian: 0.0012, ResidualP95: 0.0104, Validated: 238,
		ValidatedOf: 245, TickSec: 5, VideoFrom: "en",
		Alignment: &Alignment{Coverage: 0.9541, Segments: make([]Segment, 9), Gaps: make([]Gap, 4)}}
	info := r.SyncInfo()
	// Matroska upper-cases tag names; the reader must not care.
	got, ok := ParseSyncInfo(map[string]string{"dualaudio_sync": info.Encode(), "ENCODER": "x"})
	if !ok {
		t.Fatal("tag did not parse back")
	}
	if got.Status != StatusOK || got.InPlace != 238 || got.Checked != 245 || got.Segments != 9 ||
		got.ResidualMs != 1.2 || got.VideoFrom != "en" || got.Coverage != 0.954 {
		t.Errorf("round trip lost data: %+v", got)
	}
	if _, ok := ParseSyncInfo(map[string]string{"title": "x"}); ok {
		t.Error("a file without the tag must not report a verdict")
	}
}

func TestClampTickAndMinRun(t *testing.T) {
	for in, want := range map[float64]float64{0: 5, -3: 5, 0.2: 1, 2: 2, 10: 10, 120: 30} {
		if got := ClampTick(in); got != want {
			t.Errorf("ClampTick(%v) = %v, want %v", in, got, want)
		}
	}
}
