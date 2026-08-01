package server

import (
	"context"
	"reflect"
	"testing"

	"anime-upscaling/internal/process"
)

func TestJobFinishMarksFailedWhenAnyFileFailed(t *testing.T) {
	job := &Job{
		Status:   "running",
		Progress: JobProgress{Total: 2, Completed: 1, Failed: 1},
	}

	job.finish(context.Background())

	if job.Status != "failed" {
		t.Fatalf("expected failed status, got %q", job.Status)
	}
	if job.FinishedAt == nil {
		t.Fatal("expected finished_at to be set")
	}
}

// TestActiveFilesCoversOnlyLiveJobs pins what the overlap guard is allowed to
// block on: a file is claimed only while the job holding it is still queued or
// running. A finished job must not keep an episode locked out forever.
func TestActiveFilesCoversOnlyLiveJobs(t *testing.T) {
	m := &JobManager{jobs: map[string]*Job{
		"a": {Status: "running", Files: []string{"ep01.mkv", "ep02.mkv"}},
		"b": {Status: "queued", Files: []string{"ep03.mkv"}},
		"c": {Status: "completed", Files: []string{"ep04.mkv"}},
		"d": {Status: "failed", Files: []string{"ep05.mkv"}},
		"e": {Status: "cancelled", Files: []string{"ep06.mkv"}},
	}}

	got := m.ActiveFiles()
	want := map[string]bool{"ep01.mkv": true, "ep02.mkv": true, "ep03.mkv": true}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ActiveFiles() = %v, want %v", got, want)
	}
}

// TestOverlappingReportsPlanCollisions verifies the guard compares the plan —
// which includes files adopted mid-pipeline, the ones most likely to collide —
// and reports them in plan order.
func TestOverlappingReportsPlanCollisions(t *testing.T) {
	plans := []process.FilePlan{
		{Name: "Digimon Tamers S01E39.mkv"},
		{Name: "Dragon Ball GT S01E58.mkv"},
		{Name: "Dragon Ball GT S01E59.mkv"},
	}

	if got := overlapping(plans, nil); got != nil {
		t.Errorf("no active jobs: got %v, want nil", got)
	}
	if got := overlapping(plans, map[string]bool{"Other S01E01.mkv": true}); got != nil {
		t.Errorf("disjoint active job: got %v, want nil", got)
	}

	busy := map[string]bool{"Dragon Ball GT S01E59.mkv": true, "Digimon Tamers S01E39.mkv": true}
	want := []string{"Digimon Tamers S01E39.mkv", "Dragon Ball GT S01E59.mkv"}
	if got := overlapping(plans, busy); !reflect.DeepEqual(got, want) {
		t.Errorf("overlapping() = %v, want %v", got, want)
	}
}

func TestSummarizeNamesTruncatesLongLists(t *testing.T) {
	for _, tc := range []struct {
		in   []string
		want string
	}{
		{[]string{"a"}, "a"},
		{[]string{"a", "b", "c"}, "a, b, c"},
		{[]string{"a", "b", "c", "d", "e"}, "a, b, c and 2 more"},
	} {
		if got := summarizeNames(tc.in); got != tc.want {
			t.Errorf("summarizeNames(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestJobFinishCancelledWinsOverFailed(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	job := &Job{
		Status:   "running",
		Progress: JobProgress{Total: 1, Failed: 1},
	}

	job.finish(ctx)

	if job.Status != "cancelled" {
		t.Fatalf("expected cancelled status, got %q", job.Status)
	}
}
