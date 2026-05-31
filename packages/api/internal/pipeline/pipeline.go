package pipeline

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"sync"
	"time"
)

// PipelineStep defines a single operation in a pipeline.
type PipelineStep struct {
	Operation         string  `json:"operation"`
	Scale             int     `json:"scale,omitempty"`
	Processor         string  `json:"processor,omitempty"`
	Model             string  `json:"model,omitempty"`
	NoiseLevel        int     `json:"noise_level,omitempty"`
	Multiplier        int     `json:"multiplier,omitempty"`
	RifeModel         string  `json:"rife_model,omitempty"`
	SceneThresh       float64 `json:"scene_thresh,omitempty"`
	Quality           string  `json:"quality,omitempty"`
	Resolution        int     `json:"resolution,omitempty"`
	FrameRate         int     `json:"frame_rate,omitempty"`
	FrameRateMode     string  `json:"frame_rate_mode,omitempty"`
	FrameRateAbsolute float64 `json:"frame_rate_absolute,omitempty"`
	Threads           int     `json:"threads,omitempty"`
	Codec             string  `json:"codec,omitempty"`
	Preset            string  `json:"preset,omitempty"`
	Tune              string  `json:"tune,omitempty"`
	PixFmt            string  `json:"pix_fmt,omitempty"`
	AudioCodec        string  `json:"audio_codec,omitempty"`
	UseGPU            bool    `json:"use_gpu,omitempty"`
}

