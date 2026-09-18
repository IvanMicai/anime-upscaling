package cache

import "testing"

func TestSlotCoversEveryStage(t *testing.T) {
	var s FileStatus
	for _, label := range []string{"input", "output", "optimize", "interpolated", "merged"} {
		slot := s.Slot(label)
		if slot == nil {
			t.Fatalf("no slot for %q", label)
		}
		*slot = &SourceEntry{Size: int64(len(label))}
	}
	if s.Input.Size != 5 || s.Output.Size != 6 || s.Optimize.Size != 8 || s.Interpolated.Size != 12 || s.Merged.Size != 6 {
		t.Errorf("slots alias the wrong fields: %+v", s)
	}
	if s.Slot("optimized") != nil {
		t.Error(`"optimized" is the folder name, not the cache label; it must not resolve`)
	}
}
