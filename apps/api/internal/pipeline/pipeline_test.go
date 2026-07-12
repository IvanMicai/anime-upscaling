package pipeline

import (
	"path/filepath"
	"testing"
)

// TestListSortedByName verifies List() returns pipelines in stable natural
// order by name, regardless of insertion order. This is the fix for the UI
// list reshuffling on every request (Go map iteration order is randomized).
func TestListSortedByName(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "pipelines.json"))

	// Insert deliberately out of alphabetical order, including a case and a
	// numeric case that plain byte sort would get wrong ("Pipeline 10" vs "2").
	for _, name := range []string{"banana", "Apple", "Pipeline 10", "Pipeline 2", "cherry"} {
		if _, err := store.Create(name, []PipelineStep{{Operation: "upscale"}}); err != nil {
			t.Fatalf("create %q: %v", name, err)
		}
	}

	want := []string{"Apple", "banana", "cherry", "Pipeline 2", "Pipeline 10"}

	// Call List() several times: map iteration is randomized per call, so a
	// missing sort would surface as a different order on some iteration.
	for attempt := 0; attempt < 20; attempt++ {
		list := store.List()
		if len(list) != len(want) {
			t.Fatalf("attempt %d: got %d pipelines, want %d", attempt, len(list), len(want))
		}
		for i, p := range list {
			if p.Name != want[i] {
				t.Fatalf("attempt %d: position %d = %q, want %q (full order: %v)", attempt, i, p.Name, want[i], names(list))
			}
		}
	}
}

func names(list []Pipeline) []string {
	out := make([]string, len(list))
	for i, p := range list {
		out[i] = p.Name
	}
	return out
}
