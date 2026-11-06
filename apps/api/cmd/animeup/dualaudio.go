package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	"anime-upscaling/internal/config"
	"anime-upscaling/internal/dualaudio"
)

var dualaudioExts = map[string]bool{".mkv": true, ".mp4": true, ".m4v": true}

func cmdDualAudio(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("usage: animeup dualaudio <match|build|analyze|visual|visual-regions|visual-motion|visual-refine|visual-fill|visual-audio|visual-seams|visual-seams-evaluate|compare-alignments|shared-audio|waveform-gap|waveform-probe> [flags]")
	}
	cfg := config.NewConfig()
	tools := dualaudio.Tools{FFmpeg: cfg.FFmpegBin, FFprobe: cfg.FFprobeBin}
	switch args[0] {
	case "match":
		return dualaudioMatch(ctx, tools, args[1:])
	case "build":
		return dualaudioBuild(ctx, tools, args[1:])
	case "analyze":
		return dualaudioAnalyze(ctx, tools, args[1:])
	case "visual":
		return dualaudioVisual(ctx, tools, args[1:])
	case "shared-audio":
		return dualaudioSharedAudio(ctx, tools, args[1:])
	case "waveform-gap":
		return dualaudioWaveformGap(ctx, tools, args[1:])
	case "waveform-probe":
		return dualaudioWaveformProbe(ctx, tools, args[1:])
	case "visual-audio":
		return dualaudioVisualAudio(ctx, tools, args[1:])
	case "visual-refine":
		return dualaudioVisualRefine(ctx, tools, args[1:])
	case "compare-alignments":
		return dualaudioCompareAlignments(ctx, tools, args[1:])
	case "visual-seams-evaluate":
		return dualaudioVisualSeamsEvaluate(ctx, tools, args[1:])
	case "visual-seams":
		return dualaudioVisualSeams(args[1:])
	case "visual-regions":
		return dualaudioVisualRegions(ctx, tools, args[1:])
	case "visual-motion":
		return dualaudioVisualMotion(ctx, tools, args[1:])
	case "visual-fill":
		return dualaudioVisualFill(ctx, tools, args[1:])
	}
	return fmt.Errorf("unknown dualaudio command: %s", args[0])
}

func dualaudioVisualMotion(ctx context.Context, tools dualaudio.Tools, args []string) error {
	fs := flag.NewFlagSet("dualaudio visual-motion", flag.ContinueOnError)
	base := fs.String("base", "", "reference video")
	dub := fs.String("dub", "", "dubbed video")
	start := fs.Float64("base-start", math.NaN(), "reference window start")
	center := fs.Float64("dub-center", math.NaN(), "expected dubbed window start")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *base == "" || *dub == "" {
		return errors.New("--base and --dub are required")
	}
	result, err := dualaudio.CompareVisualMotion(ctx, tools, *base, *dub, *start, *center)
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(struct {
		Status   string                         `json:"status"`
		Warning  string                         `json:"warning"`
		Evidence dualaudio.VisualMotionEvidence `json:"evidence"`
	}{"review", "Motion similarity does not establish dubbed audio sync; no media was written.", result})
}

func dualaudioWaveformProbe(ctx context.Context, tools dualaudio.Tools, args []string) error {
	fs := flag.NewFlagSet("dualaudio waveform-probe", flag.ContinueOnError)
	base := fs.String("base", "", "reference audio file")
	dub := fs.String("dub", "", "dubbed audio file")
	baseStart := fs.Float64("base-start", math.NaN(), "reference window start")
	dubStart := fs.Float64("dub-start", math.NaN(), "dubbed window start")
	window := fs.Float64("window", .5, "duration of the dubbed query, 0.5 to 3 seconds")
	stream := fs.Int("dub-stream", 0, "audio stream index in dub")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *base == "" || *dub == "" {
		return errors.New("--base and --dub are required")
	}
	probe, err := dualaudio.ProbeStereoWaveformWindow(ctx, tools, *base, *dub, *stream, *baseStart, *dubStart, *window)
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(struct {
		Status  string                     `json:"status"`
		Warning string                     `json:"warning"`
		Window  float64                    `json:"window_seconds"`
		Probe   dualaudio.WaveformEvidence `json:"probe"`
	}{"review", "Shared sound is not independent proof of lip sync; no media was written.", *window, probe})
}

func dualaudioWaveformGap(ctx context.Context, tools dualaudio.Tools, args []string) error {
	fs := flag.NewFlagSet("dualaudio waveform-gap", flag.ContinueOnError)
	base := fs.String("base", "", "reference video and audio")
	dub := fs.String("dub", "", "dubbed video and audio")
	mapPath := fs.String("alignment", "", "alignment JSON or analyze --json result")
	timelinePath := fs.String("timeline", "", "visual timeline JSON")
	start := fs.Float64("start", math.NaN(), "gap start in reference seconds")
	end := fs.Float64("end", math.NaN(), "gap end in reference seconds")
	stream := fs.Int("dub-stream", 0, "audio stream index in dub")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *base == "" || *dub == "" || *mapPath == "" || *timelinePath == "" {
		return errors.New("--base, --dub, --alignment and --timeline are required")
	}
	raw, err := os.ReadFile(*mapPath)
	if err != nil {
		return err
	}
	var wrapped struct {
		Alignment *dualaudio.Alignment `json:"alignment"`
	}
	if err = json.Unmarshal(raw, &wrapped); err != nil {
		return fmt.Errorf("alignment: %w", err)
	}
	if wrapped.Alignment == nil {
		wrapped.Alignment = &dualaudio.Alignment{}
		if err = json.Unmarshal(raw, wrapped.Alignment); err != nil {
			return fmt.Errorf("alignment: %w", err)
		}
	}
	raw, err = os.ReadFile(*timelinePath)
	if err != nil {
		return err
	}
	var timeline dualaudio.VisualTimeline
	if err = json.Unmarshal(raw, &timeline); err != nil {
		return fmt.Errorf("visual timeline: %w", err)
	}
	result, err := dualaudio.ProposeWaveformGap(ctx, tools, *base, *dub, *stream, wrapped.Alignment, &timeline, dualaudio.Gap{Start: *start, End: *end})
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(struct {
		Status    string                         `json:"status"`
		Warning   string                         `json:"warning"`
		Candidate *dualaudio.WaveformGapProposal `json:"candidate"`
	}{"review", "Shared sound and visual anchors do not prove dubbed-source lip sync; no media was written.", result})
}

