package files

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSafeVideoFilename(t *testing.T) {
	exts := []string{".mkv", ".mp4", ".avi"}
	cases := []struct {
		name string
		want bool
	}{
		{"episode 01.mkv", true},
		{"movie.MP4", true},
		{"../movie.mkv", false},
		{"subdir/movie.mkv", false},
		{"movie.txt", false},
		{"movie..mkv", false},
		{"", false},
	}

	for _, c := range cases {
		if got := SafeVideoFilename(c.name, exts); got != c.want {
			t.Errorf("SafeVideoFilename(%q) = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestSafeRelDir(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"", true},
		{"season1", true},
		{"season1/specials", true},
		{"a/b/c", true},
		{"/abs", false},
		{"..", false},
		{"a/..", false},
		{"a/../b", false},
		{"a//b", false},
		{"a\\b", false},
		{".", false},
		{"./a", false},
		{"a/./b", false},
		{"a/", false},
	}

	for _, c := range cases {
		if got := SafeRelDir(c.path); got != c.want {
			t.Errorf("SafeRelDir(%q) = %v, want %v", c.path, got, c.want)
		}
	}
}

func TestSafeVideoRelPath(t *testing.T) {
	exts := []string{".mkv", ".mp4"}
	cases := []struct {
		path string
		want bool
	}{
		{"episode01.mkv", true},
		{"season1/ep01.mkv", true},
		{"season1/specials/ep01.MP4", true},
		{"", false},
		{"../movie.mkv", false},
		{"/abs/movie.mkv", false},
		{"a//b.mkv", false},
		{"season1/../movie.mkv", false},
		{"season1/movie.txt", false},
		{"season1/", false},
		{"season1\\ep01.mkv", false},
	}

	for _, c := range cases {
		if got := SafeVideoRelPath(c.path, exts); got != c.want {
			t.Errorf("SafeVideoRelPath(%q) = %v, want %v", c.path, got, c.want)
		}
	}
}

func TestListAllWithStatus(t *testing.T) {
	root := t.TempDir()
	bases := map[string]string{
		"input":        filepath.Join(root, "input"),
		"output":       filepath.Join(root, "output"),
		"optimized":    filepath.Join(root, "optimized"),
		"interpolated": filepath.Join(root, "interpolated"),
	}
	subPath := "season1"
	for _, b := range bases {
		if err := os.MkdirAll(filepath.Join(b, subPath), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	// only-input.mkv: lives in input only
	mustWrite(t, filepath.Join(bases["input"], subPath, "only-input.mkv"), 100)
	// only-output.mkv: lives only in output (Upscaling) - the bug case
	mustWrite(t, filepath.Join(bases["output"], subPath, "only-output.mkv"), 200)
	// only-optimized.mkv: lives only in optimized
	mustWrite(t, filepath.Join(bases["optimized"], subPath, "only-optimized.mkv"), 300)
	// only-interpolated.mkv: lives only in interpolated
	mustWrite(t, filepath.Join(bases["interpolated"], subPath, "only-interpolated.mkv"), 400)
	// shared.mkv: lives in input + output with different sizes
	mustWrite(t, filepath.Join(bases["input"], subPath, "shared.mkv"), 10)
	mustWrite(t, filepath.Join(bases["output"], subPath, "shared.mkv"), 20)
	// non-video.txt: must be filtered out
	mustWrite(t, filepath.Join(bases["input"], subPath, "ignore.txt"), 1)
	// subdirs that exist in different bases
	if err := os.MkdirAll(filepath.Join(bases["output"], subPath, "specials"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(bases["optimized"], subPath, "extras"), 0o755); err != nil {
		t.Fatal(err)
	}

	exts := []string{".mkv", ".mp4"}

	vfiles, dirs, err := ListAllWithStatus("input", bases, subPath, exts)
	if err != nil {
		t.Fatalf("ListAllWithStatus error: %v", err)
	}

	wantNames := []string{"only-input.mkv", "only-interpolated.mkv", "only-optimized.mkv", "only-output.mkv", "shared.mkv"}
	if len(vfiles) != len(wantNames) {
		t.Fatalf("got %d files, want %d (%v)", len(vfiles), len(wantNames), vfiles)
	}
	for i, n := range wantNames {
		if vfiles[i].Name != n {
			t.Errorf("vfiles[%d].Name = %q, want %q", i, vfiles[i].Name, n)
		}
	}

	by := map[string]VideoFile{}
	for _, v := range vfiles {
		by[v.Name] = v
	}

	// only-input on primary=input → Size mirrors input
	oi := by["only-input.mkv"]
	if !oi.HasInput || oi.InputSize != 100 || oi.Size != 100 {
		t.Errorf("only-input flags wrong: %+v", oi)
	}
	if oi.HasUpscaled || oi.HasOptimized || oi.HasInterpolated {
		t.Errorf("only-input should not have other flags: %+v", oi)
	}

	// only-output on primary=input → Size 0 (missing from primary), HasUpscaled true
	oo := by["only-output.mkv"]
	if oo.Size != 0 {
		t.Errorf("only-output Size should be 0 when primary=input, got %d", oo.Size)
	}
	if oo.HasInput || !oo.HasUpscaled || oo.UpscaledSize != 200 {
		t.Errorf("only-output flags wrong: %+v", oo)
	}

	// only-optimized
	oop := by["only-optimized.mkv"]
	if oop.Size != 0 || !oop.HasOptimized || oop.OptimizedSize != 300 {
		t.Errorf("only-optimized flags wrong: %+v", oop)
	}

	// only-interpolated
	ointp := by["only-interpolated.mkv"]
	if ointp.Size != 0 || !ointp.HasInterpolated || ointp.InterpolatedSize != 400 {
		t.Errorf("only-interpolated flags wrong: %+v", ointp)
	}

	// shared on primary=input → Size = 10 (input), UpscaledSize = 20
	sh := by["shared.mkv"]
	if sh.Size != 10 || !sh.HasInput || sh.InputSize != 10 {
		t.Errorf("shared input fields wrong: %+v", sh)
	}
	if !sh.HasUpscaled || sh.UpscaledSize != 20 {
		t.Errorf("shared upscaled fields wrong: %+v", sh)
	}

	// txt is excluded
	if _, ok := by["ignore.txt"]; ok {
		t.Errorf("non-video file should not appear: %+v", by)
	}

	// dirs union, sorted
	wantDirs := []string{"extras", "specials"}
	if len(dirs) != len(wantDirs) {
		t.Fatalf("dirs = %v, want %v", dirs, wantDirs)
	}
	for i, d := range wantDirs {
		if dirs[i] != d {
			t.Errorf("dirs[%d] = %q, want %q", i, dirs[i], d)
		}
	}

	// Now switch primary to output → Size for shared should be 20
	vfilesOut, _, err := ListAllWithStatus("output", bases, subPath, exts)
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range vfilesOut {
		if v.Name == "shared.mkv" && v.Size != 20 {
			t.Errorf("primary=output shared.mkv Size = %d, want 20", v.Size)
		}
		if v.Name == "only-input.mkv" && v.Size != 0 {
			t.Errorf("primary=output only-input.mkv Size = %d, want 0", v.Size)
		}
	}

	// Missing subPath (no folder in any base) → empty result, no error
	vfilesMissing, dirsMissing, err := ListAllWithStatus("input", bases, "nonexistent", exts)
	if err != nil {
		t.Fatalf("missing subPath should not error: %v", err)
	}
	if len(vfilesMissing) != 0 || len(dirsMissing) != 0 {
		t.Errorf("missing subPath should be empty, got files=%v dirs=%v", vfilesMissing, dirsMissing)
	}
}

func mustWrite(t *testing.T, path string, size int) {
	t.Helper()
	if err := os.WriteFile(path, make([]byte, size), 0o644); err != nil {
		t.Fatal(err)
	}
}
