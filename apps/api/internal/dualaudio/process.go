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

// Phases reported through Job.OnPhase, in order.
const (
	PhaseDecode   = "decode"
	PhaseAlign    = "align"
	PhaseValidate = "validate"
	PhaseWrite    = "write"
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
	// MinValidated is the share of validation windows whose peak must land on
	// zero. Clean episodes sit above 0.8; one with a misplaced stretch measured
	// 0.36.
	MinValidated float64
	// MaxOffSec is how many seconds may play at a consistently wrong offset.
	MaxOffSec float64
}

// DefaultGate matches the validated thresholds: 50 ms is below the point where
// dialogue reads as out of sync, and 90% coverage allows for credits and
// previews that exist in only one release.
func DefaultGate() Gate {
	return Gate{MaxResidualSec: 0.05, MinCoverage: 0.90, MinValidated: 0.75, MaxOffSec: 8}
}

// Result is what one episode produced.
type Result struct {
	Base           string     `json:"base"`
	Dub            string     `json:"dub"`
	Status         Status     `json:"status"`
	Notes          []string   `json:"notes,omitempty"`
	Alignment      *Alignment `json:"alignment,omitempty"`
	ResidualMedian float64    `json:"residual_median"`
	ResidualP95    float64    `json:"residual_p95"`
	Validated      int        `json:"validated"`    // validation windows with the peak on zero
	ValidatedOf    int        `json:"validated_of"` // ... out of this many eligible
	Misses         []Miss     `json:"misses,omitempty"`
	OffSec         float64    `json:"off_sec"` // seconds at a consistently wrong offset
	TickSec        float64    `json:"tick_sec"`
	Written        bool       `json:"written"`
	VideoFrom      string     `json:"video_from,omitempty"` // language tag of the video's file
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
	// TickSec is the spacing of the sync checks; 0 means DefaultTickSec.
	TickSec float64
	// Tags are extra container tags. The sync verdict is added to them.
	Tags map[string]string
	// VideoFrom is the language tag of the base file, recorded in the verdict.
	VideoFrom string
	// OnPhase, when set, is told which stage an episode is in.
	OnPhase func(phase string)
}

// Process analyses, rebuilds, validates and — only if the gate passes — writes
// one episode. The point of the gate is that an episode which did not line up
// is reported with its reason rather than shipped quietly out of sync.
func Process(ctx context.Context, t Tools, j Job) (*Result, error) {
	res := &Result{Base: j.BasePath, Dub: j.DubPath, Status: StatusFail, VideoFrom: j.VideoFrom}
	phase := func(p string) {
		if j.OnPhase != nil {
			j.OnPhase(p)
		}
	}
	phase(PhaseDecode)

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
	phase(PhaseAlign)
	baseFeat, dubFeat := ExtractFeatures(baseMono), ExtractFeatures(dubMono)
	al := Align(baseFeat, dubFeat, baseDur, dubDur, j.Options)
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
	if j.GapFill == GapFillBase {
		if basePCM, err = t.DecodePCM(ctx, j.BasePath, outRate, outChannels, j.BaseStream); err != nil {
			return res, err
		}
	}
	phase(PhaseValidate)
	rendered, best := Refine(al, baseFeat, dubFeat, dubPCM, basePCM, j.GapFill, j.Options.MinConfidence, j.TickSec)
	res.TickSec = best.TickSec
	res.ResidualMedian, res.ResidualP95 = best.Median, best.P95
	res.Validated, res.ValidatedOf, res.Misses, res.OffSec = best.Hits, best.Eligible, best.Misses, best.OffSec
	res.Status, res.Notes = grade(al, best, j.Gate)

	if j.OutPath != "" && (res.Status == StatusOK || j.Force) {
		phase(PhaseWrite)
		mo := j.Mux
		mo.Tags = map[string]string{}
		for k, v := range j.Tags {
			mo.Tags[k] = v
		}
		// The verdict travels WITH the file, so anything that lists it later can
		// say how good the sync is without re-measuring it.
		sync := res.SyncInfo()
		mo.Tags[SyncTag] = sync.Encode()
		mo.Tags[SyncSummaryTag] = sync.Summary()
		if err := t.Mux(ctx, j.BasePath, rendered, j.OutPath, mo); err != nil {
			return res, err
		}
		res.Written = true
	}
	return res, nil
}

