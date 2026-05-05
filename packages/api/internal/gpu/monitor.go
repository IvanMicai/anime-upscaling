// Package gpu provides a lightweight GPU health monitor used to gate work
// dispatch when the GPU driver is wedged (e.g. NVRM Xid 119 / GSP RPC timeouts).
//
// The monitor periodically runs a cheap probe (default: `nvidia-smi -L`) with
// a hard timeout. After N consecutive failures it flips to unhealthy and any
// caller blocked on WaitHealthy stays blocked until the probe succeeds again
// or the caller's context is cancelled.
//
// Host-side recovery (PCI remove+rescan, container bounce) lives outside this
// process — see scripts/gpu-watchdog.sh in the server-management repo. This
// monitor's job is only to stop feeding the wedged driver with new
// allocations, which previously piled up uninterruptible nvidia-container-cli
// processes and made manual recovery harder.
package gpu

import (
	"context"
	"fmt"
	"os/exec"
	"sync"
	"time"
)

// Status is a snapshot of the monitor's most recent observation.
type Status struct {
	Enabled             bool      `json:"enabled"`
	Healthy             bool      `json:"healthy"`
	LastCheck           time.Time `json:"last_check,omitempty"`
	LastHealthy         time.Time `json:"last_healthy,omitempty"`
	ConsecutiveFailures int       `json:"consecutive_failures"`
	LastError           string    `json:"last_error,omitempty"`
}

// ProbeFunc returns nil if the GPU is responsive, a non-nil error otherwise.
type ProbeFunc func(ctx context.Context) error

// Monitor probes GPU health on a fixed interval and lets callers block until
// the GPU is responsive again. Safe for concurrent use.
type Monitor struct {
	interval         time.Duration
	probeTimeout     time.Duration
	failureThreshold int

	mu        sync.RWMutex
	probe     ProbeFunc
	status    Status
	healthyCh chan struct{} // closed iff status.Healthy
}

// NewMonitor returns a monitor with the given probe cadence. When enabled is
// false, Watch is a no-op and the monitor reports healthy forever — useful for
// CPU-only deployments. Defaults are applied for non-positive durations.
func NewMonitor(enabled bool, interval, probeTimeout time.Duration, failureThreshold int) *Monitor {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	if probeTimeout <= 0 {
		probeTimeout = 8 * time.Second
	}
	if failureThreshold <= 0 {
		failureThreshold = 2
	}
	m := &Monitor{
		interval:         interval,
		probeTimeout:     probeTimeout,
		failureThreshold: failureThreshold,
		probe:            NvidiaSmiProbe,
		status:           Status{Enabled: enabled, Healthy: true},
		healthyCh:        make(chan struct{}),
	}
	close(m.healthyCh) // start healthy; waiters unblock immediately
	return m
}

// SetProbe overrides the default probe. Used in tests.
func (m *Monitor) SetProbe(p ProbeFunc) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.probe = p
}

// Watch runs the probe loop until ctx is done. No-op for disabled monitors.
func (m *Monitor) Watch(ctx context.Context) {
	m.mu.RLock()
	enabled := m.status.Enabled
	m.mu.RUnlock()
	if !enabled {
		return
	}
	t := time.NewTicker(m.interval)
	defer t.Stop()
	m.runProbe(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			m.runProbe(ctx)
		}
	}
}

func (m *Monitor) runProbe(ctx context.Context) {
	pctx, cancel := context.WithTimeout(ctx, m.probeTimeout)
	defer cancel()
	m.mu.RLock()
	probe := m.probe
	m.mu.RUnlock()
	err := probe(pctx)
	m.applyResult(err)
}

func (m *Monitor) applyResult(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now().UTC()
	m.status.LastCheck = now
	if err == nil {
		m.status.ConsecutiveFailures = 0
		m.status.LastError = ""
		m.status.LastHealthy = now
		if !m.status.Healthy {
			m.status.Healthy = true
			close(m.healthyCh)
		}
		return
	}
	m.status.ConsecutiveFailures++
	m.status.LastError = err.Error()
	if m.status.ConsecutiveFailures >= m.failureThreshold && m.status.Healthy {
		m.status.Healthy = false
		m.healthyCh = make(chan struct{})
	}
}

// IsHealthy reports the current health state.
func (m *Monitor) IsHealthy() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.status.Healthy
}

// Status returns a copy of the latest observation.
func (m *Monitor) Status() Status {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.status
}

// WaitHealthy blocks until the GPU is healthy or ctx is done. Returns nil when
// healthy, ctx.Err() otherwise.
func (m *Monitor) WaitHealthy(ctx context.Context) error {
	m.mu.RLock()
	healthy := m.status.Healthy
	ch := m.healthyCh
	m.mu.RUnlock()
	if healthy {
		return nil
	}
	select {
	case <-ch:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// NvidiaSmiProbe runs `nvidia-smi -L` and returns an error if the binary
// fails or hangs past the context deadline. Listing devices is enough to
// exercise the GSP RPC path that wedges under Xid 119 without doing any real
// allocation.
func NvidiaSmiProbe(ctx context.Context) error {
	out, err := exec.CommandContext(ctx, "nvidia-smi", "-L").CombinedOutput()
	if err != nil {
		return fmt.Errorf("nvidia-smi -L: %w (output: %q)", err, string(out))
	}
	return nil
}
