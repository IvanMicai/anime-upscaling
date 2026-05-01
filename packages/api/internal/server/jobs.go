package server

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"anime-upscaling/internal/config"
	"anime-upscaling/internal/files"
	"anime-upscaling/internal/logger"
	"anime-upscaling/internal/pipeline"
	"anime-upscaling/internal/process"
	"anime-upscaling/internal/queue"
	"anime-upscaling/internal/runner"
)

var (
	ErrJobNotFound        = errors.New("job not found")
	ErrJobShutdownTimeout = errors.New("job did not stop in time")
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
	Scale             int                 `json:"scale"`
	Resolution        int                 `json:"resolution"`
	FrameRate         int                 `json:"frame_rate"`
	FrameRateMode     string              `json:"frame_rate_mode,omitempty"`
	FrameRateAbsolute float64             `json:"frame_rate_absolute,omitempty"`
	Multiplier        int                 `json:"multiplier,omitempty"`
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
	done          chan struct{}
	mu            sync.Mutex
}

// StartJobParams holds all parameters for creating and starting a job.
type StartJobParams struct {
	Type              string
	Files             []string
	Source            string
	SourceDir         string
	Scale             int
	Resolution        int
	FrameRate         int
	FrameRateMode     string
	FrameRateAbsolute float64
	Multiplier        int
	Threads           int
	RifeModel         string
	SceneThresh       float64
	Processor         string
	Model             string
	NoiseLevel        int
	Quality           string
	Codec             string
	Preset            string
	Tune              string
	PixFmt            string
	AudioCodec        string
	UseGPU            bool
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
	j.mu.Unlock()
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
	done := j.done
	j.mu.Unlock()
	if done != nil {
		select {
		case <-done:
		default:
			close(done)
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
		ID:                j.ID,
		Type:              j.Type,
		Status:            j.Status,
		Source:            j.Source,
		Scale:             j.Scale,
		Resolution:        j.Resolution,
		FrameRate:         j.FrameRate,
		FrameRateMode:     j.FrameRateMode,
		FrameRateAbsolute: j.FrameRateAbsolute,
		Multiplier:        j.Multiplier,
		RifeModel:         j.RifeModel,
		SceneThresh:       j.SceneThresh,
		Threads:           j.Threads,
		Processor:         j.Processor,
		Model:             j.Model,
		NoiseLevel:        j.NoiseLevel,
		Quality:           j.Quality,
		Codec:             j.Codec,
		Preset:            j.Preset,
		Tune:              j.Tune,
		PixFmt:            j.PixFmt,
		AudioCodec:        j.AudioCodec,
		UseGPU:            j.UseGPU,
		PipelineName:      j.PipelineName,
		PipelineSteps:     j.PipelineSteps,
		Files:             j.Files,
		Progress:          prog,
		CreatedAt:         j.CreatedAt,
		FinishedAt:        j.FinishedAt,
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
		ID:                j.ID,
		Type:              j.Type,
		Status:            j.Status,
		Source:            j.Source,
		Scale:             j.Scale,
		Resolution:        j.Resolution,
		FrameRate:         j.FrameRate,
		FrameRateMode:     j.FrameRateMode,
		FrameRateAbsolute: j.FrameRateAbsolute,
		Multiplier:        j.Multiplier,
		RifeModel:         j.RifeModel,
		SceneThresh:       j.SceneThresh,
		Threads:           j.Threads,
		Processor:         j.Processor,
		Model:             j.Model,
		NoiseLevel:        j.NoiseLevel,
		Quality:           j.Quality,
		Codec:             j.Codec,
		Preset:            j.Preset,
		Tune:              j.Tune,
		PixFmt:            j.PixFmt,
		AudioCodec:        j.AudioCodec,
		UseGPU:            j.UseGPU,
		PipelineName:      j.PipelineName,
		PipelineSteps:     j.PipelineSteps,
		Files:             j.Files,
		Progress:          prog,
		Logs:              logs,
		CreatedAt:         j.CreatedAt,
		FinishedAt:        j.FinishedAt,
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

// skipOutputDir returns the output directory whose presence indicates a file
// can be skipped for the given job type. Returns "" for jobs that don't have
// an output dir (e.g. "check") or that don't support pre-pass skip (e.g.
// "custom_pipeline" — which has per-step skip handled inline).
func skipOutputDir(jobType string, cfg config.Config) string {
	switch jobType {
	case "upscale":
		return cfg.OutputDir
	case "optimize":
		return cfg.OptimizedDir
	case "interpolate":
		return cfg.InterpolatedDir
	}
	return ""
}

func (m *JobManager) StartJob(p StartJobParams) *Job {
	sort.Strings(p.Files)
	ctx, cancel := context.WithCancel(context.Background())
	jobID := m.generateID()
	ctx = runner.WithJobID(ctx, jobID)

	job := &Job{
		ID:                jobID,
		Type:              p.Type,
		Status:            "queued",
		Source:            p.Source,
		Scale:             p.Scale,
		Resolution:        p.Resolution,
		FrameRate:         p.FrameRate,
		FrameRateMode:     p.FrameRateMode,
		FrameRateAbsolute: p.FrameRateAbsolute,
		Multiplier:        p.Multiplier,
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
		done:        make(chan struct{}),
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

		// Pré-pass: identifica skips upfront, antes de lançar qualquer worker.
		// Skipped files entram em Progress.Skipped via addLog imediatamente, e
		// ficam fora da lista de dispatch para não gerar goroutines no-op nem
		// SKIP duplicado pelas checagens inline em upscale/optimize/interpolate.
		toProcess := p.Files
		if outDir := skipOutputDir(p.Type, cfg); outDir != "" {
			toProcess = nil
			for i, f := range p.Files {
				if files.FileExists(filepath.Join(outDir, f)) {
					onEvent(logger.JobLog{
						Source:  "PIPELINE",
						Level:   "SKIP",
						Index:   i + 1,
						Message: "Pulando " + f + " (já existe)",
						Time:    time.Now(),
					})
					continue
				}
				toProcess = append(toProcess, f)
			}
		}

		switch p.Type {
		case "upscale":
			upOpts := runner.UpscaleOptions{
				Processor:  job.Processor,
				Model:      job.Model,
				NoiseLevel: job.NoiseLevel,
			}
			for i, f := range toProcess {
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
			jobFrameRate := job.FrameRate
			jobFrameRateAbsolute := 0.0
			if job.FrameRateMode == "absolute" && job.FrameRateAbsolute > 0 {
				jobFrameRateAbsolute = job.FrameRateAbsolute
			}
			jobThreads := job.Threads
			useGPU := job.UseGPU && cfg.GPUVendor != "" && job.Codec != "copy" && job.Codec != "libvpx-vp9"
			for i, f := range toProcess {
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
						process.OptimizeFile(ctx, cfg, r, filename, idx, jobSource, src, jobResolution, jobFrameRate, jobFrameRateAbsolute, crf, jobThreads, stepOpts, onEvent, onProgress)
					}()
				} else {
					if err := m.ffmpegQ.Submit(ctx, func(slot int) {
						defer wg.Done()
						job.setRunningOnce()
						ffSrc := runner.FFmpegSource(slot, cfg.FFmpegStreams)
						process.OptimizeFile(ctx, cfg, r, filename, idx, jobSource, ffSrc, jobResolution, jobFrameRate, jobFrameRateAbsolute, crf, jobThreads, encOpts, onEvent, onProgress)
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
			for i, f := range toProcess {
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
	jobID := m.generateID()
	ctx = runner.WithJobID(ctx, jobID)

	source := "input"
	if s := dirToSourceName(m.cfg, sourceDir); s != "" {
		source = s
	}

	job := &Job{
		ID:            jobID,
		Type:          "custom_pipeline",
		Status:        "queued",
		Source:        source,
		PipelineName:  pipelineName,
		PipelineSteps: steps,
		Files:         files,
		Progress:      JobProgress{Total: len(files) * len(steps)},
		CreatedAt:     time.Now().UTC(),
		cancel:        cancel,
		done:          make(chan struct{}),
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

// DeleteJob cancels the job (if running/queued), waits for it to terminate,
// then removes it from the manager. Returns ErrJobNotFound if the id is unknown
// or ErrJobShutdownTimeout if the job did not stop within the timeout.
func (m *JobManager) DeleteJob(id string) error {
	m.mu.RLock()
	job := m.jobs[id]
	m.mu.RUnlock()
	if job == nil {
		return ErrJobNotFound
	}

	job.mu.Lock()
	active := job.Status == "running" || job.Status == "queued"
	done := job.done
	if active {
		job.cancel()
	}
	job.mu.Unlock()

	if active {
		go func() { runner.StopByJobID(job.ID) }()
	}

	if done != nil {
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			return ErrJobShutdownTimeout
		}
	}

	m.mu.Lock()
	delete(m.jobs, id)
	m.mu.Unlock()
	return nil
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
		go func() { runner.StopByJobID(job.ID) }()
	}
	job.mu.Unlock()

	return job
}
