package server

import (
	"context"
	"testing"
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