const maxSolveRounds = 4

func betterThan(v, best Validation) bool {
	if v.OffSec != best.OffSec {
		return v.OffSec < best.OffSec
	}
	return len(v.Offs) < len(best.Offs)
}

// Refine renders, validates and re-solves in a closed loop, and leaves al at
// the best map found.
//
// Validation does not only fail an episode, it MEASURES what is missing: a
// confident residual of +0.14 s over a stretch says the right lag there is the
// segment's lag + 0.14. Those become candidate lags, the solver runs again, and
// the loop goes on while the consistent error shrinks.
func Refine(al *Alignment, baseFeat, dubFeat *Features, dubPCM, basePCM []int16, fill GapFill, minConf, tickSec float64) ([]int16, Validation) {
	var rendered []int16
	var best Validation
	bestSegs, bestGaps, bestCov := al.Segments, al.Gaps, al.Coverage
	for round := 0; round < maxSolveRounds; round++ {
		out := Render(al, dubPCM, basePCM, fill)
		v := Validate(out, baseFeat, al.Segments, minConf, tickSec)
		if round > 0 && !betterThan(v, best) {
			break // no better: keep the best so far
		}
		rendered, best = out, v
		bestSegs, bestGaps, bestCov = al.Segments, al.Gaps, al.Coverage
		// Feed back EVERY confident disagreement, not only consistent runs: a
		// segment too short for the sweep yields a single disagreeing window. A
		// false one costs a round and nothing else — a candidate that does not
		// fit simply never scores.
		if !al.AddCandidates(v.Offs) {
			break
		}
		al.Solve(baseFeat, dubFeat)
		if len(al.Segments) == 0 {
			break
		}
	}
	al.Segments, al.Gaps, al.Coverage = bestSegs, bestGaps, bestCov

	// The loop can only offer the solver new LAGS. A seam in the wrong place
	// with both lags already correct is invisible to it — re-solving returns the
	// same map — so that case is tried separately, and kept only if it validates
	// better. Segments are copied first: SnapSeams edits in place, and bestSegs
	// is the rollback.
	al.Segments = append([]Segment(nil), bestSegs...)
	if al.SnapSeams(baseFeat, dubFeat, best.Runs) {
		out := Render(al, dubPCM, basePCM, fill)
		if v := Validate(out, baseFeat, al.Segments, minConf, tickSec); betterThan(v, best) {
			rendered, best = out, v
		} else {
			al.Segments, al.Gaps, al.Coverage = bestSegs, bestGaps, bestCov
		}
	} else {
		al.Segments = bestSegs
	}
	return rendered, best
}

func grade(al *Alignment, v Validation, g Gate) (Status, []string) {
	med, p95, n, eligible := v.Median, v.P95, v.Hits, v.Eligible
	var notes []string
	if al.Coverage < g.MinCoverage {
		notes = append(notes, fmt.Sprintf("coverage %.0f%% < %.0f%%", al.Coverage*100, g.MinCoverage*100))
	}
	switch {
	case eligible == 0:
		notes = append(notes, "validation found no window to check")
	case med > g.MaxResidualSec:
		notes = append(notes, fmt.Sprintf("median residual %.0f ms > %.0f ms", med*1000, g.MaxResidualSec*1000))
	case p95 > g.MaxResidualSec*3:
		notes = append(notes, fmt.Sprintf("p95 residual %.0f ms", p95*1000))
	}
	if v.OffSec > g.MaxOffSec {
		notes = append(notes, fmt.Sprintf("%.0f s playing at a consistently wrong offset", v.OffSec))
	}
	if eligible > 0 && float64(n) < g.MinValidated*float64(eligible) {
		notes = append(notes, fmt.Sprintf("only %d/%d validation windows in place", n, eligible))
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
