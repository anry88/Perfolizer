package core

import (
	"context"
	"sync"
	"time"
)

type Metric struct {
	RPS        float64
	AvgLatency float64
}

type StatsRunner struct {
	mu            sync.RWMutex
	counts        map[string]int
	latSum        map[string]time.Duration
	knownSamplers map[string]bool // Track seen samplers
	startTime     time.Time

	// Callback for updates
	OnUpdate func(data map[string]Metric)
}

func NewStatsRunner(ctx context.Context, onUpdate func(data map[string]Metric)) *StatsRunner {
	sr := &StatsRunner{
		counts:        make(map[string]int),
		latSum:        make(map[string]time.Duration),
		knownSamplers: make(map[string]bool),
		startTime:     time.Now(),
		OnUpdate:      onUpdate,
	}
	// Start reporter loop
	go sr.reportLoop(ctx)
	return sr
}

func (sr *StatsRunner) ReportResult(result *SampleResult) {
	sr.mu.Lock()
	defer sr.mu.Unlock()
	sr.counts[result.SamplerName]++
	sr.latSum[result.SamplerName] += result.Duration()
	sr.knownSamplers[result.SamplerName] = true
}

func (sr *StatsRunner) reportLoop(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Continue reporting
		}

		sr.mu.Lock()
		// Copy current state
		currentCounts := make(map[string]int)
		currentSum := make(map[string]time.Duration)
		known := make([]string, 0, len(sr.knownSamplers))

		for k := range sr.knownSamplers {
			known = append(known, k)
			if v, ok := sr.counts[k]; ok {
				currentCounts[k] = v
			}
			if v, ok := sr.latSum[k]; ok {
				currentSum[k] = v
			}
		}

		// Reset for next interval
		sr.counts = make(map[string]int)
		sr.latSum = make(map[string]time.Duration)
		sr.mu.Unlock()

		data := make(map[string]Metric)

		// Calculate aggregate (Total)
		totalCount := 0
		var totalSum time.Duration

		for _, name := range known {
			count := currentCounts[name]
			sum := currentSum[name]

			totalCount += count
			totalSum += sum

			rps := float64(count)
			avgLat := 0.0
			if count > 0 {
				avgLat = float64(sum.Milliseconds()) / float64(count)
			}
			data[name] = Metric{RPS: rps, AvgLatency: avgLat}
		}

		// Total Metric
		totalRps := float64(totalCount)
		totalAvgLat := 0.0
		if totalCount > 0 {
			totalAvgLat = float64(totalSum.Milliseconds()) / float64(totalCount)
		}
		data["Total"] = Metric{RPS: totalRps, AvgLatency: totalAvgLat}

		if sr.OnUpdate != nil {
			sr.OnUpdate(data)
		}
	}
}
