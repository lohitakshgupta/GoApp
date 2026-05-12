package observability

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/lohitakshgupta/GoApp/internal/job"
)

type Metrics struct {
	mu                sync.RWMutex
	jobsCreated       int64
	jobsStarted       int64
	jobsRetried       int64
	jobsCompleted     map[job.State]int64
	processingSeconds map[job.State]float64
	startedAt         time.Time
}

func NewMetrics() *Metrics {
	return &Metrics{
		jobsCompleted:     make(map[job.State]int64),
		processingSeconds: make(map[job.State]float64),
		startedAt:         time.Now().UTC(),
	}
}

func (m *Metrics) JobCreated() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.jobsCreated++
}

func (m *Metrics) JobStarted() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.jobsStarted++
}

func (m *Metrics) JobRetried() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.jobsRetried++
}

func (m *Metrics) JobCompleted(state job.State, duration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.jobsCompleted[state]++
	m.processingSeconds[state] += duration.Seconds()
}

func (m *Metrics) Prometheus() string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var b strings.Builder
	fmt.Fprintf(&b, "job_service_uptime_seconds %.0f\n", time.Since(m.startedAt).Seconds())
	fmt.Fprintf(&b, "job_created_total %d\n", m.jobsCreated)
	fmt.Fprintf(&b, "job_started_total %d\n", m.jobsStarted)
	fmt.Fprintf(&b, "job_retried_total %d\n", m.jobsRetried)
	for state, count := range m.jobsCompleted {
		fmt.Fprintf(&b, "job_completed_total{state=%q} %d\n", state, count)
	}
	for state, seconds := range m.processingSeconds {
		fmt.Fprintf(&b, "job_processing_seconds_sum{state=%q} %.6f\n", state, seconds)
	}
	return b.String()
}
