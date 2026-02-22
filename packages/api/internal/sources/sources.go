package sources

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"sync"
	"time"
)

type Source struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Path string `json:"path"`
}

var mu sync.Mutex

func generateID() string {
	return fmt.Sprintf("src_%d_%04x", time.Now().UnixNano()/1e6, rand.Intn(0xFFFF))
}

func Load(filePath string) ([]Source, error) {
	mu.Lock()
	defer mu.Unlock()
	return load(filePath)
}

func load(filePath string) ([]Source, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return []Source{}, nil
		}
		return nil, fmt.Errorf("read sources: %w", err)
	}
	var sources []Source
	if err := json.Unmarshal(data, &sources); err != nil {
		return nil, fmt.Errorf("parse sources: %w", err)
	}
	return sources, nil
}

func save(filePath string, sources []Source) error {
	data, err := json.MarshalIndent(sources, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal sources: %w", err)
	}
	return os.WriteFile(filePath, data, 0644)
}

func Add(filePath, name, path string) (Source, error) {
	mu.Lock()
	defer mu.Unlock()

	sources, err := load(filePath)
	if err != nil {
		return Source{}, err
	}
	s := Source{
		ID:   generateID(),
		Name: name,
		Path: path,
	}
	sources = append(sources, s)
	if err := save(filePath, sources); err != nil {
		return Source{}, err
	}
	return s, nil
}

func Remove(filePath, id string) error {
	mu.Lock()
	defer mu.Unlock()

	sources, err := load(filePath)
	if err != nil {
		return err
	}
	filtered := make([]Source, 0, len(sources))
	found := false
	for _, s := range sources {
		if s.ID == id {
			found = true
			continue
		}
		filtered = append(filtered, s)
	}
	if !found {
		return fmt.Errorf("source not found: %s", id)
	}
	return save(filePath, filtered)
}

func Get(filePath, id string) (*Source, error) {
	mu.Lock()
	defer mu.Unlock()

	sources, err := load(filePath)
	if err != nil {
		return nil, err
	}
	for _, s := range sources {
		if s.ID == id {
			return &s, nil
		}
	}
	return nil, nil
}
