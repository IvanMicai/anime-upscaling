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