// Pipeline is a named, ordered sequence of processing steps.
type Pipeline struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Steps     []PipelineStep `json:"steps"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
}

// QualityToCRF maps quality preset names to CRF values.
var QualityToCRF = map[string]int{
	"ultra": 16,
	"alta":  19,
	"media": 22,
	"baixa": 26,
}

// ValidProcessors lists allowed upscale processors.
var ValidProcessors = map[string]bool{
	"": true, "realesrgan": true, "libplacebo": true, "realcugan": true,
}

// ModelScaleCompat maps model/shader name to the set of valid scale factors.
var ModelScaleCompat = map[string]map[int]bool{
	// realesrgan
	"realesr-animevideov3":  {2: true, 3: true, 4: true},
	"realesrgan-plus-anime": {4: true},
	"realesrgan-plus":       {4: true},
	// libplacebo (shaders work with any scale)
	"anime4k-v4-a":     {2: true, 3: true, 4: true},
	"anime4k-v4-a+a":   {2: true, 3: true, 4: true},
	"anime4k-v4-b":     {2: true, 3: true, 4: true},
	"anime4k-v4-b+b":   {2: true, 3: true, 4: true},
	"anime4k-v4-c":     {2: true, 3: true, 4: true},
	"anime4k-v4-c+a":   {2: true, 3: true, 4: true},
	"anime4k-v4.1-gan": {2: true, 3: true, 4: true},
	// realcugan
	"models-se":   {2: true, 3: true, 4: true},
	"models-pro":  {2: true, 3: true},
	"models-nose": {2: true},
}

// ValidUpscaleModels lists all valid model/shader names across processors.
var ValidUpscaleModels = func() map[string]bool {
	m := map[string]bool{"": true}
	for k := range ModelScaleCompat {
		m[k] = true
	}
	return m
}()

// ValidModelScale checks whether a model+scale combination is supported.
func ValidModelScale(model string, scale int) bool {
	if model == "" {
		return true
	}
	scales, ok := ModelScaleCompat[model]
	if !ok {
		return false
	}
	return scales[scale]
}

// ValidRifeModels lists all valid RIFE model names.
var ValidRifeModels = map[string]bool{
	"":          true,
	"rife-v4.6": true, "rife-v4.26": true, "rife-v4.25": true, "rife-v4.25-lite": true,
	"rife-v4": true, "rife-v3.1": true, "rife-v3.0": true,
	"rife-v2.4": true, "rife-v2.3": true, "rife-v2": true,
	"rife-anime": true, "rife-UHD": true, "rife-HD": true, "rife": true,
}

// ValidCodecs lists allowed video codec values for optimize steps.
var ValidCodecs = map[string]bool{
	"": true, "libx265": true, "libx264": true, "libvpx-vp9": true, "copy": true,
}

// ValidPresets lists allowed encoding speed presets.
var ValidPresets = map[string]bool{
	"": true, "ultrafast": true, "superfast": true, "veryfast": true,
	"fast": true, "medium": true, "slow": true, "slower": true, "veryslow": true,
}

// ValidTunes lists allowed tune modes.
var ValidTunes = map[string]bool{
	"": true, "none": true, "animation": true, "film": true, "grain": true, "zerolatency": true,
}

// ValidPixFmts lists allowed pixel formats.
var ValidPixFmts = map[string]bool{
	"": true, "yuv420p10le": true, "yuv420p": true, "yuv444p": true,
}

// ValidAudioCodecs lists allowed audio codec values.
var ValidAudioCodecs = map[string]bool{
	"": true, "copy": true, "aac": true, "libopus": true, "libmp3lame": true,
}

// ValidOperations lists allowed step operation types.
var ValidOperations = map[string]bool{
	"upscale":     true,
	"interpolate": true,
	"optimize":    true,
}

// Store manages CRUD for pipeline definitions stored in a JSON file.
type Store struct {
	mu       sync.RWMutex
	filePath string
	data     map[string]Pipeline
}

// NewStore creates a Store that persists to the given file path.
func NewStore(filePath string) *Store {
	s := &Store{
		filePath: filePath,
		data:     make(map[string]Pipeline),
	}
	s.load()
	return s
}

func (s *Store) load() {
	f, err := os.Open(s.filePath)
	if err != nil {
		return
	}
	defer f.Close()

	var pipelines []Pipeline
	if err := json.NewDecoder(f).Decode(&pipelines); err != nil {
		return
	}
	for _, p := range pipelines {
		s.data[p.ID] = p
	}
}

func (s *Store) flush() error {
	pipelines := make([]Pipeline, 0, len(s.data))
	for _, p := range s.data {
		pipelines = append(pipelines, p)
	}

	data, err := json.MarshalIndent(pipelines, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal pipelines: %w", err)
	}
	return os.WriteFile(s.filePath, data, 0644)
}

func generateID() string {
	return fmt.Sprintf("p_%d_%04x", time.Now().Unix(), rand.Intn(0xFFFF))
}

// List returns all pipeline definitions.
func (s *Store) List() []Pipeline {
	s.mu.RLock()
	defer s.mu.RUnlock()

	list := make([]Pipeline, 0, len(s.data))
	for _, p := range s.data {
		list = append(list, p)
	}
	return list
}

// Get returns a single pipeline by ID, or nil if not found.
func (s *Store) Get(id string) *Pipeline {
	s.mu.RLock()
	defer s.mu.RUnlock()

	p, ok := s.data[id]
	if !ok {
		return nil
	}
	return &p
}

// Create adds a new pipeline and persists to disk.
func (s *Store) Create(name string, steps []PipelineStep) (Pipeline, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	p := Pipeline{
		ID:        generateID(),
		Name:      name,
		Steps:     steps,
		CreatedAt: now,
		UpdatedAt: now,
	}

	s.data[p.ID] = p
	if err := s.flush(); err != nil {
		delete(s.data, p.ID)
		return Pipeline{}, fmt.Errorf("save: %w", err)
	}
	return p, nil
}

// Update modifies an existing pipeline and persists to disk.
func (s *Store) Update(id string, name *string, steps []PipelineStep) (*Pipeline, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	p, ok := s.data[id]
	if !ok {
		return nil, nil
	}

	if name != nil {
		p.Name = *name
	}
	if steps != nil {
		p.Steps = steps
	}
	p.UpdatedAt = time.Now().UTC()

	s.data[id] = p
	if err := s.flush(); err != nil {
		return nil, fmt.Errorf("save: %w", err)
	}
	return &p, nil
}

// Delete removes a pipeline and persists to disk.
func (s *Store) Delete(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.data[id]; !ok {
		return false
	}

	old := s.data[id]
	delete(s.data, id)
	if err := s.flush(); err != nil {
		s.data[id] = old
		return false
	}
	return true
}
