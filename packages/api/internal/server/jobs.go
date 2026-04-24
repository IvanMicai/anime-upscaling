package server

import (
	"context"
	"fmt"
	"math/rand"
	"sort"
	"sync"
	"time"

	"anime-upscaling/internal/config"
	"anime-upscaling/internal/logger"
	"anime-upscaling/internal/pipeline"
	"anime-upscaling/internal/process"
	"anime-upscaling/internal/queue"
	"anime-upscaling/internal/runner"
)

// logEntry is a type alias for logger.JobLog used within the server package.
type logEntry = logger.JobLog

type JobProgress struct {
	Total      int                         `json:"total"`
	Completed  int                         `json:"completed"`
	Failed     int                         `json:"failed"`
	Skipped    int                         `json:"skipped"`
	Current    string                      `json:"current"`
	Containers map[string]*runner.Progress `json:"containers,omitempty"`
}

type Job struct {
	ID            string                  `json:"id"`
	Type          string                  `json:"type"`
	Status        string                  `json:"status"`
	Source        string                  `json:"source"`
	Scale         int                     `json:"scale"`
	Resolution    int                     `json:"resolution"`
	Multiplier    int                     `json:"multiplier,omitempty"`
	RifeModel     string                  `json:"rife_model,omitempty"`
	SceneThresh   float64                 `json:"scene_thresh,omitempty"`
	Threads       int                     `json:"threads,omitempty"`
	Processor     string                  `json:"processor,omitempty"`
	Model         string                  `json:"model,omitempty"`
	NoiseLevel    int                     `json:"noise_level,omitempty"`
	Quality       string                  `json:"quality,omitempty"`
	Codec         string                  `json:"codec,omitempty"`
	Preset        string                  `json:"preset,omitempty"`
	Tune          string                  `json:"tune,omitempty"`
	PixFmt        string                  `json:"pix_fmt,omitempty"`
	AudioCodec    string                  `json:"audio_codec,omitempty"`
	UseGPU        bool                    `json:"use_gpu,omitempty"`
	PipelineName  string                  `json:"pipeline_name,omitempty"`
	PipelineSteps []pipeline.PipelineStep `json:"pipeline_steps,omitempty"`
	Files         []string                `json:"files"`
	Progress      JobProgress             `json:"progress"`
	Logs          []logEntry              `json:"-"`
	CreatedAt     time.Time               `json:"created_at"`
	FinishedAt    *time.Time              `json:"finished_at,omitempty"`
	cancel        context.CancelFunc
	mu            sync.Mutex
	listeners     []chan logEntry
}

// StartJobParams holds all parameters for creating and starting a job.
type StartJobParams struct {
	Type        string
	Files       []string
	Source      string
	SourceDir   string
	Scale       int
	Resolution  int
	Multiplier  int
	Threads     int
	RifeModel   string
	SceneThresh float64
	Processor   string
	Model       string
	NoiseLevel  int
	Quality     string
	Codec       string
	Preset      string
	Tune        string
	PixFmt      string
	AudioCodec  string
	UseGPU      bool
}

