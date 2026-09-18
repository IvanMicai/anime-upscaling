// Package dualaudio joins the video of one release with the dubbed audio of
// another, keeping the dub in sync segment by segment.
//
// It works across languages because a dub is normally made over the same
// music-and-effects master: only the voices differ, and the score is what the
// two tracks are matched on. When that does not hold (a dub made over a
// different master) confidence collapses and the episode is graded "review"
// instead of being written out of sync.
package dualaudio

import (
	"context"
	"fmt"
)

// Status is the verdict of the quality gate.
type Status string

const (
	StatusOK     Status = "ok"
	StatusReview Status = "review"
	StatusFail   Status = "fail"
)

// Gate holds the thresholds an episode must meet to be written.
type Gate struct {
	MaxResidualSec float64 // median residual; p95 may be 3x this
	MinCoverage    float64
}

// DefaultGate matches the validated thresholds: 50 ms is below the point where
// dialogue reads as out of sync, and 90% coverage allows for credits and
// previews that exist in only one release.
func DefaultGate() Gate { return Gate{MaxResidualSec: 0.05, MinCoverage: 0.90} }

// Result is what one episode produced.
type Result struct {
	Base           string     `json:"base"`
	Dub            string     `json:"dub"`
	Status         Status     `json:"status"`
	Notes          []string   `json:"notes,omitempty"`
	Alignment      *Alignment `json:"alignment,omitempty"`
	ResidualMedian float64    `json:"residual_median"`
	ResidualP95    float64    `json:"residual_p95"`
	Written        bool       `json:"written"`
}

// Job describes one episode to build.
type Job struct {
	BasePath, DubPath, OutPath string
	BaseStream, DubStream      int
	Options                    Options
	Gate                       Gate
	GapFill                    GapFill
	Mux                        MuxOptions
	// Force writes the file even when the gate says "review".
	Force bool
}

// Process analyses, rebuilds, validates and — only if the gate passes — writes
// one episode. The point of the gate is that an episode which did not line up
// is reported with its reason rather than shipped quietly out of sync.
func Process(ctx context.Context, t Tools, j Job) (*Result, error) {
	res := &Result{Base: j.BasePath, Dub: j.DubPath, Status: StatusFail}

	baseDur, err := t.Duration(ctx, j.BasePath)
	if err != nil {
		return res, err
	}
	dubDur, err := t.Duration(ctx, j.DubPath)
	if err != nil {
		return res, err
	}
	baseMono, err := t.DecodePCM(ctx, j.BasePath, featRate, 1, j.BaseStream)
	if err != nil {
		return res, err
	}
	dubMono, err := t.DecodePCM(ctx, j.DubPath, featRate, 1, j.DubStream)
	if err != nil {
		return res, err
	}
	baseFeat := ExtractFeatures(baseMono)
	al := Align(baseFeat, ExtractFeatures(dubMono), baseDur, dubDur, j.Options)
	res.Alignment = al
	if len(al.Segments) == 0 {
		res.Notes = append(res.Notes, "no window locked: the two files are probably different content")
		return res, nil
	}

	dubPCM, err := t.DecodePCM(ctx, j.DubPath, outRate, outChannels, j.DubStream)
	if err != nil {
		return res, err
	}
	var basePCM []int16
	if j.GapFill == GapFillBase && len(al.Gaps) > 0 {
		if basePCM, err = t.DecodePCM(ctx, j.BasePath, outRate, outChannels, j.BaseStream); err != nil {
			return res, err
		}
	}
	rendered := Render(al, dubPCM, basePCM, j.GapFill)

	var n int
	res.ResidualMedian, res.ResidualP95, n = Residual(rendered, baseFeat, al.Segments, j.Options.MinConfidence)
	res.Status, res.Notes = grade(al, res.ResidualMedian, res.ResidualP95, n, j.Gate)

	if j.OutPath != "" && (res.Status == StatusOK || j.Force) {
		if err := t.Mux(ctx, j.BasePath, rendered, j.OutPath, j.Mux); err != nil {
			return res, err
		}
		res.Written = true
	}
	return res, nil
}

func grade(al *Alignment, med, p95 float64, n int, g Gate) (Status, []string) {
	var notes []string
	if al.Coverage < g.MinCoverage {
		notes = append(notes, fmt.Sprintf("coverage %.0f%% < %.0f%%", al.Coverage*100, g.MinCoverage*100))
	}
	switch {
	case n == 0:
		notes = append(notes, "validation found no confident window")
	case med > g.MaxResidualSec:
		notes = append(notes, fmt.Sprintf("median residual %.0f ms > %.0f ms", med*1000, g.MaxResidualSec*1000))
	case p95 > g.MaxResidualSec*3:
		notes = append(notes, fmt.Sprintf("p95 residual %.0f ms", p95*1000))
	}
	if al.WindowsTotal > 0 && al.WindowsLocked*2 < al.WindowsTotal {
		notes = append(notes, fmt.Sprintf("only %d/%d windows locked", al.WindowsLocked, al.WindowsTotal))
	}
	if al.DriftSuspected {
		notes = append(notes, "sources appear to run at different rates; resampling is not implemented")
	}
	if len(notes) > 0 {
		return StatusReview, notes
	}
	return StatusOK, nil
}
