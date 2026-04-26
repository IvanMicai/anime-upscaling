package files

import "testing"

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