func (j *Job) updateContainerProgress(p runner.Progress) {
	j.mu.Lock()
	if j.Progress.Containers == nil {
		j.Progress.Containers = make(map[string]*runner.Progress)
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
	case "STEP":
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

func (j *Job) finish(ctx context.Context) {
	j.mu.Lock()
	now := time.Now().UTC()
	j.FinishedAt = &now
	if ctx.Err() != nil {
		j.Status = "cancelled"
	} else if j.Progress.Failed > 0 {
		j.Status = "failed"
	} else {
		j.Status = "completed"
	}
	for _, ch := range j.listeners {
		close(ch)
	}
	j.listeners = nil
	j.mu.Unlock()
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

func copyContainers(m map[string]*runner.Progress) map[string]*runner.Progress {
	if m == nil {
		return nil
	}
	cp := make(map[string]*runner.Progress, len(m))
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
		ID:            j.ID,
		Type:          j.Type,
		Status:        j.Status,
		Source:        j.Source,
		Scale:         j.Scale,
		Resolution:    j.Resolution,
		Multiplier:    j.Multiplier,
		RifeModel:     j.RifeModel,
		SceneThresh:   j.SceneThresh,
		Threads:       j.Threads,
		Processor:     j.Processor,
		Model:         j.Model,
		NoiseLevel:    j.NoiseLevel,
		Quality:       j.Quality,
		Codec:         j.Codec,
		Preset:        j.Preset,
		Tune:          j.Tune,
		PixFmt:        j.PixFmt,
		AudioCodec:    j.AudioCodec,
		UseGPU:        j.UseGPU,
		PipelineName:  j.PipelineName,
		PipelineSteps: j.PipelineSteps,
		Files:         j.Files,
		Progress:      prog,
		CreatedAt:     j.CreatedAt,
		FinishedAt:    j.FinishedAt,
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
		ID:            j.ID,
		Type:          j.Type,
		Status:        j.Status,
		Source:        j.Source,
		Scale:         j.Scale,
		Resolution:    j.Resolution,
		Multiplier:    j.Multiplier,
		RifeModel:     j.RifeModel,
		SceneThresh:   j.SceneThresh,
		Threads:       j.Threads,
		Processor:     j.Processor,
		Model:         j.Model,
		NoiseLevel:    j.NoiseLevel,
		Quality:       j.Quality,
		Codec:         j.Codec,
		Preset:        j.Preset,
		Tune:          j.Tune,
		PixFmt:        j.PixFmt,
		AudioCodec:    j.AudioCodec,
		UseGPU:        j.UseGPU,
		PipelineName:  j.PipelineName,
		PipelineSteps: j.PipelineSteps,
		Files:         j.Files,
		Progress:      prog,
		Logs:          logs,
		CreatedAt:     j.CreatedAt,
		FinishedAt:    j.FinishedAt,
	}
}

type JobManager struct {
	mu      sync.RWMutex
	jobs    map[string]*Job
	cfg     config.Config
	runner  *runner.Runner
	gpuQ    *queue.GPUQueue
	ffmpegQ *queue.Queue
}

func NewJobManager(cfg config.Config) *JobManager {
	return &JobManager{
		jobs:    make(map[string]*Job),
		cfg:     cfg,
		runner:  runner.NewRunner(cfg),
		gpuQ:    queue.NewGPUQueue(cfg.GPUCount, cfg.StreamsPerGPU),
		ffmpegQ: queue.New(cfg.FFmpegStreams),
	}
}

// HasActiveJobs returns true if any job is currently queued or running.
func (m *JobManager) HasActiveJobs() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, j := range m.jobs {
		j.mu.Lock()
		active := j.Status == "queued" || j.Status == "running"
		j.mu.Unlock()
		if active {
			return true
		}
	}
	return false
}

// ApplySettings reconstructs the GPU and FFmpeg queues with new concurrency settings.
// Only safe when there are no active jobs — callers must check HasActiveJobs first
// to avoid discarding queues that still hold acquired slots.
func (m *JobManager) ApplySettings(streamsPerGPU, ffmpegStreams int, gpuVendor string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cfg.StreamsPerGPU = streamsPerGPU
	m.cfg.FFmpegStreams = ffmpegStreams
	m.cfg.GPUVendor = gpuVendor
	m.gpuQ = queue.NewGPUQueue(m.cfg.GPUCount, streamsPerGPU)
	m.ffmpegQ = queue.New(ffmpegStreams)
}

// Config returns a copy of the current configuration (including runtime-mutable fields).
func (m *JobManager) Config() config.Config {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.cfg
}

func (m *JobManager) generateID() string {
	return fmt.Sprintf("j_%d_%04x", time.Now().Unix(), rand.Intn(0xFFFF))
}

func (m *JobManager) StartJob(p StartJobParams) *Job {
	sort.Strings(p.Files)
	ctx, cancel := context.WithCancel(context.Background())

	job := &Job{
		ID:          m.generateID(),
		Type:        p.Type,
		Status:      "queued",
		Source:      p.Source,
		Scale:       p.Scale,
		Resolution:  p.Resolution,
		Multiplier:  p.Multiplier,
		RifeModel:   p.RifeModel,
		SceneThresh: p.SceneThresh,
		Threads:     p.Threads,
		Processor:   p.Processor,
		Model:       p.Model,
		NoiseLevel:  p.NoiseLevel,
		Quality:     p.Quality,
		Codec:       p.Codec,
		Preset:      p.Preset,
		Tune:        p.Tune,
		PixFmt:      p.PixFmt,
		AudioCodec:  p.AudioCodec,
		UseGPU:      p.UseGPU,
		Files:       p.Files,
		Progress:    JobProgress{Total: len(p.Files)},
		CreatedAt:   time.Now().UTC(),
		cancel:      cancel,
	}

	m.mu.Lock()
	m.jobs[job.ID] = job
	m.mu.Unlock()

	r := m.runner
	cfg := m.cfg
	onEvent := func(e logEntry) {
		job.addLog(e)
	}
	onProgress := func(p runner.Progress) {
		job.updateContainerProgress(p)
	}

	go func() {
		var wg sync.WaitGroup

		switch p.Type {
		case "upscale":
			upOpts := runner.UpscaleOptions{
				Processor:  job.Processor,
				Model:      job.Model,
				NoiseLevel: job.NoiseLevel,
			}
			for i, f := range p.Files {
				wg.Add(1)
				idx := i + 1
				filename := f
				gpuID, streamIdx, err := m.gpuQ.Acquire(ctx, 0)
				if err != nil {
					wg.Done()
					break // ctx cancelled
				}
				go func() {
					defer wg.Done()
					defer m.gpuQ.Release(gpuID, streamIdx)
					job.setRunningOnce()
					process.UpscaleFile(ctx, cfg, r, gpuID, streamIdx, filename, idx, job.Scale, upOpts, p.SourceDir, cfg.OutputDir, onEvent, onProgress)
				}()
			}

		case "optimize":
			crf := pipeline.QualityToCRF[job.Quality]
			if crf == 0 {
				crf = 19
			}
			encOpts := runner.EncodeOptions{
				Codec:      job.Codec,
				Preset:     job.Preset,
				Tune:       job.Tune,
				PixFmt:     job.PixFmt,
				AudioCodec: job.AudioCodec,
				GPUVendor:  cfg.GPUVendor,
			}
			jobSource := p.Source
			jobResolution := job.Resolution
			jobThreads := job.Threads
			useGPU := job.UseGPU && cfg.GPUVendor != "" && job.Codec != "copy" && job.Codec != "libvpx-vp9"
			for i, f := range p.Files {
				wg.Add(1)
				idx := i + 1
				filename := f
				if useGPU {
					gpuID, streamIdx, err := m.gpuQ.Acquire(ctx, 0)
					if err != nil {
						wg.Done()
						break // ctx cancelled
					}
					stepOpts := encOpts
					stepOpts.UseGPU = true
					stepOpts.GPUDevice = gpuID
					src := runner.GPUSource(gpuID, streamIdx, cfg.StreamsPerGPU)
					go func() {
						defer wg.Done()
						defer m.gpuQ.Release(gpuID, streamIdx)
						job.setRunningOnce()
						process.OptimizeFile(ctx, cfg, r, filename, idx, jobSource, src, jobResolution, crf, jobThreads, stepOpts, onEvent, onProgress)
					}()
				} else {
					if err := m.ffmpegQ.Submit(ctx, func(slot int) {
						defer wg.Done()
						job.setRunningOnce()
						ffSrc := runner.FFmpegSource(slot, cfg.FFmpegStreams)
						process.OptimizeFile(ctx, cfg, r, filename, idx, jobSource, ffSrc, jobResolution, crf, jobThreads, encOpts, onEvent, onProgress)
					}); err != nil {
						wg.Done()
						break // ctx cancelled
					}
				}
			}

		case "check":
			jobSource := p.Source
			for i, f := range p.Files {
				wg.Add(1)
				idx := i + 1
				filename := f
				if err := m.ffmpegQ.Submit(ctx, func(slot int) {
					defer wg.Done()
					job.setRunningOnce()
					ffSrc := runner.FFmpegSource(slot, cfg.FFmpegStreams)
					process.CheckFile(ctx, cfg, r, filename, idx, jobSource, ffSrc, onEvent, onProgress)
				}); err != nil {
					wg.Done()
					break // ctx cancelled
				}
			}

		case "interpolate":
			rifeOpts := runner.RifeOptions{
				Model:       job.RifeModel,
				SceneThresh: job.SceneThresh,
			}
			for i, f := range p.Files {
				wg.Add(1)
				idx := i + 1
				filename := f
				gpuID, streamIdx, err := m.gpuQ.Acquire(ctx, 0)
				if err != nil {
					wg.Done()
					break // ctx cancelled
				}
				go func() {
					defer wg.Done()
					defer m.gpuQ.Release(gpuID, streamIdx)
					job.setRunningOnce()
					process.InterpolateFile(ctx, cfg, r, gpuID, streamIdx, filename, idx, job.Multiplier, rifeOpts, p.SourceDir, cfg.InterpolatedDir, onEvent, onProgress)
				}()
			}
		}

		wg.Wait()

		job.finish(ctx)
	}()

	return job
}

func (m *JobManager) StartPipelineJob(pipelineName string, steps []pipeline.PipelineStep, files []string, sourceDir string) *Job {
	sort.Strings(files)
	ctx, cancel := context.WithCancel(context.Background())

	source := "input"
	if s := dirToSourceName(m.cfg, sourceDir); s != "" {
		source = s
	}

	job := &Job{
		ID:            m.generateID(),
		Type:          "custom_pipeline",
		Status:        "queued",
		Source:        source,
		PipelineName:  pipelineName,
		PipelineSteps: steps,
		Files:         files,
		Progress:      JobProgress{Total: len(files)},
		CreatedAt:     time.Now().UTC(),
		cancel:        cancel,
	}

	m.mu.Lock()
	m.jobs[job.ID] = job
	m.mu.Unlock()

	r := m.runner
	cfg := m.cfg
	gpuQ := m.gpuQ
	ffmpegQ := m.ffmpegQ
	onEvent := func(e logEntry) {
		job.addLog(e)
	}
	onProgress := func(p runner.Progress) {
		job.updateContainerProgress(p)
	}

	go func() {
		var wg sync.WaitGroup

		for i, f := range files {
			wg.Add(1)
			idx := i + 1
			filename := f
			go func() {
				defer wg.Done()
				job.setRunningOnce()
				process.RunCustomPipelineForFile(ctx, cfg, r, gpuQ, ffmpegQ, steps, filename, idx, sourceDir, onEvent, onProgress)
			}()
		}

		wg.Wait()

		job.finish(ctx)
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
		// Stop video2x processes for upscale/pipeline/interpolate jobs
		if job.Type == "upscale" || job.Type == "pipeline" || job.Type == "interpolate" || job.Type == "custom_pipeline" {
			go func() { runner.StopByPrefix(runner.ProcessPrefix + "video2x-") }()
		}
		// Stop ffmpeg processes for optimize/pipeline/check/custom_pipeline jobs
		if job.Type == "optimize" || job.Type == "pipeline" || job.Type == "check" || job.Type == "custom_pipeline" {
			go func() { runner.StopByPrefix(runner.ProcessPrefix + "ffmpeg-") }()
		}
	}
	job.mu.Unlock()

	return job
}
