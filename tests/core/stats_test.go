package core_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"perfolizer/pkg/core"
)

func TestStatsRunnerPublishesMetrics(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	updates := make(chan map[string]core.Metric, 4)
	runner := core.NewStatsRunner(ctx, func(data map[string]core.Metric) {
		select {
		case updates <- data:
		default:
		}
	})

	start := time.Now()
	runner.ReportResult(&core.SampleResult{
		SamplerName: "HTTP",
		StartTime:   start,
		EndTime:     start.Add(100 * time.Millisecond),
		Success:     true,
	})
	runner.ReportResult(&core.SampleResult{
		SamplerName: "HTTP",
		StartTime:   start,
		EndTime:     start.Add(300 * time.Millisecond),
		Success:     false,
		Error:       errors.New("network"),
	})

	var snapshot map[string]core.Metric
	select {
	case snapshot = <-updates:
	case <-time.After(2500 * time.Millisecond):
		t.Fatal("timed out waiting for stats update")
	}

	httpMetric, ok := snapshot["HTTP"]
	if !ok {
		t.Fatalf("expected sampler metric in snapshot, got %#v", snapshot)
	}
	if httpMetric.TotalRequests != 2 {
		t.Fatalf("expected total requests 2, got %d", httpMetric.TotalRequests)
	}
	if httpMetric.TotalErrors != 1 {
		t.Fatalf("expected total errors 1, got %d", httpMetric.TotalErrors)
	}
	if httpMetric.Errors != 1 {
		t.Fatalf("expected interval errors 1, got %d", httpMetric.Errors)
	}
	if httpMetric.AvgLatency <= 0 {
		t.Fatalf("expected positive avg latency, got %f", httpMetric.AvgLatency)
	}
	if httpMetric.RPS <= 0 {
		t.Fatalf("expected positive RPS, got %f", httpMetric.RPS)
	}

	total := snapshot["Total"]
	if total.TotalRequests != 2 {
		t.Fatalf("expected total requests 2, got %d", total.TotalRequests)
	}
	if total.TotalErrors != 1 {
		t.Fatalf("expected total errors 1, got %d", total.TotalErrors)
	}

	copied := runner.Snapshot()
	if copied["Total"].TotalRequests != 2 {
		t.Fatalf("expected snapshot copy to include total requests 2, got %d", copied["Total"].TotalRequests)
	}
}
