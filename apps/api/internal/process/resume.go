package process

import (
	"sort"

	"anime-upscaling/internal/config"
	"anime-upscaling/internal/files"
	"anime-upscaling/internal/pipeline"
	"anime-upscaling/internal/runner"
)

// FilePlan describes where a single file enters a custom pipeline: the step it
// starts on and the directory that step reads from.
type FilePlan struct {
	Name      string
	StartStep int
	InputDir  string
}

// RemainingSteps is how many steps this file still has to run. It is the file's
// contribution to a job's progress total.
func (p FilePlan) RemainingSteps(steps []pipeline.PipelineStep) int {
	if p.StartStep >= len(steps) {
		return 0
	}
	return len(steps) - p.StartStep
}

// StageDir maps a canonical stage folder name to its absolute path, or "" if
// the name is not a known stage.
func StageDir(cfg config.Config, stage string) string {
	switch stage {
	case "input":
		return cfg.InputDir
	case "output":
		return cfg.OutputDir
	case "interpolated":
		return cfg.InterpolatedDir
	case "optimized":
		return cfg.OptimizedDir
	}
	return ""
}

// QueueKind identifies which worker pool a pipeline step contends for.
type QueueKind int

const (
	QueueNone QueueKind = iota
	QueueGPU
	QueueFFmpeg
)

// optimizeUsesGPU centralises the condition for routing an optimize step to the
// GPU pool, so the admission planner and the executor cannot drift apart about
// which queue a given file will block on.
func optimizeUsesGPU(cfg config.Config, step pipeline.PipelineStep) bool {
	return step.UseGPU && cfg.GPUVendor != "" && step.Codec != "copy" && step.Codec != "libvpx-vp9"
}

// firstAcquire finds the first step at or after startStep that contends for a
// worker pool, returning that pool and the step's index. Returns (QueueNone, -1)
// when the file never acquires one (only cleanups left).
func firstAcquire(cfg config.Config, steps []pipeline.PipelineStep, startStep int) (QueueKind, int) {
	for i := startStep; i < len(steps); i++ {
		switch steps[i].Operation {
		case "upscale", "interpolate":
			return QueueGPU, i
		case "optimize":
			if optimizeUsesGPU(cfg, steps[i]) {
				return QueueGPU, i
			}
			return QueueFFmpeg, i
		}
	}
	return QueueNone, -1
}

// FirstQueue reports which worker pool a file first blocks on when it resumes
// at startStep, or QueueNone when it never acquires one (only cleanups left).
func FirstQueue(cfg config.Config, steps []pipeline.PipelineStep, startStep int) QueueKind {
	kind, _ := firstAcquire(cfg, steps, startStep)
	return kind
}

// GroupForAdmission partitions plan indices by the worker pool each file first
// blocks on, ordering every pool by the same rule the queues use to pick among
// waiters: files further along the pipeline first, then natural order.
//
// The ordering matters because a pool with free slots hands them out on
// Acquire's fast path, which ignores priority entirely — whoever is admitted
// first simply wins. Admitting in plan order would therefore give the GPUs to
// fresh step-0 files while files resumed at a later step, which the priority
// comparator is meant to favour, waited behind all of them.
func GroupForAdmission(cfg config.Config, steps []pipeline.PipelineStep, plans []FilePlan) map[QueueKind][]int {
	groups := make(map[QueueKind][]int, 3)
	acquireStep := make([]int, len(plans))
	for i, p := range plans {
		kind, step := firstAcquire(cfg, steps, p.StartStep)
		acquireStep[i] = step
		groups[kind] = append(groups[kind], i)
	}
	// priority mirrors what the file will pass to Acquire on its first step, so
	// admission order and waiter order agree.
	for _, idxs := range groups {
		sort.SliceStable(idxs, func(a, b int) bool {
			ia, ib := idxs[a], idxs[b]
			return pipelinePriority(acquireStep[ia], ia+1) > pipelinePriority(acquireStep[ib], ib+1)
		})
	}
	return groups
}

// PlanPipelineFiles builds the execution plan for a custom-pipeline run.
//
// Files handed in via sourceFiles always start at step 0: their presence in the
// source folder is unambiguous intent to run the whole pipeline, and re-running
// a step is safe where trusting a possibly-partial intermediate file is not.
//
// On top of those it adopts orphans — files sitting in a stage folder the
// pipeline writes to that are absent from the source set. A pipeline whose
// cleanup steps delete each file from the earlier stages as it advances strands
// everything that was mid-flight when a run is cancelled: re-running from the
// source folder alone silently drops those files, because cleanup already
// removed them from it. An orphan resumes at the step after the furthest
// producing step whose output folder still holds it.
//
// An orphan is only adopted when real processing work remains. A file that has
// reached the last producing step has finished the pipeline — at most some
// trailing cleanup did not run — so it is left alone rather than re-listed on
// every subsequent run.
func PlanPipelineFiles(cfg config.Config, steps []pipeline.PipelineStep, sourceDir string, sourceFiles []string) []FilePlan {
	ordered := make([]string, len(sourceFiles))
	copy(ordered, sourceFiles)
	files.SortNatural(ordered)

	plans := make([]FilePlan, 0, len(ordered))
	inSource := make(map[string]bool, len(ordered))
	for _, f := range ordered {
		inSource[f] = true
		plans = append(plans, FilePlan{Name: f, StartStep: 0, InputDir: sourceDir})
	}

	// Producing steps in pipeline order, paired with the folder they write to.
	type producer struct {
		stepIdx int
		dir     string
	}
	var producers []producer
	for i, s := range steps {
		stage := pipeline.StepOutputStage(s.Operation)
		if stage == "" {
			continue
		}
		if dir := StageDir(cfg, stage); dir != "" {
			producers = append(producers, producer{stepIdx: i, dir: dir})
		}
	}
	if len(producers) == 0 {
		return plans
	}
	lastProducer := producers[len(producers)-1].stepIdx

	// Walk each producing stage folder once, recording per filename the
	// furthest producer that still holds a copy. Visiting producers in pipeline
	// order means a later stage simply overwrites an earlier one.
	furthest := make(map[string]producer)
	for _, p := range producers {
		found, err := files.WalkVideos(p.dir, cfg.VideoExts)
		if err != nil {
			continue
		}
		for rel := range found {
			if inSource[rel] || runner.IsSanitizeArtifact(rel) {
				continue
			}
			furthest[rel] = p
		}
	}

	orphans := make([]string, 0, len(furthest))
	for name, p := range furthest {
		if p.stepIdx >= lastProducer {
			continue // already through the pipeline
		}
		orphans = append(orphans, name)
	}
	files.SortNatural(orphans)

	for _, name := range orphans {
		p := furthest[name]
		plans = append(plans, FilePlan{Name: name, StartStep: p.stepIdx + 1, InputDir: p.dir})
	}
	return plans
}
