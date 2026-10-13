package dualaudio

import (
	"encoding/json"
	"fmt"
)

// Container tags the merged file carries. Matroska stores them upper-cased.
const (
	SyncTag        = "DUALAUDIO_SYNC"
	SyncSummaryTag = "DUALAUDIO_SYNC_SUMMARY"
)

// SyncInfo is the sync verdict written into the merged file. It is what the
// file explorer shows, so it holds what a person needs to decide whether to
// trust the file — not the whole alignment.
type SyncInfo struct {
	Version    int      `json:"v"`
	Status     Status   `json:"status"`
	ResidualMs float64  `json:"residual_ms"` // median error of the confident checks
	P95Ms      float64  `json:"p95_ms"`
	InPlace    int      `json:"in_place"` // checks whose peak landed on zero
	Checked    int      `json:"checked"`  // ... out of this many
	OffSec     float64  `json:"off_s"`    // seconds at a consistently wrong offset
	Coverage   float64  `json:"coverage"` // share of the video that has dub audio
	Segments   int      `json:"segments"`
	Gaps       int      `json:"gaps"`
	TickSec    float64  `json:"tick_s"`
	VideoFrom  string   `json:"video,omitempty"` // language tag of the file the video came from
	Notes      []string `json:"notes,omitempty"`
}

// SyncInfo summarises a result.
func (r *Result) SyncInfo() SyncInfo {
	s := SyncInfo{Version: 1, Status: r.Status, ResidualMs: round1(r.ResidualMedian * 1000),
		P95Ms: round1(r.ResidualP95 * 1000), InPlace: r.Validated, Checked: r.ValidatedOf,
		OffSec: round1(r.OffSec), TickSec: r.TickSec, VideoFrom: r.VideoFrom, Notes: r.Notes}
	if r.Alignment != nil {
		s.Coverage = round3(r.Alignment.Coverage)
		s.Segments, s.Gaps = len(r.Alignment.Segments), len(r.Alignment.Gaps)
	}
	return s
}

// Encode renders the verdict as the tag value.
func (s SyncInfo) Encode() string {
	b, err := json.Marshal(s)
	if err != nil {
		return "{}"
	}
	return string(b)
}

// Summary is the one-line, human-readable form, for tools that only show tags.
func (s SyncInfo) Summary() string {
	share := 0.0
	if s.Checked > 0 {
		share = float64(s.InPlace) / float64(s.Checked) * 100
	}
	return fmt.Sprintf("%s — residual %.0f ms, %d/%d checks in place (%.0f%%), %.0f s off, every %.0f s",
		s.Status, s.ResidualMs, s.InPlace, s.Checked, share, s.OffSec, s.TickSec)
}

// ParseSyncInfo reads the verdict back from container tags; keys are matched
// case-insensitively because muxers normalise them.
func ParseSyncInfo(tags map[string]string) (*SyncInfo, bool) {
	for k, v := range tags {
		if !equalFold(k, SyncTag) {
			continue
		}
		var s SyncInfo
		if json.Unmarshal([]byte(v), &s) != nil || s.Version == 0 {
			return nil, false
		}
		return &s, true
	}
	return nil, false
}

func equalFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		x, y := a[i], b[i]
		if x >= 'a' && x <= 'z' {
			x -= 32
		}
		if y >= 'a' && y <= 'z' {
			y -= 32
		}
		if x != y {
			return false
		}
	}
	return true
}

func round1(x float64) float64 { return float64(int(x*10+0.5)) / 10 }
func round3(x float64) float64 { return float64(int(x*1000+0.5)) / 1000 }
