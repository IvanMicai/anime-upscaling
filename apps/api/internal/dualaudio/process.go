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
	"math"
	"os"
)

// Phases reported through Job.OnPhase, in order.
const (
	PhaseDecode   = "decode"
	PhaseSeparate = "separate"
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
	// VoiceSeparation is "used", or why it was not, when it was asked for.
	VoiceSeparation  string  `json:"voice_separation,omitempty"`
	Written          bool    `json:"written"`
	VisualCandidates int     `json:"visual_candidates,omitempty"`
	VideoFrom        string  `json:"video_from,omitempty"` // language tag of the video's file
	GapFill          GapFill `json:"gap_fill,omitempty"`   // content used where no dub was aligned
}

// Job describes one episode to build.
type Job struct {
	BasePath, DubPath, OutPath string
	BaseStream, DubStream      int
	// BaseAnalysisPath and DubAnalysisPath, when set, are what alignment and
	// validation listen to instead of the originals: the same tracks with the
	// voices removed, on the same timeline (stream 0). Dialogue is the one thing
	// the two languages never share, and without it the music, effects and
	// ambience under every line still match. The output is always rendered from
	// the original dub audio.
	BaseAnalysisPath, DubAnalysisPath string
	// VoiceSeparation asks the separator service (Tools.Separator) for both
	// tracks without voices and uses them as the analysis tracks. Without a
	// configured separator, or if it fails, the job goes on with the
	// originals and Result.VoiceSeparation says why.
	VoiceSeparation bool
	Options         Options
	Gate            Gate
	GapFill         GapFill
	Mux             MuxOptions
	// VisualAssist adds audio-confirmed offsets proposed by video matching.
	VisualAssist bool
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
	res := &Result{Base: j.BasePath, Dub: j.DubPath, Status: StatusFail, VideoFrom: j.VideoFrom, GapFill: j.GapFill}
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
	if j.VoiceSeparation && j.BaseAnalysisPath == "" && j.DubAnalysisPath == "" {
		dir, err := os.MkdirTemp("", "dualaudio-voices-")
		if err != nil {
			return res, err
		}
		defer func() { _ = os.RemoveAll(dir) }()
		phase(PhaseSeparate)
		res.VoiceSeparation = t.separateBoth(ctx, &j, dir)
	}
	anaBase, anaBaseStream := j.BasePath, j.BaseStream
	if j.BaseAnalysisPath != "" {
		anaBase, anaBaseStream = j.BaseAnalysisPath, 0
	}
	anaDub, anaDubStream := j.DubPath, j.DubStream
	if j.DubAnalysisPath != "" {
		anaDub, anaDubStream = j.DubAnalysisPath, 0
	}
	baseMono, err := t.DecodePCM(ctx, anaBase, featRate, 1, anaBaseStream)
	if err != nil {
		return res, err
	}
	dubMono, err := t.DecodePCM(ctx, anaDub, featRate, 1, anaDubStream)
	if err != nil {
		return res, err
	}
	phase(PhaseAlign)
	baseFeat, dubFeat := ExtractFeatures(baseMono), ExtractFeatures(dubMono)
	al := Align(baseFeat, dubFeat, baseDur, dubDur, j.Options)
	res.Alignment = al
	visualAttempted := false
	// Audio-only matching can find no initial segment even when a local audio
	// match near a distinctive visual sequence is available. Give the opt-in
	// visual proposal pass a chance before returning. These proposals are still
	// audio-confirmed and the usual validation and write gate remain mandatory.
	if len(al.Segments) == 0 && j.VisualAssist {
		visualAttempted = true
		proposals, err := t.visualCandidatesFor(ctx, j.BasePath, j.DubPath, baseDur, dubDur, baseFeat, dubFeat, j.Options.MinConfidence)
		if err != nil {
			return res, fmt.Errorf("visual assistance: %w", err)
		}
		res.VisualCandidates = len(proposals)
		if len(proposals) > 0 {
			seedVisualCandidates(al, proposals, j.Options.WindowSec)
			al.Solve(baseFeat, dubFeat)
		}
	}
	if len(al.Segments) == 0 {
		res.Notes = append(res.Notes, "no window locked: the two files may differ or matching evidence may be insufficient")
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
	// Validation listens to the analysis tracks; the output uses the originals.
	valDubPCM, valBasePCM := dubPCM, basePCM
	if j.DubAnalysisPath != "" {
		if valDubPCM, err = t.DecodePCM(ctx, anaDub, outRate, outChannels, anaDubStream); err != nil {
			return res, err
		}
	}
	if j.BaseAnalysisPath != "" && j.GapFill == GapFillBase {
		if valBasePCM, err = t.DecodePCM(ctx, anaBase, outRate, outChannels, anaBaseStream); err != nil {
			return res, err
		}
	}
	phase(PhaseValidate)
	rendered, best := Refine(al, baseFeat, dubFeat, valDubPCM, valBasePCM, j.GapFill, j.Options.MinConfidence, j.TickSec)
	if j.VisualAssist && !visualAttempted {
		proposals, err := t.visualCandidatesFor(ctx, j.BasePath, j.DubPath, baseDur, dubDur, baseFeat, dubFeat, j.Options.MinConfidence)
		if err != nil {
			return res, fmt.Errorf("visual assistance: %w", err)
		}
		res.VisualCandidates = len(proposals)
		if len(proposals) > 0 {
			originalSegs := append([]Segment(nil), al.Segments...)
			originalGaps := append([]Gap(nil), al.Gaps...)
			originalCoverage := al.Coverage
			for _, p := range proposals {
				al.addCandidate(int(math.Round(p.lag)), float64(p.frame)+p.lag)
			}
			al.Solve(baseFeat, dubFeat)
			if len(al.Segments) > 0 {
				candidateAudio, candidate := Refine(al, baseFeat, dubFeat, valDubPCM, valBasePCM, j.GapFill, j.Options.MinConfidence, j.TickSec)
				oldAlign := *al
				oldAlign.Segments, oldAlign.Gaps, oldAlign.Coverage = originalSegs, originalGaps, originalCoverage
				oldStatus, _ := grade(&oldAlign, best, j.Gate)
				newStatus, _ := grade(al, candidate, j.Gate)
				if preferVisualResult(oldStatus, newStatus, best, candidate, originalCoverage, al.Coverage) {
					rendered, best = candidateAudio, candidate
				} else {
					al.Segments, al.Gaps, al.Coverage = originalSegs, originalGaps, originalCoverage
				}
			} else {
				al.Segments, al.Gaps, al.Coverage = originalSegs, originalGaps, originalCoverage
			}
		}
	}

	if j.BaseAnalysisPath != "" || j.DubAnalysisPath != "" {
		rendered = Render(al, dubPCM, basePCM, j.GapFill)
		if res.VoiceSeparation == "used" {
			if rendered, best, err = t.consensusCheck(ctx, j, al, baseFeat, dubFeat, dubPCM, basePCM, rendered, best); err != nil {
				return res, err
			}
		}
	}

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

// seedVisualCandidates initializes an alignment that audio-only matching left
// empty. Its inputs have already passed the visual and audio proposal checks.
func seedVisualCandidates(al *Alignment, proposals []estimate, windowSec float64) {
	if al.cand == nil {
		al.cand = make(map[int]*candidate)
	}
	if al.wm == 0 {
		al.wm = int(math.Round(windowSec * fps))
	}
	for _, p := range proposals {
		al.addCandidate(int(math.Round(p.lag)), float64(p.frame)+p.lag)
	}
}

// A proposed visual alignment must not discard audio evidence, even if the
// count of confident off-offset windows decreases.
func preferVisualResult(oldStatus, newStatus Status, old, candidate Validation, oldCoverage, newCoverage float64) bool {
	if candidate.Eligible == 0 || newCoverage < oldCoverage-0.01 {
		return false
	}
	if old.Eligible > 0 && candidate.Hits*old.Eligible < old.Hits*candidate.Eligible {
		return false
	}
	if oldStatus == StatusOK && newStatus != StatusOK {
		return false
	}
	if oldStatus == StatusOK && (candidate.Median > old.Median || candidate.P95 > old.P95) {
		return false
	}
	return (newStatus == StatusOK && oldStatus != StatusOK) || (newStatus == oldStatus && betterThan(candidate, old))
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

	// Short stretches at their own small offset, inside one segment. Each
	// round is kept only if the validation improves; new runs can surface once
	// a neighbouring one is fixed, hence the few rounds.
	for round := 0; round < maxSplitRounds; round++ {
		keepSegs, keepGaps, keepCov := append([]Segment(nil), al.Segments...), al.Gaps, al.Coverage
		if !al.SplitRuns(baseFeat, dubFeat, best.Runs) {
			break
		}
		out := Render(al, dubPCM, basePCM, fill)
		v := Validate(out, baseFeat, al.Segments, minConf, tickSec)
		if !betterThan(v, best) || hitShare(v) < hitShare(best)-0.005 {
			al.Segments, al.Gaps, al.Coverage = keepSegs, keepGaps, keepCov
			break
		}
		rendered, best = out, v
	}

	// Whatever is still far off is different material: leave it to the base.
	keepSegs, keepGaps, keepCov := append([]Segment(nil), al.Segments...), al.Gaps, al.Coverage
	if al.GapRuns(best.Runs) {
		out := Render(al, dubPCM, basePCM, fill)
		v := Validate(out, baseFeat, al.Segments, minConf, tickSec)
		if betterThan(v, best) && hitShare(v) >= hitShare(best)-0.005 {
			rendered, best = out, v
		} else {
			al.Segments, al.Gaps, al.Coverage = keepSegs, keepGaps, keepCov
		}
	}
	return rendered, best
}

const maxSplitRounds = 4

// separateBoth fills j's analysis paths with the voice-free tracks and reports
// "used", or why the originals are used instead. Separation is an aid: a
// missing or failing separator must not stop a merge that may well pass
// without it.
func (t Tools) separateBoth(ctx context.Context, j *Job, dir string) string {
	if t.Separator == "" {
		return "unavailable: no separator configured (SEPARATOR_URL)"
	}
	paths := make([]string, 2)
	for i, src := range []struct {
		path   string
		stream int
		name   string
	}{{j.BasePath, j.BaseStream, "base"}, {j.DubPath, j.DubStream, "dub"}} {
		pcm, err := t.DecodePCM(ctx, src.path, outRate, outChannels, src.stream)
		if err != nil {
			return "failed: " + err.Error()
		}
		if paths[i], err = t.separateVoices(ctx, pcm, dir, src.name); err != nil {
			return "failed: " + err.Error()
		}
	}
	j.BaseAnalysisPath, j.DubAnalysisPath = paths[0], paths[1]
	return "used"
}

// selfCheckRuns aligns a track against the base from scratch and returns every
// stretch of at least selfCheckMinSec that sits measurably off zero, as runs
// SplitRuns understands (the residual is what the map's lag lacks there).
func selfCheckRuns(feat, base *Features) []OffRun {
	if feat == nil || base == nil || feat.Frames < 100 {
		return nil
	}
	dur := float64(base.Frames) / fps
	var runs []OffRun
	keep := func(start, end, lag float64, n int) {
		if end-start >= selfCheckMinSec && math.Abs(lag) >= hitToleranceSec && math.Abs(lag) <= splitMaxLagSec {
			runs = append(runs, OffRun{Start: start, End: end, Lag: lag, Windows: n})
		}
	}
	// The coarse sweep finds long stretches; the fine validation (10 s windows
	// every second, expecting zero everywhere) finds the 10-20 s ones the sweep
	// is too coarse to resolve.
	check := Align(base, feat, dur, float64(feat.Frames)/fps, DefaultOptions())
	for _, s := range check.Segments {
		keep(s.BaseStart, s.BaseEnd, s.Lag, s.Windows)
	}
	v := validateFeatures(feat, base, []Segment{{BaseStart: 0, BaseEnd: dur}}, DefaultOptions().MinConfidence, 1, 0, nil)
	for _, r := range v.Runs {
		keep(r.Start, r.End, r.Lag, r.Windows)
	}
	return runs
}

const (
	selfCheckMinSec = 5.0
	maxCheckTries   = 6 // each costs one separation
)

// consensusCheck is the finished-file check, run before writing. The map was
// optimised on the voice-free tracks, so checking it there is circular: it
// agrees with itself. Two checks that do not share that bias are run on the
// track as it will be written — once with the voices, once separated again
// after rendering. Stretches either finds off zero are tried as corrections (a
// seam move when the corrected lag is a neighbour's, a split otherwise) and
// kept only when the error both measure shrinks and the validation holds.
func (t Tools) consensusCheck(ctx context.Context, j Job, al *Alignment, baseFeat, dubFeat *Features,
	dubPCM, basePCM, rendered []int16, best Validation) ([]int16, Validation, error) {
	origMono, err := t.DecodePCM(ctx, j.BasePath, featRate, 1, j.BaseStream)
	if err != nil {
		return rendered, best, err
	}
	origBase := ExtractFeatures(origMono)
	dir, err := os.MkdirTemp("", "dualaudio-check-")
	if err != nil {
		return rendered, best, err
	}
	defer func() { _ = os.RemoveAll(dir) }()

	// Each check alone misses stretches the other sees, and reports some false
	// ones. So candidates come from either, and each is TRIED: applied,
	// rendered and measured again by both. It stays only if the total error
	// both checks measure goes down and the validation holds. A false
	// detection, applied, creates a real error the other check reports.
	measure := func(out []int16) ([]OffRun, float64, error) {
		withVoices := selfCheckRuns(ExtractFeatures(downmix16k(out)), origBase)
		path, err := t.separateVoices(ctx, out, dir, "check")
		if err != nil {
			return nil, 0, err
		}
		mono, err := t.DecodePCM(ctx, path, featRate, 1, 0)
		if err != nil {
			return nil, 0, err
		}
		separated := selfCheckRuns(ExtractFeatures(mono), baseFeat)
		debugf("check: with voices %+v | separated %+v", withVoices, separated)
		return append(withVoices, separated...), offSeconds(withVoices) + offSeconds(separated), nil
	}
	cands, score, err := measure(rendered)
	if err != nil {
		debugf("check skipped: %v", err)
		return rendered, best, nil // an aid, like the separation itself
	}
	valDub, valBase := dubPCM, basePCM
	if j.DubAnalysisPath != "" {
		if valDub, err = t.DecodePCM(ctx, j.DubAnalysisPath, outRate, outChannels, 0); err != nil {
			return rendered, best, err
		}
	}
	if j.BaseAnalysisPath != "" && j.GapFill == GapFillBase {
		if valBase, err = t.DecodePCM(ctx, j.BaseAnalysisPath, outRate, outChannels, 0); err != nil {
			return rendered, best, err
		}
	}
	tried := 0
	for len(cands) > 0 && tried < maxCheckTries {
		r := cands[0]
		cands = cands[1:]
		keepSegs, keepGaps, keepCov := append([]Segment(nil), al.Segments...), al.Gaps, al.Coverage
		snapped := al.SnapSeams(baseFeat, dubFeat, []OffRun{r})
		split := al.SplitRuns(baseFeat, dubFeat, []OffRun{r})
		if !snapped && !split {
			continue
		}
		tried++
		v := Validate(Render(al, valDub, valBase, j.GapFill), baseFeat, al.Segments, j.Options.MinConfidence, j.TickSec)
		out := Render(al, dubPCM, basePCM, j.GapFill)
		next, nextScore, err := measure(out)
		if err != nil || nextScore >= score || v.OffSec > best.OffSec || hitShare(v) < hitShare(best)-0.01 {
			debugf("check: %+v rejected: error %.0f->%.0f s, validation off %.0f->%.0f, hits %.3f->%.3f",
				r, score, nextScore, best.OffSec, v.OffSec, hitShare(best), hitShare(v))
			al.Segments, al.Gaps, al.Coverage = keepSegs, keepGaps, keepCov
			continue
		}
		debugf("check: %+v kept: error %.0f->%.0f s", r, score, nextScore)
		rendered, best, score, cands = out, v, nextScore, next
	}
	return rendered, best, nil
}

// debugf traces the refinement passes when DUALAUDIO_DEBUG is set.
func debugf(format string, args ...any) {
	if os.Getenv("DUALAUDIO_DEBUG") != "" {
		fmt.Fprintf(os.Stderr, "dualaudio: "+format+"\n", args...)
	}
}

func offSeconds(runs []OffRun) float64 {
	var t float64
	for _, r := range runs {
		t += r.End - r.Start
	}
	return t
}

func hitShare(v Validation) float64 {
	if v.Eligible == 0 {
		return 0
	}
	return float64(v.Hits) / float64(v.Eligible)
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
	// A slow lag drift is approximated by a staircase of constant segments.
	// Whether that staircase is fine enough is exactly what the measurements
	// above decide, so drift only explains a failure; it is not one on its own.
	// Checked on three Pokemon episodes flagged for drift alone: each finished
	// file then verified at 0-10 ms against its own base track.
	if al.DriftSuspected && len(notes) > 0 {
		notes = append(notes, "sources appear to run at different rates; resampling is not implemented")
	}
	if len(notes) > 0 {
		return StatusReview, notes
	}
	return StatusOK, nil
}
