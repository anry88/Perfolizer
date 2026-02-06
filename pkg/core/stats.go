package core

import (
	"sync"
	"time"
)

type StatsRunner struct {
	mu          sync.RWMutex
	sampleCount int
	latencySum  time.Duration
	startTime   time.Time

	// Callback for updates
	OnUpdate func(rps float64, avgLatency float64)
}

func NewStatsRunner(onUpdate func(rps float64, avgLatency float64)) *StatsRunner {
	sr := &StatsRunner{
		startTime: time.Now(),
		OnUpdate:  onUpdate,
	}
	// Start reporter loop
	go sr.reportLoop()
	return sr
}

func (sr *StatsRunner) ReportResult(result *SampleResult) {
	sr.mu.Lock()
	defer sr.mu.Unlock()
	sr.sampleCount++
	sr.latencySum += result.Duration()
}

func (sr *StatsRunner) reportLoop() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for range ticker.C {
		sr.mu.Lock()
		count := sr.sampleCount
		sum := sr.latencySum
		// Reset for next second
		sr.sampleCount = 0
		sr.latencySum = 0
		sr.mu.Unlock()

		rps := float64(count) // Since bucket is 1 second
		avgLat := 0.0
		if count > 0 {
			avgLat = float64(sum.Milliseconds()) / float64(count)
		}

		if sr.OnUpdate != nil {
			sr.OnUpdate(rps, avgLat)
		}
	}
}
