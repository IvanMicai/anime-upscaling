package server

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"anime-upscaling/internal/config"
	"anime-upscaling/internal/docker"
	"anime-upscaling/internal/logger"
	"anime-upscaling/internal/process"
)

// logEntry is a type alias for logger.JobLog used within the server package.
type logEntry = logger.JobLog

type JobProgress struct {
	Total     int              `json:"total"`
	Completed int              `json:"completed"`
	Failed    int              `json:"failed"`
	Skipped   int              `json:"skipped"`
	Current   string           `json:"current"`
	Container *docker.Progress `json:"container,omitempty"`
}

type Job struct {
	ID         string       `json:"id"`
	Type       string       `json:"type"`
	Status     string       `json:"status"`
	Files      []string     `json:"files"`
	Progress   JobProgress  `json:"progress"`
	Logs       []logEntry   `json:"-"`
	CreatedAt  time.Time    `json:"created_at"`
	FinishedAt *time.Time   `json:"finished_at,omitempty"`
	cancel     context.CancelFunc
	mu         sync.Mutex
	listeners  []chan logEntry
}

func (j *Job) updateContainerProgress(p docker.Progress) {
	j.mu.Lock()
	j.Progress.Container = &p
	j.mu.Unlock()
}

func (j *Job) addLog(e logEntry) {
	j.mu.Lock()
	j.Logs = append(j.Logs, e)
	// Update progress based on log level
	switch e.Level {
	case "OK":
		j.Progress.Completed++
		j.Progress.Current = ""
		j.Progress.Container = nil
	case "ERRO":
		j.Progress.Failed++
		j.Progress.Current = ""
		j.Progress.Container = nil
	case "SKIP":
		j.Progress.Skipped++
		j.Progress.Current = ""
		j.Progress.Container = nil
	case "INFO":
		j.Progress.Current = e.Message
	}
	listeners := make([]chan logEntry, len(j.listeners))
	copy(listeners, j.listeners)
	j.mu.Unlock()

	// Notify SSE listeners (non-blocking)
	for _, ch := range listeners {
		select {
		case ch <- e:
		default:
		}
	}
}

// subscribe returns a channel for real-time log events and whether the job is still running.
// If the job already finished, the channel is returned closed.
func (j *Job) subscribe() (chan logEntry, bool) {
	j.mu.Lock()
	defer j.mu.Unlock()
	ch := make(chan logEntry, 64)
	if j.Status != "running" {
		close(ch)
		return ch, false
	}
	j.listeners = append(j.listeners, ch)
	return ch, true
}

func (j *Job) unsubscribe(ch chan logEntry) {
	j.mu.Lock()
	defer j.mu.Unlock()
	for i, l := range j.listeners {
		if l == ch {
			j.listeners = append(j.listeners[:i], j.listeners[i+1:]...)
			// Only close if we removed it (wasn't already closed by job finish)
			close(ch)
			return
		}
	}
	// Not found means job already finished and closed all channels
}

func (j *Job) snapshot() Job {
	j.mu.Lock()
	defer j.mu.Unlock()
	return Job{
		ID:         j.ID,
		Type:       j.Type,
		Status:     j.Status,
		Files:      j.Files,
		Progress:   j.Progress,
		CreatedAt:  j.CreatedAt,
		FinishedAt: j.FinishedAt,
	}
}

func (j *Job) snapshotWithLogs() Job {
	j.mu.Lock()
	defer j.mu.Unlock()
	logs := make([]logEntry, len(j.Logs))
	copy(logs, j.Logs)
	return Job{
		ID:         j.ID,
		Type:       j.Type,
		Status:     j.Status,
		Files:      j.Files,
		Progress:   j.Progress,
		Logs:       logs,
		CreatedAt:  j.CreatedAt,
		FinishedAt: j.FinishedAt,
	}
}

type JobManager struct {
	mu   sync.RWMutex
	jobs map[string]*Job
	cfg  config.Config
}

func NewJobManager(cfg config.Config) *JobManager {
	return &JobManager{
		jobs: make(map[string]*Job),
		cfg:  cfg,
	}
}

func (m *JobManager) generateID() string {
	return fmt.Sprintf("j_%d_%04x", time.Now().Unix(), rand.Intn(0xFFFF))
}

func (m *JobManager) StartJob(jobType string, files []string) *Job {
	ctx, cancel := context.WithCancel(context.Background())

	job := &Job{
		ID:        m.generateID(),
		Type:      jobType,
		Status:    "running",
		Files:     files,
		Progress:  JobProgress{Total: len(files)},
		CreatedAt: time.Now().UTC(),
		cancel:    cancel,
	}

	m.mu.Lock()
	m.jobs[job.ID] = job
	m.mu.Unlock()

	d := docker.NewDocker(m.cfg)
	onEvent := func(e logEntry) {
		job.addLog(e)
	}
	onProgress := func(p docker.Progress) {
		job.updateContainerProgress(p)
	}

	go func() {
		var err error
		switch jobType {
		case "upscale":
			err = process.RunUpscale(ctx, m.cfg, d, files, onEvent, onProgress)
		case "optimize":
			err = process.RunOptimize(ctx, m.cfg, d, files, onEvent, onProgress)
		case "pipeline":
			err = process.RunPipeline(ctx, m.cfg, d, files, onEvent, onProgress)
		}

		job.mu.Lock()
		now := time.Now().UTC()
		job.FinishedAt = &now
		if ctx.Err() != nil {
			job.Status = "cancelled"
		} else if err != nil {
			job.Status = "failed"
		} else {
			job.Status = "completed"
		}
		// Close all listener channels to signal end
		for _, ch := range job.listeners {
			close(ch)
		}
		job.listeners = nil
		job.mu.Unlock()
	}()

	return job
}

func (m *JobManager) GetJob(id string) *Job {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.jobs[id]
}

func (m *JobManager) ListJobs() []Job {
	m.mu.RLock()
	defer m.mu.RUnlock()
	list := make([]Job, 0, len(m.jobs))
	for _, j := range m.jobs {
		list = append(list, j.snapshot())
	}
	return list
}

func (m *JobManager) CancelJob(id string) *Job {
	m.mu.RLock()
	job := m.jobs[id]
	m.mu.RUnlock()
	if job == nil {
		return nil
	}

	job.mu.Lock()
	if job.Status == "running" {
		job.cancel()
	}
	job.mu.Unlock()

	return job
}