// shared-audio is an exploratory review aid, not a language or lip-sync check.
func dualaudioSharedAudio(ctx context.Context, tools dualaudio.Tools, args []string) error {
	fs := flag.NewFlagSet("dualaudio shared-audio", flag.ContinueOnError)
	file := fs.String("file", "", "file containing both audio streams")
	reference := fs.Int("reference-stream", 0, "reference audio stream index")
	candidate := fs.Int("candidate-stream", 1, "candidate audio stream index")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *file == "" || *reference < 0 || *candidate < 0 || *reference == *candidate {
		return errors.New("--file and two different nonnegative audio stream indexes are required")
	}
	a, err := tools.DecodePCM(ctx, *file, 8000, 1, *reference)
	if err != nil {
		return err
	}
	b, err := tools.DecodePCM(ctx, *file, 8000, 1, *candidate)
	if err != nil {
		return err
	}
	windows, err := dualaudio.FindSharedAudioWindows(a, b)
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(struct {
		Warning string                        `json:"warning"`
		Windows []dualaudio.SharedAudioWindow `json:"windows"`
	}{
		"Exploratory shared audio only: music/effects may match, and unmatched windows are not proof of different audio or speech.", windows,
	})
}

// dualaudioVisualSeams reports possible boundary intervals without editing media.
func dualaudioVisualSeams(args []string) error {
	fs := flag.NewFlagSet("dualaudio visual-seams", flag.ContinueOnError)
	timeline := fs.String("timeline", "", "visual timeline JSON")
	alignment := fs.String("alignment", "", "alignment JSON or analyze --json result")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *timeline == "" || *alignment == "" {
		return errors.New("--timeline and --alignment are required")
	}
	raw, err := os.ReadFile(*timeline)
	if err != nil {
		return err
	}
	var visual dualaudio.VisualTimeline
	if err := json.Unmarshal(raw, &visual); err != nil {
		return fmt.Errorf("visual timeline: %w", err)
	}
	raw, err = os.ReadFile(*alignment)
	if err != nil {
		return err
	}
	var result struct {
		Alignment *dualaudio.Alignment `json:"alignment"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return fmt.Errorf("alignment: %w", err)
	}
	if result.Alignment == nil {
		result.Alignment = &dualaudio.Alignment{}
		if err := json.Unmarshal(raw, result.Alignment); err != nil {
			return fmt.Errorf("alignment: %w", err)
		}
	}
	if len(result.Alignment.Segments) == 0 {
		return errors.New("alignment contains no segments")
	}
	reviews := dualaudio.FindVisualBoundaryReviews(&visual, result.Alignment.Segments)
	if reviews == nil {
		reviews = []dualaudio.VisualBoundaryReview{}
	}
	return json.NewEncoder(os.Stdout).Encode(struct {
		Warning    string                           `json:"warning"`
		Boundaries []dualaudio.VisualBoundaryReview `json:"boundaries"`
	}{
		"Review only: visual evidence does not prove audio or lip synchronization, and intervals are not automatic edits.", reviews,
	})
}

// dualaudioVisualSeamsEvaluate tests one proposed correction in memory only.
func dualaudioVisualSeamsEvaluate(ctx context.Context, tools dualaudio.Tools, args []string) error {
	fs := flag.NewFlagSet("dualaudio visual-seams-evaluate", flag.ContinueOnError)
	base := fs.String("base", "", "video and reference audio file")
	dub := fs.String("dub", "", "dubbed audio file")
	stream := fs.Int("dub-stream", 0, "audio stream index in dub")
	timeline := fs.String("timeline", "", "visual timeline JSON")
	alignment := fs.String("alignment", "", "alignment JSON or analyze --json result")
	tick := fs.Float64("tick", 1, "seconds between validation windows (1-30)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *base == "" || *dub == "" || *timeline == "" || *alignment == "" || *stream < 0 || math.IsNaN(*tick) || math.IsInf(*tick, 0) || *tick < dualaudio.MinTickSec || *tick > dualaudio.MaxTickSec {
		return errors.New("--base, --dub, --timeline, --alignment and a tick between 1 and 30 are required")
	}
	raw, err := os.ReadFile(*timeline)
	if err != nil {
		return err
	}
	var visual dualaudio.VisualTimeline
	if err = json.Unmarshal(raw, &visual); err != nil {
		return fmt.Errorf("visual timeline: %w", err)
	}
	raw, err = os.ReadFile(*alignment)
	if err != nil {
		return err
	}
	var wrapper struct {
		Alignment *dualaudio.Alignment `json:"alignment"`
	}
	if err = json.Unmarshal(raw, &wrapper); err != nil {
		return fmt.Errorf("alignment: %w", err)
	}
	if wrapper.Alignment == nil {
		wrapper.Alignment = &dualaudio.Alignment{}
		if err = json.Unmarshal(raw, wrapper.Alignment); err != nil {
			return fmt.Errorf("alignment: %w", err)
		}
	}
	a := wrapper.Alignment
	if len(a.Segments) == 0 || a.BaseDuration <= 0 || a.DubDuration <= 0 {
		return errors.New("alignment requires segments and positive durations")
	}
	reviews := dualaudio.FindVisualBoundaryReviews(&visual, a.Segments)
	if len(reviews) == 0 {
		return json.NewEncoder(os.Stdout).Encode(struct {
			Status string `json:"status"`
			Reason string `json:"reason"`
		}{"review", "no supported visual boundary candidate"})
	}
	candidate, err := dualaudio.ProposeVisualBoundaryAlignment(a, reviews[0])
	if err != nil {
		return err
	}
	// The supplied map must describe these files; otherwise the in-memory
	// comparison could be misleading or allocate for an unrelated duration.
	baseDuration, err := tools.Duration(ctx, *base)
	if err != nil {
		return err
	}
	dubDuration, err := tools.Duration(ctx, *dub)
	if err != nil {
		return err
	}
	if baseDuration > 7200 || dubDuration > 7200 || math.Abs(baseDuration-a.BaseDuration) > .25 || math.Abs(dubDuration-a.DubDuration) > .25 {
		return errors.New("alignment durations do not match the supplied files or exceed two hours")
	}
	baseMono, err := tools.DecodePCM(ctx, *base, 16000, 1, 0)
	if err != nil {
		return err
	}
	baseFeat := dualaudio.ExtractFeatures(baseMono)
	dubPCM, err := tools.DecodePCM(ctx, *dub, 48000, 2, *stream)
	if err != nil {
		return err
	}
	basePCM, err := tools.DecodePCM(ctx, *base, 48000, 2, 0)
	if err != nil {
		return err
	}
	measure := func(al *dualaudio.Alignment) dualaudio.Validation {
		rendered := dualaudio.Render(al, dubPCM, basePCM, dualaudio.GapFillBase)
		return dualaudio.Validate(rendered, baseFeat, al.Segments, dualaudio.DefaultOptions().MinConfidence, *tick)
	}
	before := measure(a)
	after := measure(candidate)
	type metrics struct {
		Hits     int     `json:"hits"`
		Eligible int     `json:"eligible"`
		OffSec   float64 `json:"off_seconds"`
		Median   float64 `json:"median_seconds"`
		P95      float64 `json:"p95_seconds"`
	}
	compact := func(v dualaudio.Validation) metrics { return metrics{v.Hits, v.Eligible, v.OffSec, v.Median, v.P95} }
	return json.NewEncoder(os.Stdout).Encode(struct {
		Status    string                         `json:"status"`
		Warning   string                         `json:"warning"`
		Boundary  dualaudio.VisualBoundaryReview `json:"boundary"`
		Baseline  metrics                        `json:"baseline"`
		Candidate metrics                        `json:"candidate"`
		Improved  bool                           `json:"audio_metric_improved"`
		Alignment *dualaudio.Alignment           `json:"candidate_alignment"`
	}{
		"review", "Same-material validation is not independent proof of lip sync; no media was written.", reviews[0], compact(before), compact(after), dualaudio.BetterVisualBoundaryEvidence(before, after), candidate,
	})
}

// dualaudioCompareAlignments evaluates two maps on their common covered windows.
func dualaudioCompareAlignments(ctx context.Context, tools dualaudio.Tools, args []string) error {
	fs := flag.NewFlagSet("dualaudio compare-alignments", flag.ContinueOnError)
	base := fs.String("base", "", "video and reference audio file")
	dub := fs.String("dub", "", "dubbed audio file")
	first := fs.String("first", "", "first alignment JSON or analyze --json result")
	second := fs.String("second", "", "second alignment JSON or analyze --json result")
	timeline := fs.String("timeline", "", "optional visual timeline JSON")
	stream := fs.Int("dub-stream", 0, "audio stream index in dub")
	tick := fs.Float64("tick", 1, "seconds between validation windows (1-30)")
	stereo := fs.Bool("stereo", true, "compare stereo features instead of mono")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *base == "" || *dub == "" || *first == "" || *second == "" || *stream < 0 || math.IsNaN(*tick) || math.IsInf(*tick, 0) || *tick < dualaudio.MinTickSec || *tick > dualaudio.MaxTickSec {
		return errors.New("--base, --dub, --first, --second and a tick between 1 and 30 are required")
	}
	load := func(path string) (*dualaudio.Alignment, error) {
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		var result struct {
			Alignment *dualaudio.Alignment `json:"alignment"`
		}
		if err = json.Unmarshal(raw, &result); err != nil {
			return nil, fmt.Errorf("alignment: %w", err)
		}
		if result.Alignment == nil {
			result.Alignment = &dualaudio.Alignment{}
			if err = json.Unmarshal(raw, result.Alignment); err != nil {
				return nil, fmt.Errorf("alignment: %w", err)
			}
		}
		if len(result.Alignment.Segments) == 0 {
			return nil, errors.New("alignment contains no segments")
		}
		return result.Alignment, nil
	}
	a, err := load(*first)
	if err != nil {
		return err
	}
	b, err := load(*second)
	if err != nil {
		return err
	}
	common, err := dualaudio.CommonCoverageSegments(a, b)
	if err != nil {
		return err
	}
	if len(common) == 0 {
		return json.NewEncoder(os.Stdout).Encode(struct {
			Status string `json:"status"`
			Reason string `json:"reason"`
		}{"review", "no common mapped intervals"})
	}
	var visualEvidence *dualaudio.VisualComparison
	if *timeline != "" {
		raw, err := os.ReadFile(*timeline)
		if err != nil {
			return err
		}
		var visual dualaudio.VisualTimeline
		if err := json.Unmarshal(raw, &visual); err != nil {
			return fmt.Errorf("visual timeline: %w", err)
		}
		evidence, err := dualaudio.CompareAlignmentVisualEvidence(&visual, a, b, .15)
		if err != nil {
			return err
		}
		visualEvidence = &evidence
	}
	baseDuration, err := tools.Duration(ctx, *base)
	if err != nil {
		return err
	}
	dubDuration, err := tools.Duration(ctx, *dub)
	if err != nil {
		return err
	}
	if baseDuration > 7200 || dubDuration > 7200 || math.Abs(baseDuration-a.BaseDuration) > .25 || math.Abs(dubDuration-a.DubDuration) > .25 {
		return errors.New("alignment durations do not match the supplied files or exceed two hours")
	}
	channels := 1
	if *stereo {
		channels = 2
	}
	baseRaw, err := tools.DecodePCM(ctx, *base, 16000, channels, 0)
	if err != nil {
		return err
	}
	var baseFeat *dualaudio.Features
	if *stereo {
		baseFeat = dualaudio.ExtractStereoFeatures(baseRaw)
	} else {
		baseFeat = dualaudio.ExtractFeatures(baseRaw)
	}
	dubPCM, err := tools.DecodePCM(ctx, *dub, 48000, 2, *stream)
	if err != nil {
		return err
	}
	basePCM, err := tools.DecodePCM(ctx, *base, 48000, 2, 0)
	if err != nil {
		return err
	}
	measure := func(al *dualaudio.Alignment) dualaudio.Validation {
		rendered := dualaudio.Render(al, dubPCM, basePCM, dualaudio.GapFillBase)
		if *stereo {
			return dualaudio.ValidateStereo(rendered, baseFeat, common, dualaudio.DefaultOptions().MinConfidence, *tick)
		}
		return dualaudio.Validate(rendered, baseFeat, common, dualaudio.DefaultOptions().MinConfidence, *tick)
	}
	before, after := measure(a), measure(b)
	seconds := 0.0
	for _, v := range common {
		seconds += v.BaseEnd - v.BaseStart
	}
	type metrics struct {
		Hits     int                `json:"hits"`
		Eligible int                `json:"eligible"`
		OffSec   float64            `json:"off_seconds"`
		Median   float64            `json:"median_seconds"`
		P95      float64            `json:"p95_seconds"`
		OffRuns  []dualaudio.OffRun `json:"off_runs,omitempty"`
	}
	compact := func(v dualaudio.Validation) metrics {
		return metrics{v.Hits, v.Eligible, v.OffSec, v.Median, v.P95, v.Runs}
	}
	var both *bool
	if visualEvidence != nil {
		value := dualaudio.BetterVisualBoundaryEvidence(before, after) && visualEvidence.VisualMetricNotWorse
		both = &value
	}
	return json.NewEncoder(os.Stdout).Encode(struct {
		Status        string                      `json:"status"`
		Warning       string                      `json:"warning"`
		Stereo        bool                        `json:"stereo"`
		CommonSeconds float64                     `json:"common_mapped_seconds"`
		First         metrics                     `json:"first"`
		Second        metrics                     `json:"second"`
		Improved      bool                        `json:"audio_metric_improved"`
		Visual        *dualaudio.VisualComparison `json:"visual_evidence,omitempty"`
		Both          *bool                       `json:"audio_and_visual_metrics_favor_second,omitempty"`
	}{"review", "Same-material metrics are not independent proof of lip sync; no media was written.", *stereo, seconds, compact(before), compact(after), dualaudio.BetterVisualBoundaryEvidence(before, after), visualEvidence, both})
}

func dualaudioVisualRegions(ctx context.Context, tools dualaudio.Tools, args []string) error {
	fs := flag.NewFlagSet("dualaudio visual-regions", flag.ContinueOnError)
	base := fs.String("base", "", "file providing the reference video")
	dub := fs.String("dub", "", "file providing the comparison video")
	baseStart := fs.Float64("base-start", math.NaN(), "start of the two-second reference window")
	dubCenter := fs.Float64("dub-center", math.NaN(), "expected start of the matching window in the comparison video")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *base == "" || *dub == "" {
		return errors.New("--base and --dub are required")
	}
	result, err := dualaudio.CompareVisualRegions(ctx, tools, *base, *dub, *baseStart, *dubCenter)
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(result)
}

func dualaudioVisualFill(ctx context.Context, tools dualaudio.Tools, args []string) error {
	fs := flag.NewFlagSet("dualaudio visual-fill", flag.ContinueOnError)
	base := fs.String("base", "", "file that provides the video")
	dub := fs.String("dub", "", "file that provides the dubbed audio")
	timeline := fs.String("timeline", "", "JSON from dualaudio visual or visual-refine")
	limit := fs.Int("limit", 100, "maximum missing windows to examine (1-1000)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *base == "" || *dub == "" || *timeline == "" {
		return errors.New("--base, --dub and --timeline are required")
	}
	data, err := os.ReadFile(*timeline)
	if err != nil {
		return err
	}
	var visual dualaudio.VisualTimeline
	if err := json.Unmarshal(data, &visual); err != nil {
		return fmt.Errorf("visual timeline: %w", err)
	}
	result, err := dualaudio.FillVisualGaps(ctx, tools, *base, *dub, &visual, *limit)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "examined %d missing windows; recovered %d; result remains review-only\n", result.Attempted, result.Recovered)
	return json.NewEncoder(os.Stdout).Encode(result.Timeline)
}

func dualaudioVisualRefine(ctx context.Context, tools dualaudio.Tools, args []string) error {
	fs := flag.NewFlagSet("dualaudio visual-refine", flag.ContinueOnError)
	base := fs.String("base", "", "file that provides the video")
	dub := fs.String("dub", "", "file that provides the dubbed audio")
	timeline := fs.String("timeline", "", "JSON from dualaudio visual --timeline")
	limit := fs.Int("limit", 8, "maximum windows to examine (1-100)")
	acceptedAt := fs.String("accepted-at", "", "comma-separated start times of accepted windows to recheck at 24 fps")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *base == "" || *dub == "" || *timeline == "" {
		return errors.New("--base, --dub and --timeline are required")
	}
	data, err := os.ReadFile(*timeline)
	if err != nil {
		return err
	}
	var visual dualaudio.VisualTimeline
	if err := json.Unmarshal(data, &visual); err != nil {
		return fmt.Errorf("visual timeline: %w", err)
	}
	var result *dualaudio.VisualRefinement
	if *acceptedAt != "" {
		parts := strings.Split(*acceptedAt, ",")
		if *limit < 1 || *limit > 100 || len(parts) > *limit {
			return errors.New("--accepted-at exceeds --limit or the limit is invalid")
		}
		var starts []float64
		for _, part := range parts {
			v, e := strconv.ParseFloat(strings.TrimSpace(part), 64)
			if e != nil {
				return fmt.Errorf("invalid accepted window start %q", part)
			}
			starts = append(starts, v)
		}
		result, err = dualaudio.RefineVisualAccepted(ctx, tools, *base, *dub, &visual, starts)
	} else {
		result, err = dualaudio.RefineVisualRejections(ctx, tools, *base, *dub, &visual, *limit)
	}
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "examined %d windows; refined %d; result remains review-only\n", result.Attempted, result.Recovered)
	return json.NewEncoder(os.Stdout).Encode(result.Timeline)
}

// dualaudioVisualAudio evaluates a saved timeline without writing media. The
// report is explicitly review-only even if audio metrics happen to pass.
func dualaudioVisualAudio(ctx context.Context, tools dualaudio.Tools, args []string) error {
	fs := flag.NewFlagSet("dualaudio visual-audio", flag.ContinueOnError)
	base := fs.String("base", "", "file that provides the video")
	dub := fs.String("dub", "", "file that provides the dubbed audio")
	timeline := fs.String("timeline", "", "JSON from dualaudio visual --timeline")
	stream := fs.Int("dub-stream", 0, "audio stream index inside --dub")
	stereo := fs.Bool("stereo", false, "experimentally analyze left and right channels separately")
	repair := fs.Bool("repair-audio", false, "experimentally test localized stereo audio corrections")
	holdout := fs.Bool("holdout", false, "reserve non-overlapping stereo windows for evaluation")
	waveformAt := fs.String("waveform-at", "", "comma-separated base start times for experimental three-second stereo waveform probes")
	waveformEvery := fs.Float64("waveform-every", 0, "experimentally sample stereo waveform probes at this interval in seconds (minimum 10)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *base == "" || *dub == "" || *timeline == "" {
		return errors.New("--base, --dub and --timeline are required")
	}
	if *repair && !*stereo {
		return errors.New("--repair-audio requires --stereo")
	}
	if *holdout && !*stereo {
		return errors.New("--holdout requires --stereo")
	}
	if *holdout && *repair {
		return errors.New("--holdout cannot be combined with --repair-audio")
	}
	if math.IsNaN(*waveformEvery) || math.IsInf(*waveformEvery, 0) || *waveformEvery < 0 || (*waveformEvery > 0 && *waveformEvery < 10) {
		return errors.New("--waveform-every must be zero or at least 10 seconds")
	}
	if *waveformEvery > 0 && (*waveformAt != "" || !*stereo || *holdout) {
		return errors.New("--waveform-every requires --stereo and cannot be combined with --waveform-at or --holdout")
	}
	var probeTimes []float64
	if *waveformAt != "" {
		if !*stereo || *holdout {
			return errors.New("--waveform-at requires --stereo and cannot be combined with --holdout")
		}
		parts := strings.Split(*waveformAt, ",")
		if len(parts) > 20 {
			return errors.New("--waveform-at accepts at most 20 times")
		}
		for _, part := range parts {
			t, err := strconv.ParseFloat(strings.TrimSpace(part), 64)
			if err != nil {
				return fmt.Errorf("invalid waveform probe time %q", part)
			}
			probeTimes = append(probeTimes, t)
		}
	}
	data, err := os.ReadFile(*timeline)
	if err != nil {
		return err
	}
	var visual dualaudio.VisualTimeline
	if err := json.Unmarshal(data, &visual); err != nil {
		return fmt.Errorf("visual timeline: %w", err)
	}
	baseDur, err := tools.Duration(ctx, *base)
	if err != nil {
		return err
	}
	dubDur, err := tools.Duration(ctx, *dub)
	if err != nil {
		return err
	}
	if *waveformEvery > 0 {
		if math.Ceil(baseDur / *waveformEvery) > 200 {
			return errors.New("--waveform-every would exceed 200 probes; increase the interval")
		}
		for at := *waveformEvery / 2; at+4.5 <= baseDur; at += *waveformEvery {
			probeTimes = append(probeTimes, at)
		}
	}
	channels := 1
	if *stereo {
		channels = 2
	}
	baseMono, err := tools.DecodePCM(ctx, *base, 16000, channels, 0)
	if err != nil {
		return err
	}
	dubMono, err := tools.DecodePCM(ctx, *dub, 16000, channels, *stream)
	if err != nil {
		return err
	}
	var baseFeat, dubFeat *dualaudio.Features
	if *stereo {
		baseFeat, dubFeat = dualaudio.ExtractStereoFeatures(baseMono), dualaudio.ExtractStereoFeatures(dubMono)
	} else {
		baseFeat, dubFeat = dualaudio.ExtractFeatures(baseMono), dualaudio.ExtractFeatures(dubMono)
	}
	var candidate *dualaudio.VisualAudioCandidate
	if *holdout {
		candidate, err = dualaudio.VisualAudioAlignHoldout(&visual, baseFeat, dubFeat, baseDur, dubDur, dualaudio.DefaultOptions().MinConfidence)
	} else {
		candidate, err = dualaudio.VisualAudioAlign(&visual, baseFeat, dubFeat, baseDur, dubDur, dualaudio.DefaultOptions().MinConfidence)
	}
	if err != nil {
		return err
	}
	dubPCM, err := tools.DecodePCM(ctx, *dub, 48000, 2, *stream)
	if err != nil {
		return err
	}
	basePCM, err := tools.DecodePCM(ctx, *base, 48000, 2, 0)
	if err != nil {
		return err
	}
	rendered := dualaudio.Render(candidate.Alignment, dubPCM, basePCM, dualaudio.GapFillBase)
	var v dualaudio.Validation
	if *holdout {
		v = dualaudio.ValidateStereoHoldout(rendered, baseFeat, candidate.Alignment.Segments, candidate.TrainingSourceWindows, dualaudio.DefaultOptions().MinConfidence)
	} else if *stereo {
		v = dualaudio.ValidateStereo(rendered, baseFeat, candidate.Alignment.Segments, dualaudio.DefaultOptions().MinConfidence, 5)
	} else {
		v = dualaudio.Validate(rendered, baseFeat, candidate.Alignment.Segments, dualaudio.DefaultOptions().MinConfidence, 5)
	}
	var repairs []dualaudio.OffRun
	if *repair && len(v.Runs) > 0 {
		proposed, runs := dualaudio.ProposeVisualAudioRepairs(candidate.Alignment, v.Runs)
		if len(runs) > 0 {
			_, oldConflict := dualaudio.FindVisualAudioConflicts(&visual, candidate.Alignment.Segments, .15)
			_, newConflict := dualaudio.FindVisualAudioConflicts(&visual, proposed.Segments, .15)
			proposedAudio := dualaudio.Render(proposed, dubPCM, basePCM, dualaudio.GapFillBase)
			newValidation := dualaudio.ValidateStereo(proposedAudio, baseFeat, proposed.Segments, dualaudio.DefaultOptions().MinConfidence, 5)
			// Keep a repair only when the same set of validation windows improves
			// without worsening the existing aggregate evidence. This comparison
			// is exploratory, not an independent holdout or a sync verdict.
			if dualaudio.ShouldKeepVisualAudioRepair(v, newValidation, oldConflict, newConflict) {
				candidate.Alignment, rendered, v, repairs = proposed, proposedAudio, newValidation, runs
			}
		}
	}
	var source dualaudio.SourceValidation
	if *stereo {
		source = dualaudio.ValidateDubSourceStereo(rendered, dubFeat, candidate.Alignment.Segments, dualaudio.DefaultOptions().MinConfidence)
	} else {
		source = dualaudio.ValidateDubSource(rendered, dubFeat, candidate.Alignment.Segments, dualaudio.DefaultOptions().MinConfidence)
	}
	conflicts, conflictSeconds := dualaudio.FindVisualAudioConflicts(&visual, candidate.Alignment.Segments, .15)
	seamJumps := dualaudio.FindVisualAudioSeamJumps(candidate.Alignment.Segments, .04)
	var probes []dualaudio.WaveformEvidence
	type skippedWaveformProbe struct {
		BaseStart float64 `json:"base_start"`
		Reason    string  `json:"reason"`
	}
	var skippedProbes []skippedWaveformProbe
	for _, at := range probeTimes {
		probe, err := dualaudio.ProbeVisualWaveform(&visual, baseMono, dubMono, at)
		if err != nil {
			if *waveformEvery > 0 {
				skippedProbes = append(skippedProbes, skippedWaveformProbe{BaseStart: at, Reason: err.Error()})
				continue
			}
			return fmt.Errorf("waveform probe at %.3f: %w", at, err)
		}
		probes = append(probes, probe)
	}
	return json.NewEncoder(os.Stdout).Encode(struct {
		Status                      string                                 `json:"status"`
		Candidate                   *dualaudio.VisualAudioCandidate        `json:"candidate"`
		Hits                        int                                    `json:"hits"`
		Eligible                    int                                    `json:"eligible"`
		Median                      float64                                `json:"median"`
		P95                         float64                                `json:"p95"`
		OffSec                      float64                                `json:"off_sec"`
		Source                      dualaudio.SourceValidation             `json:"source_validation"`
		Stereo                      bool                                   `json:"stereo"`
		Holdout                     bool                                   `json:"holdout"`
		Misses                      []dualaudio.Miss                       `json:"misses,omitempty"`
		Runs                        []dualaudio.OffRun                     `json:"off_runs,omitempty"`
		Repairs                     []dualaudio.OffRun                     `json:"audio_repairs,omitempty"`
		VisualConflictSeconds       float64                                `json:"visual_conflict_seconds"`
		VisualConflicts             []dualaudio.VisualAudioConflict        `json:"visual_conflicts,omitempty"`
		WaveformProbes              []dualaudio.WaveformEvidence           `json:"waveform_probes,omitempty"`
		WaveformInterval            float64                                `json:"waveform_interval,omitempty"`
		WaveformSkipped             []skippedWaveformProbe                 `json:"waveform_skipped,omitempty"`
		SeamJumps                   []dualaudio.VisualAudioSeamJump        `json:"seam_jumps,omitempty"`
		WaveformVisualDisagreements []dualaudio.WaveformVisualDisagreement `json:"waveform_visual_disagreements,omitempty"`
	}{"review", candidate, v.Hits, v.Eligible, v.Median, v.P95, v.OffSec, source, *stereo, *holdout, v.Misses, v.Runs, repairs, conflictSeconds, conflicts, probes, *waveformEvery, skippedProbes, seamJumps, dualaudio.FindWaveformVisualDisagreements(probes)})
}

// mappingFile is what `match` writes and `build` reads. It is plain JSON on
// purpose: a pair the matcher refused can be filled in by hand.
type mappingFile struct {
	BaseDir string           `json:"base_dir"`
	DubDir  string           `json:"dub_dir"`
	Pairs   []dualaudio.Pair `json:"pairs"`
}

func listMedia(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() && dualaudioExts[strings.ToLower(filepath.Ext(e.Name()))] {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out, nil
}

// fingerprintDir decodes every file once. Decoding dominates the run, so it is
// the part spread over workers.
func fingerprintDir(ctx context.Context, t dualaudio.Tools, dir string, names []string, workers int) map[string]*dualaudio.Features {
	out := make(map[string]*dualaudio.Features, len(names))
	var mu sync.Mutex
	var wg sync.WaitGroup
	next := make(chan string)
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for name := range next {
				pcm, err := t.DecodePCM(ctx, filepath.Join(dir, name), 16000, 1, 0)
				if err != nil {
					fmt.Fprintf(os.Stderr, "  !! %s: %v\n", name, err)
					continue
				}
				fp := dualaudio.Fingerprint(dualaudio.ExtractFeatures(pcm))
				mu.Lock()
				out[name] = fp
				mu.Unlock()
			}
		}()
	}
	for _, n := range names {
		next <- n
	}
	close(next)
	wg.Wait()
	return out
}

func dualaudioMatch(ctx context.Context, t dualaudio.Tools, args []string) error {
	fs := flag.NewFlagSet("dualaudio match", flag.ContinueOnError)
	baseDir := fs.String("base-dir", "", "directory with the video source")
	dubDir := fs.String("dub-dir", "", "directory with the dubbed source")
	out := fs.String("out", "mapping.json", "where to write the pairing")
	minMargin := fs.Float64("min-margin", 1.6, "required ratio between best and second-best candidate")
	jobs := fs.Int("jobs", 2, "parallel workers")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *baseDir == "" || *dubDir == "" {
		return errors.New("--base-dir and --dub-dir are required")
	}
	baseNames, err := listMedia(*baseDir)
	if err != nil {
		return err
	}
	dubNames, err := listMedia(*dubDir)
	if err != nil {
		return err
	}
	fmt.Printf("==> fingerprinting %d base + %d dub file(s)\n", len(baseNames), len(dubNames))
	bases := fingerprintDir(ctx, t, *baseDir, baseNames, *jobs)
	dubs := fingerprintDir(ctx, t, *dubDir, dubNames, *jobs)
	if ctx.Err() != nil {
		return ctx.Err()
	}

	pairs := dualaudio.MatchAll(bases, dubs, *minMargin, *jobs)
	paired := 0
	for _, p := range pairs {
		if p.Dub == "" {
			fmt.Printf("  NO PAIR  %s  [%s]\n", p.Base, p.Note)
			continue
		}
		paired++
		note := ""
		if p.Note != "" {
			note = "  [" + p.Note + "]"
		}
		fmt.Printf("  %s\n      -> %s  (conf %.0f, margin %.1fx)%s\n", p.Base, p.Dub, p.Confidence, p.Margin, note)
	}
	fmt.Printf("\n==> paired %d of %d\n", paired, len(pairs))

	data, err := json.MarshalIndent(mappingFile{*baseDir, *dubDir, pairs}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(*out, data, 0o644)
}

func jobDefaults(gapFill string, dubLang, dubTitle string) dualaudio.Job {
	return dualaudio.Job{
		Options: dualaudio.DefaultOptions(),
		Gate:    dualaudio.DefaultGate(),
		GapFill: dualaudio.GapFill(gapFill),
		Mux: dualaudio.MuxOptions{
			BaseLang: "eng", BaseTitle: "English",
			DubLang: dubLang, DubTitle: dubTitle,
			Bitrate: "192k", DubIsDefault: true,
		},
	}
}

func dualaudioAnalyze(ctx context.Context, t dualaudio.Tools, args []string) error {
	fs := flag.NewFlagSet("dualaudio analyze", flag.ContinueOnError)
	base := fs.String("base", "", "file that provides the video")
	dub := fs.String("dub", "", "file that provides the dubbed audio")
	dubStream := fs.Int("dub-stream", 0, "audio stream index inside --dub")
	// Without this the CLI cannot reproduce a job's verdict: the app defaults to
	// a 1 s tick and analyze to 5 s, and the number of validation windows the
	// grade is computed over changes with it.
	tick := fs.Float64("tick", 0, "seconds between sync checks (0 = default)")
	visual := fs.Bool("visual-assist", false, "propose audio-confirmed offsets from video")
	jsonOutput := fs.Bool("json", false, "output the full analysis as JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *base == "" || *dub == "" {
		return errors.New("--base and --dub are required")
	}
	j := jobDefaults("base", "por", "Português (BR)")
	j.BasePath, j.DubPath, j.DubStream = *base, *dub, *dubStream
	j.TickSec = *tick
	j.VisualAssist = *visual
	res, err := dualaudio.Process(ctx, t, j)
	if err != nil {
		return err
	}
	if *jsonOutput {
		return json.NewEncoder(os.Stdout).Encode(res)
	}
	printResult(filepath.Base(*base), res)
	return nil
}

func printResult(name string, r *dualaudio.Result) {
	fmt.Printf("%-7s %s\n", r.Status, name)
	fmt.Printf("        visual candidates: %d\n", r.VisualCandidates)
	if al := r.Alignment; al != nil {
		fmt.Printf("        coverage %.1f%%, windows %d/%d, validated %d/%d, off %.0f s, residual %.0f ms (p95 %.0f ms)\n",
			al.Coverage*100, al.WindowsLocked, al.WindowsTotal, r.Validated, r.ValidatedOf, r.OffSec,
			r.ResidualMedian*1000, r.ResidualP95*1000)
		for _, s := range al.Segments {
			fmt.Printf("        %8.1f -> %8.1f s  lag %+8.2f s  (n=%d, conf %.0f)\n",
				s.BaseStart, s.BaseEnd, s.Lag, s.Windows, s.Confidence)
		}
		for _, g := range al.Gaps {
			fmt.Printf("        %8.1f -> %8.1f s  base only\n", g.Start, g.End)
		}
	}
	if len(r.Misses) > 0 && r.Status != dualaudio.StatusOK {
		fmt.Print("        out of place at:")
		for _, m := range r.Misses {
			fmt.Printf(" %.0fs(%+.2f|c%.0f)", m.BaseTime, m.Lag, m.Confidence)
		}
		fmt.Println()
	}
	for _, n := range r.Notes {
		fmt.Printf("        note: %s\n", n)
	}
}

func dualaudioBuild(ctx context.Context, t dualaudio.Tools, args []string) error {
	fs := flag.NewFlagSet("dualaudio build", flag.ContinueOnError)
	mapping := fs.String("mapping", "mapping.json", "pairing written by `dualaudio match`")
	outDir := fs.String("out-dir", "", "where to write the .dual.mkv files")
	jobs := fs.Int("jobs", 2, "parallel episodes")
	force := fs.Bool("force", false, "write episodes graded review too")
	gapFill := fs.String("gap-fill", "base", "what plays where the dub has no counterpart: base|silence")
	dubLang := fs.String("dub-lang", "por", "ISO 639-2 language of the dub")
	dubTitle := fs.String("dub-title", "Português (BR)", "track title of the dub")
	skip := fs.Bool("skip-existing", false, "leave finished episodes alone")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *outDir == "" {
		return errors.New("--out-dir is required")
	}
	raw, err := os.ReadFile(*mapping)
	if err != nil {
		return err
	}
	var mf mappingFile
	if err := json.Unmarshal(raw, &mf); err != nil {
		return fmt.Errorf("%s: %w", *mapping, err)
	}
	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		return err
	}

	var todo []dualaudio.Pair
	for _, p := range mf.Pairs {
		if p.Dub != "" {
			todo = append(todo, p)
		}
	}
	if len(todo) == 0 {
		return fmt.Errorf("no usable pair in %s — run `dualaudio match` again", *mapping)
	}
	fmt.Printf("==> %d episode(s), %d in parallel\n", len(todo), *jobs)

	results := make([]*dualaudio.Result, len(todo))
	var wg sync.WaitGroup
	var mu sync.Mutex
	next := make(chan int)
	for w := 0; w < *jobs; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range next {
				p := todo[i]
				j := jobDefaults(*gapFill, *dubLang, *dubTitle)
				j.BasePath = filepath.Join(mf.BaseDir, p.Base)
				j.DubPath = filepath.Join(mf.DubDir, p.Dub)
				j.OutPath = filepath.Join(*outDir, strings.TrimSuffix(p.Base, filepath.Ext(p.Base))+".dual.mkv")
				j.Force = *force
				if *skip {
					if _, err := os.Stat(j.OutPath); err == nil {
						continue
					}
				}
				res, err := dualaudio.Process(ctx, t, j)
				if err != nil {
					// One bad episode must not sink the batch.
					res.Status = dualaudio.StatusFail
					res.Notes = append(res.Notes, err.Error())
				}
				results[i] = res
				mu.Lock()
				printResult(p.Base, res)
				mu.Unlock()
			}
		}()
	}
	for i := range todo {
		if ctx.Err() != nil {
			break
		}
		next <- i
	}
	close(next)
	wg.Wait()

	counts := map[dualaudio.Status]int{}
	for _, r := range results {
		if r != nil {
			counts[r.Status]++
		}
	}
	fmt.Printf("\n==> ok=%d review=%d fail=%d\n", counts[dualaudio.StatusOK],
		counts[dualaudio.StatusReview], counts[dualaudio.StatusFail])
	return mergeReport(filepath.Join(*outDir, "report.json"), results)
}

// mergeReport folds this run into the existing report, keyed by base file. The
// report is the accumulated state of the output folder, not a log of the last
// run: rebuilding two episodes must not erase the other eighty.
func mergeReport(path string, results []*dualaudio.Result) error {
	byBase := map[string]*dualaudio.Result{}
	if raw, err := os.ReadFile(path); err == nil {
		var prev []*dualaudio.Result
		if json.Unmarshal(raw, &prev) == nil {
			for _, r := range prev {
				byBase[r.Base] = r
			}
		}
	}
	for _, r := range results {
		if r != nil {
			byBase[r.Base] = r
		}
	}
	merged := make([]*dualaudio.Result, 0, len(byBase))
	for _, r := range byBase {
		merged = append(merged, r)
	}
	sort.Slice(merged, func(i, j int) bool { return merged[i].Base < merged[j].Base })
	data, err := json.MarshalIndent(merged, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// dualaudioVisual reports a coarse visual correspondence without changing media.
func dualaudioVisual(ctx context.Context, tools dualaudio.Tools, args []string) error {
	fs := flag.NewFlagSet("dualaudio visual", flag.ContinueOnError)
	a := fs.String("base", "", "first video")
	b := fs.String("dub", "", "second video")
	at := fs.Float64("time", -1, "timestamp in the first video, in seconds")
	radius := fs.Float64("radius", 30, "search radius in seconds (5–240)")
	timeline := fs.Bool("timeline", false, "map distinctive scenes throughout both videos at 12 fps")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *a == "" || *b == "" {
		return errors.New("--base and --dub are required")
	}
	if *timeline {
		result, err := dualaudio.LocateVisualTimeline(ctx, tools, *a, *b)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(result)
	}
	result, err := dualaudio.LocateVisual(ctx, tools, *a, *b, *at, *radius)
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(result)
}
