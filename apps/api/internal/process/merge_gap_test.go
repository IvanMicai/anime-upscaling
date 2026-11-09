package process

import (
	"strings"
	"testing"

	"anime-upscaling/internal/dualaudio"
)

func TestUnmatchedAudioMessageIdentifiesFillAndBounds(t *testing.T) {
	if got := unmatchedAudioMessage(&dualaudio.Result{}); got != "" {
		t.Fatalf("unexpected message: %s", got)
	}
	r := &dualaudio.Result{GapFill: dualaudio.GapFillBase, Alignment: &dualaudio.Alignment{Gaps: []dualaudio.Gap{{Start: 4, End: 6}, {Start: 9.5, End: 10}}}}
	got := unmatchedAudioMessage(r)
	for _, want := range []string{"2 trecho(s)", "áudio do vídeo base", "4.00–6.00", "9.50–10.00"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q: %s", want, got)
		}
	}
	r.GapFill = dualaudio.GapFillSilence
	if got = unmatchedAudioMessage(r); !strings.Contains(got, "silêncio") {
		t.Fatal(got)
	}
	r.Alignment.Gaps = make([]dualaudio.Gap, 10)
	if got = unmatchedAudioMessage(r); !strings.Contains(got, "e mais 2") {
		t.Fatal(got)
	}
}
