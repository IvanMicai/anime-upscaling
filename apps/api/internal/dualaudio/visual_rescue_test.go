package dualaudio

import "testing"

func TestVisualCandidatesCanSeedAnEmptyAudioAlignment(t *testing.T) {
	pcm := toPCM(music(90, 420))
	f := ExtractFeatures(pcm)
	a := &Alignment{BaseDuration: 90, DubDuration: 90}
	seedVisualCandidates(a, []estimate{{frame: int(20 * fps), lag: 0, conf: 10}, {frame: int(60 * fps), lag: 0, conf: 10}}, 30)
	if a.wm != int(30*fps) || len(a.cand) != 1 {
		t.Fatalf("candidate state not initialized: wm=%d candidates=%d", a.wm, len(a.cand))
	}
	a.Solve(f, f)
	if len(a.Segments) == 0 {
		t.Fatal("visual proposals should let the solver build a candidate")
	}
	status, _ := grade(a, Validation{}, DefaultGate())
	if status == StatusOK {
		t.Fatal("a visual proposal must not bypass audio validation")
	}
}
