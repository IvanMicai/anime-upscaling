package dualaudio

import (
	"context"
	"fmt"
	"strings"
)

// VideoChoice says which of the two files supplies the picture.
const VideoAuto = "auto"

// MergeRequest is one name pair to merge.
type MergeRequest struct {
	PathA, PathB string // absolute
	LangA, LangB Language
	OutPath      string
	// Video is VideoAuto, or the language tag of the file whose picture to keep.
	Video   string
	GapFill GapFill
	TickSec float64
	Force   bool
	// DefaultAudio is the language tag of the default track; empty means the
	// language that did NOT supply the video — the one the merge was done for.
	DefaultAudio string
	OnPhase      func(phase string)
}

// VideoStats is what picking the better picture needs.
type VideoStats struct {
	Width, Height int
	Bitrate       int64 // bits per second, container level
}

func (v VideoStats) pixels() int { return v.Width * v.Height }

// BetterVideo reports whether a is the better picture than b: more pixels
// first, then more bits per second. Bitrate only breaks ties between equal
// resolutions — across resolutions or codecs it says little about quality.
//
// It is a measure, not a judgement. An AI upscale has four times the pixels of
// the clean source it came from and may still look worse, which is why the
// choice can be made by hand.
func BetterVideo(a, b VideoStats) bool {
	if a.pixels() != b.pixels() {
		return a.pixels() > b.pixels()
	}
	return a.Bitrate >= b.Bitrate
}

// Merge builds one dual-audio file from a name pair: it decides which file is
// the base (video), aligns the other's audio to it and writes the result with
// the sync verdict in its tags.
func Merge(ctx context.Context, t Tools, m MergeRequest) (*Result, error) {
	aIsBase := true
	switch want := strings.ToLower(m.Video); want {
	case "", VideoAuto:
		sa, err := t.VideoStats(ctx, m.PathA)
		if err != nil {
			return nil, err
		}
		sb, err := t.VideoStats(ctx, m.PathB)
		if err != nil {
			return nil, err
		}
		aIsBase = BetterVideo(sa, sb)
	case m.LangA.Tag:
	case m.LangB.Tag:
		aIsBase = false
	default:
		return nil, fmt.Errorf("video=%q matches neither %q nor %q", m.Video, m.LangA.Tag, m.LangB.Tag)
	}

	base, dub, baseLang, dubLang := m.PathA, m.PathB, m.LangA, m.LangB
	if !aIsBase {
		base, dub, baseLang, dubLang = dub, base, dubLang, baseLang
	}
	dubDefault := true
	if m.DefaultAudio != "" {
		dubDefault = strings.EqualFold(m.DefaultAudio, dubLang.Tag)
	}
	fill := m.GapFill
	if fill == "" {
		fill = GapFillBase
	}
	res, err := Process(ctx, t, Job{
		BasePath: base, DubPath: dub, OutPath: m.OutPath,
		Options: DefaultOptions(), Gate: DefaultGate(), GapFill: fill,
		Force: m.Force, TickSec: m.TickSec, VideoFrom: baseLang.Tag, OnPhase: m.OnPhase,
		Mux: MuxOptions{
			BaseLang: baseLang.ISO3, BaseTitle: baseLang.Title,
			DubLang: dubLang.ISO3, DubTitle: dubLang.Title,
			Bitrate: "192k", DubIsDefault: dubDefault,
		},
	})
	return res, err
}
