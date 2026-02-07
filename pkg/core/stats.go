package core

import (
	"context"
	"sync"
	"time"
)

type Metric struct {
	RPS           float64
	AvgLatency    float64
	Errors        int
	TotalRequests int
	TotalErrors   int
}

type StatsRunner struct {
	mu sync.RWMutex

	intervalCounts map[string]int
	intervalErrors map[string]int
	intervalLatSum map[string]time.Duration

	totalCounts map[string]int
	totalErrors map[string]int
	totalLatSum map[string]time.Duration

	knownSamplers map[string]bool
	latest        map[string]Metric

	reportInterval time.Duration

	// Callback for updates
	OnUpdate func(data map[string]Metric)
}

func NewStatsRunner(ctx context.Context, onUpdate func(data map[string]Metric)) *StatsRunner {
	sr := &StatsRunner{
		intervalCounts: make(map[string]int),
		intervalErrors: make(map[string]int),
		intervalLatSum: make(map[string]time.Duration),
		totalCounts:    make(map[string]int),
		totalErrors:    make(map[string]int),
		totalLatSum:    make(map[string]time.Duration),
		knownSamplers:  make(map[string]bool),
		latest: map[string]Metric{
			"Total": {},
		},
		reportInterval: time.Second,
		OnUpdate:       onUpdate,
	}
	go sr.reportLoop(ctx)
	return sr
}

func (sr *StatsRunner) ReportResult(result *SampleResult) {
	sr.mu.Lock()
	defer sr.mu.Unlock()

	name := result.SamplerName
	sr.knownSamplers[name] = true

	sr.intervalCounts[name]++
	sr.intervalLatSum[name] += result.Duration()

	sr.totalCounts[name]++
	sr.totalLatSum[name] += result.Duration()

	if !result.Success || result.Error != nil {
		sr.intervalErrors[name]++
		sr.totalErrors[name]++
	}
}

func (sr *StatsRunner) Snapshot() map[string]Metric {
	sr.mu.RLock()
	defer sr.mu.RUnlock()

	out := make(map[string]Metric, len(sr.latest))
	for k, v := range sr.latest {
		out[k] = v
	}
	if _, ok := out["Total"]; !ok {
		out["Total"] = Metric{}
	}
	return out
}

func (sr *StatsRunner) reportLoop(ctx context.Context) {
	ticker := time.NewTicker(sr.reportInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		sr.publishIntervalSnapshot()
	}
}

func (sr *StatsRunner) publishIntervalSnapshot() {
	sr.mu.Lock()
	defer sr.mu.Unlock()

	windowSeconds := sr.reportInterval.Seconds()
	if windowSeconds <= 0 {
		windowSeconds = 1
	}

	data := make(map[string]Metric, len(sr.knownSamplers)+1)

	totalIntervalCount := 0
	totalIntervalErrors := 0
	var totalIntervalLatSum time.Duration
	totalRequestCount := 0
	totalErrorCount := 0

	for sampler := range sr.knownSamplers {
		intervalCount := sr.intervalCounts[sampler]
		intervalErrors := sr.intervalErrors[sampler]
		intervalLatSum := sr.intervalLatSum[sampler]

		totalCount := sr.totalCounts[sampler]
		totalErrors := sr.totalErrors[sampler]

		totalIntervalCount += intervalCount
		totalIntervalErrors += intervalErrors
		totalIntervalLatSum += intervalLatSum
		totalRequestCount += totalCount
		totalErrorCount += totalErrors

		avgLatency := 0.0
		if intervalCount > 0 {
			avgLatency = float64(intervalLatSum.Milliseconds()) / float64(intervalCount)
		}

		data[sampler] = Metric{
			RPS:           float64(intervalCount) / windowSeconds,
			AvgLatency:    avgLatency,
			Errors:        intervalErrors,
			TotalRequests: totalCount,
			TotalErrors:   totalErrors,
		}
	}

	totalAvgLatency := 0.0
	if totalIntervalCount > 0 {
		totalAvgLatency = float64(totalIntervalLatSum.Milliseconds()) / float64(totalIntervalCount)
	}

	data["Total"] = Metric{
		RPS:           float64(totalIntervalCount) / windowSeconds,
		AvgLatency:    totalAvgLatency,
		Errors:        totalIntervalErrors,
		TotalRequests: totalRequestCount,
		TotalErrors:   totalErrorCount,
	}

	sr.latest = data

	sr.intervalCounts = make(map[string]int, len(sr.intervalCounts))
	sr.intervalErrors = make(map[string]int, len(sr.intervalErrors))
	sr.intervalLatSum = make(map[string]time.Duration, len(sr.intervalLatSum))

	if sr.OnUpdate != nil {
		copyData := make(map[string]Metric, len(sr.latest))
		for k, v := range sr.latest {
			copyData[k] = v
		}
		sr.OnUpdate(copyData)
	}
}
