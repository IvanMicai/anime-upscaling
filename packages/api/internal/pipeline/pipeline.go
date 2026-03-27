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
	Operation   string  `json:"operation"`
	Scale       int     `json:"scale,omitempty"`
	Multiplier  int     `json:"multiplier,omitempty"`
	RifeModel   string  `json:"rife_model,omitempty"`
	SceneThresh float64 `json:"scene_thresh,omitempty"`
	Quality     string  `json:"quality,omitempty"`
	Resolution  int     `json:"resolution,omitempty"`
	Threads     int     `json:"threads,omitempty"`
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
