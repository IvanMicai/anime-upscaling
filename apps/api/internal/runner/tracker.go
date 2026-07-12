package runner

import (
	"os/exec"
	"sync"
	"syscall"
)

// ProcessTracker keeps track of running long-lived processes so they can be
// stopped on cancel. It is safe for concurrent use.
var tracker = &processTracker{
	procs: make(map[string]*trackedCmd),
}

type trackedCmd struct {
	cmd   *exec.Cmd
	jobID string
}

type processTracker struct {
	mu    sync.Mutex
	procs map[string]*trackedCmd
}

func (t *processTracker) register(label string, cmd *exec.Cmd) {
	t.registerForJob(label, cmd, "")
}

func (t *processTracker) registerForJob(label string, cmd *exec.Cmd, jobID string) {
	t.mu.Lock()
	t.procs[label] = &trackedCmd{cmd: cmd, jobID: jobID}
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
	for label, tc := range tracker.procs {
		if len(label) >= len(prefix) && label[:len(prefix)] == prefix {
			toKill = append(toKill, tc.cmd)
		}
	}
	tracker.mu.Unlock()

	return signalAll(toKill)
}

// StopByJobID sends SIGTERM to all tracked processes registered for the given
// jobID. Used by the API server's CancelJob so cancelling one job does not kill
// processes belonging to other concurrently-running jobs.
func StopByJobID(jobID string) int {
	if jobID == "" {
		return 0
	}
	tracker.mu.Lock()
	var toKill []*exec.Cmd
	for _, tc := range tracker.procs {
		if tc.jobID == jobID {
			toKill = append(toKill, tc.cmd)
		}
	}
	tracker.mu.Unlock()

	return signalAll(toKill)
}

func signalAll(cmds []*exec.Cmd) int {
	killed := 0
	for _, cmd := range cmds {
		if cmd.Process != nil {
			if err := cmd.Process.Signal(syscall.SIGTERM); err == nil {
				killed++
			}
		}
	}
	return killed
}
