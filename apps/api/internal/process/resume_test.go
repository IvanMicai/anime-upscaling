package process

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"anime-upscaling/internal/config"
	"anime-upscaling/internal/pipeline"
)

// realPipeline mirrors the production "Upscale, Interpolation and Optimization"
// pipeline: every producing step is followed by a cleanup that deletes the file
// from the stage it just left. That cleanup is what strands files when a run is
// cancelled — they are gone from the source folder but not yet finished.
func realPipeline() []pipeline.PipelineStep {
	return []pipeline.PipelineStep{
		{Operation: "upscale", Scale: 4},                                 // 0 -> output/
		{Operation: "cleanup", CleanupFolders: []string{"input"}},        // 1
		{Operation: "interpolate", Multiplier: 2},                        // 2 -> interpolated/
		{Operation: "cleanup", CleanupFolders: []string{"output"}},       // 3
		{Operation: "optimize", Quality: "baixa", Codec: "libx265"},      // 4 -> optimized/
		{Operation: "cleanup", CleanupFolders: []string{"interpolated"}}, // 5
	}
}

func testCfg(t *testing.T) config.Config {
	t.Helper()
	base := t.TempDir()
	cfg := config.Config{
		BaseDir:         base,
		InputDir:        filepath.Join(base, "input"),
		OutputDir:       filepath.Join(base, "output"),
		OptimizedDir:    filepath.Join(base, "optimized"),
		InterpolatedDir: filepath.Join(base, "interpolated"),
		VideoExts:       []string{".mkv", ".mp4", ".avi"},
	}
	for _, d := range []string{cfg.InputDir, cfg.OutputDir, cfg.OptimizedDir, cfg.InterpolatedDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return cfg
}

func write(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestPlanPipelineFiles_AdoptsStrandedFiles reproduces the scheduling bug this
// planner exists to fix. A cancelled run left two episodes in output/ (upscaled,
// awaiting interpolate) and three in interpolated/ (awaiting optimize); cleanup
// had already deleted all five from input/. Re-running the pipeline from input/
// used to enumerate only what was still there, so those five were dropped from
// the job entirely — the GPUs picked up fresh input files and the idle FFmpeg
// pool had nothing to do, because every enumerated file was still on step 0.
func TestPlanPipelineFiles_AdoptsStrandedFiles(t *testing.T) {
	cfg := testCfg(t)
	steps := realPipeline()

	write(t, cfg.InputDir, "GT S01E58.mkv")
	write(t, cfg.InputDir, "GT S01E59.mkv")
	write(t, cfg.OutputDir, "GT S01E55.mkv")
	write(t, cfg.OutputDir, "GT S01E57.mkv")
	write(t, cfg.InterpolatedDir, "Digimon S01E39.mkv")
	write(t, cfg.InterpolatedDir, "Digimon S01E40.mkv")
	write(t, cfg.InterpolatedDir, "Digimon S01E41.mkv")

	sourceFiles := []string{"GT S01E58.mkv", "GT S01E59.mkv"}
	plans := PlanPipelineFiles(cfg, steps, cfg.InputDir, sourceFiles)

	want := map[string]FilePlan{
		// Still in the source folder -> whole pipeline, as before.
		"GT S01E58.mkv": {StartStep: 0, InputDir: cfg.InputDir},
		"GT S01E59.mkv": {StartStep: 0, InputDir: cfg.InputDir},
		// Upscaled already: resume at the cleanup after upscale, read output/.
		"GT S01E55.mkv": {StartStep: 1, InputDir: cfg.OutputDir},
		"GT S01E57.mkv": {StartStep: 1, InputDir: cfg.OutputDir},
		// Interpolated already: resume after interpolate, read interpolated/.
		"Digimon S01E39.mkv": {StartStep: 3, InputDir: cfg.InterpolatedDir},
		"Digimon S01E40.mkv": {StartStep: 3, InputDir: cfg.InterpolatedDir},
		"Digimon S01E41.mkv": {StartStep: 3, InputDir: cfg.InterpolatedDir},
	}

	if len(plans) != len(want) {
		t.Fatalf("planned %d files, want %d: %+v", len(plans), len(want), plans)
	}
	for _, got := range plans {
		exp, ok := want[got.Name]
		if !ok {
			t.Errorf("unexpected file in plan: %q", got.Name)
			continue
		}
		if got.StartStep != exp.StartStep || got.InputDir != exp.InputDir {
			t.Errorf("%s: start=%d dir=%s, want start=%d dir=%s",
				got.Name, got.StartStep, got.InputDir, exp.StartStep, exp.InputDir)
		}
	}
}

// TestPlanPipelineFiles_RoutesStrandedWorkToBothPools is the property that
// actually unblocks the reported symptom: the stranded files must reach both
// worker pools, so the idle FFmpeg slots get work instead of waiting hours for
// a fresh input file to finish upscale+interpolate.
func TestPlanPipelineFiles_RoutesStrandedWorkToBothPools(t *testing.T) {
	cfg := testCfg(t)
	steps := realPipeline()

	write(t, cfg.InputDir, "GT S01E58.mkv")
	write(t, cfg.OutputDir, "GT S01E55.mkv")
	write(t, cfg.InterpolatedDir, "Digimon S01E39.mkv")

	plans := PlanPipelineFiles(cfg, steps, cfg.InputDir, []string{"GT S01E58.mkv"})

	got := map[string]QueueKind{}
	for _, p := range plans {
		got[p.Name] = FirstQueue(cfg, steps, p.StartStep)
	}
	want := map[string]QueueKind{
		"GT S01E58.mkv":      QueueGPU,    // upscale
		"GT S01E55.mkv":      QueueGPU,    // interpolate
		"Digimon S01E39.mkv": QueueFFmpeg, // optimize, CPU encode
	}
	for name, exp := range want {
		if got[name] != exp {
			t.Errorf("%s: first queue = %v, want %v", name, got[name], exp)
		}
	}
}

// TestPlanPipelineFiles_LeavesFinishedFilesAlone guards against the planner
// re-listing the whole optimized/ library on every run. A file that reached the
// last producing step is done; at most a trailing cleanup did not run.
func TestPlanPipelineFiles_LeavesFinishedFilesAlone(t *testing.T) {
	cfg := testCfg(t)
	steps := realPipeline()

	write(t, cfg.OptimizedDir, "Done S01E01.mkv")
	// Optimized but the trailing cleanup never ran, so a stale copy lingers in
	// interpolated/. The furthest stage still wins: this file is finished.
	write(t, cfg.OptimizedDir, "Done S01E02.mkv")
	write(t, cfg.InterpolatedDir, "Done S01E02.mkv")

	plans := PlanPipelineFiles(cfg, steps, cfg.InputDir, nil)
	if len(plans) != 0 {
		t.Fatalf("planned %+v, want no files adopted", plans)
	}
}

// TestPlanPipelineFiles_SkipsSanitizeArtifacts covers the leftovers a killed run
// leaves in a stage folder. The hard link shares an inode with the real file, so
// adopting it would process the same episode twice and emit a mangled name.
func TestPlanPipelineFiles_SkipsSanitizeArtifacts(t *testing.T) {
	cfg := testCfg(t)
	steps := realPipeline()

	real := "Dragon Ball GT S01E55 [pt-BR] [480p].mkv"
	write(t, cfg.OutputDir, real)
	artifact := "Dragon_Ball_GT_S01E55_[pt-BR]_[480p]_dk8x3ne7r3oe.mkv"
	if err := os.Link(filepath.Join(cfg.OutputDir, real), filepath.Join(cfg.OutputDir, artifact)); err != nil {
		t.Fatal(err)
	}

	plans := PlanPipelineFiles(cfg, steps, cfg.InputDir, nil)
	if len(plans) != 1 {
		t.Fatalf("planned %d files, want 1: %+v", len(plans), plans)
	}
	if plans[0].Name != real {
		t.Errorf("adopted %q, want the un-sanitized name %q", plans[0].Name, real)
	}
}

// TestPlanPipelineFiles_SourceFilesAlwaysStartAtZero pins the deliberately
// conservative half of the policy: presence in the source folder means "run the
// whole pipeline", even when a later stage already holds a copy. That copy may
// be a partial file from the interrupted run, and re-running a step is cheap
// next to shipping a truncated episode.
func TestPlanPipelineFiles_SourceFilesAlwaysStartAtZero(t *testing.T) {
	cfg := testCfg(t)
	steps := realPipeline()

	const name = "GT S01E55.mkv"
	write(t, cfg.InputDir, name)
	write(t, cfg.OutputDir, name) // possibly partial output from the killed run

	plans := PlanPipelineFiles(cfg, steps, cfg.InputDir, []string{name})
	if len(plans) != 1 {
		t.Fatalf("planned %d files, want 1: %+v", len(plans), plans)
	}
	if plans[0].StartStep != 0 || plans[0].InputDir != cfg.InputDir {
		t.Errorf("start=%d dir=%s, want start=0 dir=%s",
			plans[0].StartStep, plans[0].InputDir, cfg.InputDir)
	}
}

// TestGroupForAdmission_ResumedFilesGetFreeSlotsFirst pins the ordering half of
// the fix, replayed at production scale. A pool with free slots hands them out
// on Acquire's fast path, which ignores priority — so whoever the relay admits
// first simply wins. Admitting in plan order would hand both GPUs to fresh
// step-0 input files while the two episodes already waiting on interpolate sat
// behind all 176 of them, which is the ordering the priority comparator exists
// to prevent.
func TestGroupForAdmission_ResumedFilesGetFreeSlotsFirst(t *testing.T) {
	cfg := testCfg(t)
	steps := realPipeline()

	// 176 fresh input files, as in the reported job.
	var sourceFiles []string
	for i := 58; i < 58+176; i++ {
		name := fmt.Sprintf("Dragon Ball GT S01E%03d.mkv", i)
		write(t, cfg.InputDir, name)
		sourceFiles = append(sourceFiles, name)
	}
	// Stranded mid-pipeline by the cancelled run.
	write(t, cfg.OutputDir, "Dragon Ball GT S01E55.mkv")       // awaiting interpolate
	write(t, cfg.OutputDir, "Dragon Ball GT S01E57.mkv")       // awaiting interpolate
	write(t, cfg.InterpolatedDir, "Digimon Tamers S01E39.mkv") // awaiting optimize
	write(t, cfg.InterpolatedDir, "Digimon Tamers S01E40.mkv")
	write(t, cfg.InterpolatedDir, "Digimon Tamers S01E41.mkv")

	plans := PlanPipelineFiles(cfg, steps, cfg.InputDir, sourceFiles)
	groups := GroupForAdmission(cfg, steps, plans)

	head := func(kind QueueKind, n int) []string {
		var out []string
		for i, planIdx := range groups[kind] {
			if i >= n {
				break
			}
			out = append(out, plans[planIdx].Name)
		}
		return out
	}

	// Two GPU slots, three FFmpeg slots — the deployed configuration.
	wantGPU := []string{"Dragon Ball GT S01E55.mkv", "Dragon Ball GT S01E57.mkv"}
	wantFFmpeg := []string{
		"Digimon Tamers S01E39.mkv",
		"Digimon Tamers S01E40.mkv",
		"Digimon Tamers S01E41.mkv",
	}
	if got := head(QueueGPU, 2); !reflect.DeepEqual(got, wantGPU) {
		t.Errorf("GPU slots take %v, want %v", got, wantGPU)
	}
	if got := head(QueueFFmpeg, 3); !reflect.DeepEqual(got, wantFFmpeg) {
		t.Errorf("FFmpeg slots take %v, want %v", got, wantFFmpeg)
	}
}

func TestFilePlan_RemainingSteps(t *testing.T) {
	steps := realPipeline() // 6 steps
	for _, tc := range []struct {
		start, want int
	}{{0, 6}, {1, 5}, {3, 3}, {6, 0}, {9, 0}} {
		if got := (FilePlan{StartStep: tc.start}).RemainingSteps(steps); got != tc.want {
			t.Errorf("StartStep=%d: remaining=%d, want %d", tc.start, got, tc.want)
		}
	}
}

func TestFirstQueue(t *testing.T) {
	cfg := testCfg(t)
	cfg.GPUVendor = "nvidia"
	steps := realPipeline()

	if got := FirstQueue(cfg, steps, 0); got != QueueGPU {
		t.Errorf("step 0: got %v, want QueueGPU", got)
	}
	if got := FirstQueue(cfg, steps, 4); got != QueueFFmpeg {
		t.Errorf("step 4 (CPU encode): got %v, want QueueFFmpeg", got)
	}
	// Only a cleanup left: the file never contends for a pool.
	if got := FirstQueue(cfg, steps, 5); got != QueueNone {
		t.Errorf("step 5: got %v, want QueueNone", got)
	}

	// A GPU-accelerated optimize contends for the GPU pool instead.
	gpuEncode := []pipeline.PipelineStep{{Operation: "optimize", Codec: "libx265", UseGPU: true}}
	if got := FirstQueue(cfg, gpuEncode, 0); got != QueueGPU {
		t.Errorf("gpu optimize: got %v, want QueueGPU", got)
	}
	// ...but not when the codec cannot be GPU-encoded, or no GPU is configured.
	noGPU := cfg
	noGPU.GPUVendor = ""
	if got := FirstQueue(noGPU, gpuEncode, 0); got != QueueFFmpeg {
		t.Errorf("gpu optimize without vendor: got %v, want QueueFFmpeg", got)
	}
	vp9 := []pipeline.PipelineStep{{Operation: "optimize", Codec: "libvpx-vp9", UseGPU: true}}
	if got := FirstQueue(cfg, vp9, 0); got != QueueFFmpeg {
		t.Errorf("vp9 optimize: got %v, want QueueFFmpeg", got)
	}
}
