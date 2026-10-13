package dualaudio

import (
	"context"
	"fmt"
	"math"
	"strconv"
)

// Comparing the picture of two releases means looking at the SAME FRAME in
// both, and the same frame is not at the same timestamp: releases differ in
// where the opening and the cuts sit, and the offset changes inside an episode
// (seven times in one validation episode, with a 12 s cut). Showing t=600 s from
// both compares two different scenes.
//
// A full alignment answers this but costs ~40 s. For one point it is enough to
// take the audio around it in one file and find it in a stretch of the other —
// a few minutes of audio, about a second of work.

const (
	locateWindowSec = 30.0
	// locateSearchSec is how far apart the two files may be at that point. The
	// largest difference seen between two releases of one episode was 209 s.
	locateSearchSec = 240.0
	// locateMinConfidence is the same floor the aligner uses for a window.
	locateMinConfidence = 6.0
)

// Location is where a point of file A falls in file B.
type Location struct {
	TimeA      float64 `json:"t_a"`
	TimeB      float64 `json:"t_b"`
	Lag        float64 `json:"lag"` // t_b - t_a
	Confidence float64 `json:"confidence"`
	// Matched is false when the audio around the point could not be found in B
	// (dialogue only, or content B does not have). TimeB then falls back to
	// TimeA, and the two frames may not be the same scene.
	Matched bool `json:"matched"`
}

// decodeSpan decodes [start, start+dur) of an audio stream as mono PCM.
func (t Tools) decodeSpan(ctx context.Context, path string, start, dur float64) ([]int16, error) {
	raw, err := t.run(ctx, t.FFmpeg, nil, "-v", "error", "-nostdin",
		"-ss", strconv.FormatFloat(start, 'f', 3, 64), "-t", strconv.FormatFloat(dur, 'f', 3, 64),
		"-i", path, "-map", "0:a:0", "-vn", "-ac", "1", "-ar", strconv.Itoa(featRate), "-f", "s16le", "-")
	if err != nil {
		return nil, err
	}
	pcm := make([]int16, len(raw)/2)
	for i := range pcm {
		pcm[i] = int16(uint16(raw[2*i]) | uint16(raw[2*i+1])<<8)
	}
	return pcm, nil
}

// Locate finds where time tA of file A falls in file B, by audio.
func Locate(ctx context.Context, t Tools, pathA, pathB string, tA float64) (Location, error) {
	loc := Location{TimeA: tA, TimeB: tA}
	durA, err := t.Duration(ctx, pathA)
	if err != nil {
		return loc, err
	}
	durB, err := t.Duration(ctx, pathB)
	if err != nil {
		return loc, err
	}
	if tA < 0 || tA > durA {
		return loc, fmt.Errorf("t=%.1f is outside the file (%.1f s)", tA, durA)
	}
	loc.TimeB = math.Min(tA, durB)

	// Three placements of the window around tA — centred, forward-only and
	// backward-only — and the most confident wins. Near a cut, or near the start
	// of a file that lacks the other's opening, part of a centred window has no
	// counterpart at all and cannot match; a window on one side of tA can.
	for _, offset := range []float64{-locateWindowSec / 2, 0, -locateWindowSec} {
		cand, err := locateWith(ctx, t, pathA, pathB, tA, tA+offset, durA, durB)
		if err != nil {
			return loc, err
		}
		if cand.Confidence > loc.Confidence {
			cand.TimeA = tA
			loc = cand
		}
		if loc.Matched && loc.Confidence >= 3*locateMinConfidence {
			break // clear enough; the other placements cost two more decodes
		}
	}
	if !loc.Matched {
		loc.TimeB, loc.Lag = math.Min(tA, durB), 0
	}
	return loc, nil
}

func locateWith(ctx context.Context, t Tools, pathA, pathB string, tA, winStart, durA, durB float64) (Location, error) {
	loc := Location{TimeA: tA, TimeB: math.Min(tA, durB)}
	winStart = math.Max(0, math.Min(winStart, durA-locateWindowSec))
	winDur := math.Min(locateWindowSec, durA-winStart)
	regStart := math.Max(0, winStart-locateSearchSec)
	regEnd := math.Min(durB, winStart+winDur+locateSearchSec)
	if winDur < 5 || regEnd-regStart < winDur {
		return loc, nil
	}

	winPCM, err := t.decodeSpan(ctx, pathA, winStart, winDur)
	if err != nil {
		return loc, err
	}
	regPCM, err := t.decodeSpan(ctx, pathB, regStart, regEnd-regStart)
	if err != nil {
		return loc, err
	}
	win, reg := ExtractFeatures(winPCM), ExtractFeatures(regPCM)
	if win.Frames < 100 || reg.Frames <= win.Frames {
		return loc, nil
	}
	// p0=0 and an unbounded search: the lag that comes back is the window's
	// start inside the region, in frames.
	lag, conf, ok := matchWindow(reg, win, 0, 0, -1)
	if !ok {
		return loc, nil
	}
	loc.Confidence = conf
	if conf < locateMinConfidence {
		return loc, nil
	}
	loc.Matched = true
	loc.TimeB = math.Max(0, math.Min(durB, regStart+lag/fps+(tA-winStart)))
	loc.Lag = loc.TimeB - tA
	return loc, nil
}

// Frame extracts the picture at time sec as PNG. PNG on purpose: the point of
// the frame is to judge picture quality, and JPEG would add artefacts of its own
// to exactly what is being judged.
func (t Tools) Frame(ctx context.Context, path string, sec float64) ([]byte, error) {
	if sec < 0 {
		sec = 0
	}
	// -ss before -i seeks by keyframe and then decodes up to the exact time.
	return t.run(ctx, t.FFmpeg, nil, "-v", "error", "-nostdin",
		"-ss", strconv.FormatFloat(sec, 'f', 3, 64), "-i", path,
		"-map", "0:v:0", "-frames:v", "1", "-c:v", "png", "-f", "image2pipe", "-")
}
