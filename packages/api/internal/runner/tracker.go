package runner

import (
	"os/exec"
	"sync"
	"syscall"
)

// ProcessTracker keeps track of running long-lived processes so they can be
// stopped on cancel. It is safe for concurrent use.
var tracker = &processTracker{
	procs: make(map[string]*exec.Cmd),
}

type processTracker struct {
	mu    sync.Mutex
	procs map[string]*exec.Cmd
}

func (t *processTracker) register(label string, cmd *exec.Cmd) {
	t.mu.Lock()
	t.procs[label] = cmd
	t.mu.Unlock()
}

func (t *processTracker) unregister(label string) {
	t.mu.Lock()
	delete(t.procs, label)
	t.mu.Unlock()
}

// StopByPrefix sends SIGTERM to all tracked processes whose label starts with prefix.
// Returns the number of processes signaled.
func StopByPrefix(prefix string) int {
	tracker.mu.Lock()
	var toKill []*exec.Cmd
	var labels []string
	for label, cmd := range tracker.procs {
		if len(label) >= len(prefix) && label[:len(prefix)] == prefix {
			toKill = append(toKill, cmd)
			labels = append(labels, label)
		}
	}
	tracker.mu.Unlock()

	killed := 0
	for _, cmd := range toKill {
		if cmd.Process != nil {
			if err := cmd.Process.Signal(syscall.SIGTERM); err == nil {
				killed++
			}
		}
	}
	return killed
}
