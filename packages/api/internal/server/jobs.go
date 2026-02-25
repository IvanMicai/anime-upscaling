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
	"anime-upscaling/internal/queue"
)

// logEntry is a type alias for logger.JobLog used within the server package.
type logEntry = logger.JobLog

type JobProgress struct {
	Total      int                         `json:"total"`
	Completed  int                         `json:"completed"`
	Failed     int                         `json:"failed"`
	Skipped    int                         `json:"skipped"`
	Current    string                      `json:"current"`
	Containers map[string]*docker.Progress `json:"containers,omitempty"`
}

type Job struct {
	ID         string       `json:"id"`
	Type       string       `json:"type"`
	Status     string       `json:"status"`
	Source     string       `json:"source"`
	Scale      int          `json:"scale"`
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
	if j.Progress.Containers == nil {
		j.Progress.Containers = make(map[string]*docker.Progress)
	}
	j.Progress.Containers[p.Source] = &p
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
		delete(j.Progress.Containers, e.Source)
	case "ERRO":
		j.Progress.Failed++
		j.Progress.Current = ""
		delete(j.Progress.Containers, e.Source)
	case "SKIP":
		j.Progress.Skipped++
		j.Progress.Current = ""
		delete(j.Progress.Containers, e.Source)
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

// setRunningOnce transitions the job from "queued" to "running" exactly once.
func (j *Job) setRunningOnce() {
	j.mu.Lock()
	if j.Status == "queued" {
		j.Status = "running"
	}
	j.mu.Unlock()
}

// subscribe returns a channel for real-time log events and whether the job is still active.
// If the job already finished, the channel is returned closed.
func (j *Job) subscribe() (chan logEntry, bool) {
	j.mu.Lock()
	defer j.mu.Unlock()
	ch := make(chan logEntry, 64)
	if j.Status != "running" && j.Status != "queued" {
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

func copyContainers(m map[string]*docker.Progress) map[string]*docker.Progress {
	if m == nil {
		return nil
	}
	cp := make(map[string]*docker.Progress, len(m))
	for k, v := range m {
		p := *v
		cp[k] = &p
	}
	return cp
}

func (j *Job) snapshot() Job {
	j.mu.Lock()
	defer j.mu.Unlock()
	prog := j.Progress
	prog.Containers = copyContainers(j.Progress.Containers)
	return Job{
		ID:         j.ID,
		Type:       j.Type,
		Status:     j.Status,
		Source:     j.Source,
		Scale:      j.Scale,
		Files:      j.Files,
		Progress:   prog,
		CreatedAt:  j.CreatedAt,
		FinishedAt: j.FinishedAt,
	}
}

func (j *Job) snapshotWithLogs() Job {
	j.mu.Lock()
	defer j.mu.Unlock()
	logs := make([]logEntry, len(j.Logs))
	copy(logs, j.Logs)
	prog := j.Progress
	prog.Containers = copyContainers(j.Progress.Containers)
	return Job{
		ID:         j.ID,
		Type:       j.Type,
		Status:     j.Status,
		Source:     j.Source,
		Scale:      j.Scale,
		Files:      j.Files,
		Progress:   prog,
		Logs:       logs,
		CreatedAt:  j.CreatedAt,
		FinishedAt: j.FinishedAt,
	}
}

type JobManager struct {
	mu      sync.RWMutex
	jobs    map[string]*Job
	cfg     config.Config
	docker  *docker.Docker
	gpuQ    *queue.GPUQueue
	ffmpegQ *queue.Queue
}

func NewJobManager(cfg config.Config) *JobManager {
	return &JobManager{
		jobs:    make(map[string]*Job),
		cfg:     cfg,
		docker:  docker.NewDocker(cfg),
		gpuQ:    queue.NewGPUQueue(2),
		ffmpegQ: queue.New(1),
	}
}

func (m *JobManager) generateID() string {
	return fmt.Sprintf("j_%d_%04x", time.Now().Unix(), rand.Intn(0xFFFF))
}

func (m *JobManager) StartJob(jobType string, files []string, source string, scale int) *Job {
	ctx, cancel := context.WithCancel(context.Background())

	job := &Job{
		ID:        m.generateID(),
		Type:      jobType,
		Status:    "queued",
		Source:    source,
		Scale:     scale,
		Files:     files,
		Progress:  JobProgress{Total: len(files)},
		CreatedAt: time.Now().UTC(),
		cancel:    cancel,
	}

	m.mu.Lock()
	m.jobs[job.ID] = job
	m.mu.Unlock()

	d := m.docker
	cfg := m.cfg
	onEvent := func(e logEntry) {
		job.addLog(e)
	}
	onProgress := func(p docker.Progress) {
		job.updateContainerProgress(p)
	}

	go func() {
		var wg sync.WaitGroup

		switch jobType {
		case "upscale":
			for i, f := range files {
				wg.Add(1)
				idx := i + 1
				filename := f
				gpuID, err := m.gpuQ.Acquire(ctx)
				if err != nil {
					wg.Done()
					break // ctx cancelled
				}
				go func() {
					defer wg.Done()
					defer m.gpuQ.Release(gpuID)
					job.setRunningOnce()
					process.UpscaleFile(ctx, cfg, d, gpuID, filename, idx, job.Scale, onEvent, onProgress)
				}()
			}

		case "optimize":
			jobSource := source
			for i, f := range files {
				wg.Add(1)
				idx := i + 1
				filename := f
				if err := m.ffmpegQ.Submit(ctx, func() {
					defer wg.Done()
					job.setRunningOnce()
					process.OptimizeFile(ctx, cfg, d, filename, idx, jobSource, onEvent, onProgress)
				}); err != nil {
					wg.Done()
					break // ctx cancelled
				}
			}

		case "check":
			jobSource := source
			for i, f := range files {
				wg.Add(1)
				idx := i + 1
				filename := f
				if err := m.ffmpegQ.Submit(ctx, func() {
					defer wg.Done()
					job.setRunningOnce()
					process.CheckFile(ctx, cfg, d, filename, idx, jobSource, onEvent, onProgress)
				}); err != nil {
					wg.Done()
					break // ctx cancelled
				}
			}

		case "pipeline":
			for i, f := range files {
				wg.Add(1)
				idx := i + 1
				filename := f
				gpuID, err := m.gpuQ.Acquire(ctx)
				if err != nil {
					wg.Done()
					break // ctx cancelled
				}
				go func() {
					defer m.gpuQ.Release(gpuID)
					job.setRunningOnce()
					ok := process.UpscaleFile(ctx, cfg, d, gpuID, filename, idx, job.Scale, onEvent, onProgress)
					if !ok || ctx.Err() != nil {
						wg.Done()
						return
					}
					// Phase 2: enqueue into FFmpeg queue
					if err := m.ffmpegQ.Submit(ctx, func() {
						defer wg.Done()
						process.EncodeFile(ctx, cfg, d, filename, onEvent, onProgress)
					}); err != nil {
						wg.Done() // ctx cancelled
					}
				}()
			}
		}

		wg.Wait()

		job.mu.Lock()
		now := time.Now().UTC()
		job.FinishedAt = &now
		if ctx.Err() != nil {
			job.Status = "cancelled"
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
	if job.Status == "running" || job.Status == "queued" {
		job.cancel()
		// Stop video2x containers for upscale/pipeline jobs
		if job.Type == "upscale" || job.Type == "pipeline" {
			go m.docker.StopByPrefix(context.Background(), docker.ContainerPrefix+"video2x-")
		}
		// Stop ffmpeg containers for optimize/pipeline/check jobs
		if job.Type == "optimize" || job.Type == "pipeline" || job.Type == "check" {
			go m.docker.StopByPrefix(context.Background(), docker.ContainerPrefix+"ffmpeg-")
		}
	}
	job.mu.Unlock()

	return job
}
