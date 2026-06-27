// Package gpu provides a lightweight GPU health monitor used to gate work
// dispatch when the GPU driver is wedged (e.g. NVRM Xid 119 / GSP RPC timeouts).
//
// The monitor periodically runs a cheap probe (default: `nvidia-smi -L`) with
// a hard timeout. After N consecutive failures it flips to unhealthy and any
// caller blocked on WaitHealthy stays blocked until the probe succeeds again
// or the caller's context is cancelled.
//
// Host-side recovery (PCI remove+rescan, container bounce) lives outside this
// process and is the operator's responsibility — pair this with your own host
// watchdog. This monitor's job is only to stop feeding the wedged driver with
// new allocations, which previously piled up uninterruptible
// nvidia-container-cli processes and made manual recovery harder.
package gpu

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
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

// Metric is a per-GPU utilization/temperature/memory sample. Values are the
// raw integers reported by nvidia-smi (percent, °C, MiB).
type Metric struct {
	Index       int `json:"index"`
	Utilization int `json:"utilization"`
	Temperature int `json:"temperature"`
	MemoryUsed  int `json:"memory_used"`
	MemoryTotal int `json:"memory_total"`
}

// ProbeFunc returns nil if the GPU is responsive, a non-nil error otherwise.
type ProbeFunc func(ctx context.Context) error

// Monitor probes GPU health on a fixed interval and lets callers block until
// the GPU is responsive again. Safe for concurrent use.
type Monitor struct {
	interval         time.Duration
	probeTimeout     time.Duration
	failureThreshold int
	metricsInterval  time.Duration

	mu        sync.RWMutex
	probe     ProbeFunc
	status    Status
	healthyCh chan struct{} // closed iff status.Healthy
	metrics   []Metric      // most recent per-GPU telemetry snapshot
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
		metricsInterval:  4 * time.Second,
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
	mt := time.NewTicker(m.metricsInterval)
	defer mt.Stop()
	m.runProbe(ctx)
	m.runMetrics(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			m.runProbe(ctx)
		case <-mt.C:
			m.runMetrics(ctx)
		}
	}
}

// runMetrics refreshes the cached per-GPU telemetry snapshot. Failures (e.g.
// nvidia-smi missing) leave the previous snapshot untouched rather than
// clobbering it with an empty list on a transient hiccup.
func (m *Monitor) runMetrics(ctx context.Context) {
	mctx, cancel := context.WithTimeout(ctx, m.probeTimeout)
	defer cancel()
	metrics, err := QueryGPUMetrics(mctx)
	if err != nil {
		return
	}
	m.mu.Lock()
	m.metrics = metrics
	m.mu.Unlock()
}

// Metrics returns a copy of the most recent per-GPU telemetry snapshot. The
// slice is empty when telemetry is unavailable (CPU-only, nvidia-smi missing,
// or before the first successful poll).
func (m *Monitor) Metrics() []Metric {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Metric, len(m.metrics))
	copy(out, m.metrics)
	return out
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

// QueryGPUMetrics runs a single nvidia-smi query for per-GPU utilization,
// temperature and memory. Returns an error if the binary is missing or fails;
// callers should treat that as "telemetry unavailable", not fatal.
func QueryGPUMetrics(ctx context.Context) ([]Metric, error) {
	out, err := exec.CommandContext(ctx, "nvidia-smi",
		"--query-gpu=index,utilization.gpu,temperature.gpu,memory.used,memory.total",
		"--format=csv,noheader,nounits").Output()
	if err != nil {
		return nil, fmt.Errorf("nvidia-smi query: %w", err)
	}
	var metrics []Metric
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		fields := strings.Split(line, ",")
		if len(fields) < 5 {
			continue
		}
		metrics = append(metrics, Metric{
			Index:       atoiField(fields[0]),
			Utilization: atoiField(fields[1]),
			Temperature: atoiField(fields[2]),
			MemoryUsed:  atoiField(fields[3]),
			MemoryTotal: atoiField(fields[4]),
		})
	}
	return metrics, nil
}

// atoiField parses one trimmed nvidia-smi CSV field, yielding 0 for blanks or
// non-numeric values like "[N/A]".
func atoiField(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
}
