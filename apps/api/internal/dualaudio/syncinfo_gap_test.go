package dualaudio

import (
	"strings"
	"testing"
)

func TestSyncInfoDescribesUnmatchedAudioWithoutChangingVerdict(t *testing.T) {
	r := &Result{Status: StatusOK, GapFill: GapFillBase, Alignment: &Alignment{Coverage: .98, Gaps: []Gap{{Start: 12, End: 14}}}}
	s := r.SyncInfo()
	if s.Status != StatusOK || s.GapFill != GapFillBase || !strings.Contains(s.Summary(), "1 unmatched intervals (fill: base)") {
		t.Fatalf("incorrect summary: %+v %s", s, s.Summary())
	}
	parsed, ok := ParseSyncInfo(map[string]string{SyncTag: s.Encode()})
	if !ok || parsed.GapFill != GapFillBase {
		t.Fatalf("gap fill did not round trip: %+v", parsed)
	}
	old, ok := ParseSyncInfo(map[string]string{SyncTag: `{"v":1,"status":"ok","gaps":1}`})
	if !ok || old.GapFill != "" || !strings.Contains(old.Summary(), "1 unmatched intervals") {
		t.Fatalf("old metadata incompatible: %+v", old)
	}
}
