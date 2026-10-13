package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"anime-upscaling/internal/config"
	"anime-upscaling/internal/dualaudio"
)

var dualaudioExts = map[string]bool{".mkv": true, ".mp4": true, ".m4v": true}

func cmdDualAudio(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("usage: animeup dualaudio <match|build|analyze> [flags]")
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
	}
	return fmt.Errorf("unknown dualaudio command: %s", args[0])
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
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *base == "" || *dub == "" {
		return errors.New("--base and --dub are required")
	}
	j := jobDefaults("base", "por", "Português (BR)")
	j.BasePath, j.DubPath, j.DubStream = *base, *dub, *dubStream
	j.TickSec = *tick
	res, err := dualaudio.Process(ctx, t, j)
	if err != nil {
		return err
	}
	printResult(filepath.Base(*base), res)
	return nil
}

func printResult(name string, r *dualaudio.Result) {
	fmt.Printf("%-7s %s\n", r.Status, name)
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
