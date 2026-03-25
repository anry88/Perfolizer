package ui

import (
	"strings"
	"testing"
)

func TestParsePrometheusSnapshotKeepsSamplerSeriesWithSpaces(t *testing.T) {
	metrics := `
# HELP perfolizer_test_running Test running state
perfolizer_test_running 1
perfolizer_rps{sampler="Total"} 7
perfolizer_avg_response_time_ms{sampler="Total"} 507.14
perfolizer_errors_total{sampler="Total"} 23
perfolizer_rps{sampler="Home Page - Main URL (5 RPS)"} 5
perfolizer_avg_response_time_ms{sampler="Home Page - Main URL (5 RPS)"} 430
perfolizer_errors_total{sampler="Home Page - Main URL (5 RPS)"} 11
perfolizer_rps{sampler="Search Query - random perf check alpha (1 RPS)"} 1
perfolizer_avg_response_time_ms{sampler="Search Query - random perf check alpha (1 RPS)"} 612
perfolizer_errors_total{sampler="Search Query - random perf check alpha (1 RPS)"} 4
`

	snapshot, err := parsePrometheusSnapshot(strings.NewReader(metrics))
	if err != nil {
		t.Fatalf("parsePrometheusSnapshot returned error: %v", err)
	}

	if !snapshot.Running {
		t.Fatal("expected running snapshot")
	}
	if len(snapshot.Data) != 3 {
		t.Fatalf("expected Total plus 2 sampler series, got %d entries: %#v", len(snapshot.Data), snapshot.Data)
	}
	if snapshot.Data["Home Page - Main URL (5 RPS)"].RPS != 5 {
		t.Fatalf("expected home page sampler RPS 5, got %#v", snapshot.Data["Home Page - Main URL (5 RPS)"])
	}
	if snapshot.Data["Search Query - random perf check alpha (1 RPS)"].TotalErrors != 4 {
		t.Fatalf("expected search sampler total errors 4, got %#v", snapshot.Data["Search Query - random perf check alpha (1 RPS)"])
	}
}

func TestParseMetricWithLabelsSupportsCommasInsideQuotedValues(t *testing.T) {
	name, labels, err := parseMetricWithLabels(`perfolizer_rps{sampler="GET /search, homepage flow",path="/tmp/demo"}`)
	if err != nil {
		t.Fatalf("parseMetricWithLabels returned error: %v", err)
	}
	if name != "perfolizer_rps" {
		t.Fatalf("expected metric name perfolizer_rps, got %q", name)
	}
	if labels["sampler"] != "GET /search, homepage flow" {
		t.Fatalf("expected sampler label to preserve comma, got %q", labels["sampler"])
	}
	if labels["path"] != "/tmp/demo" {
		t.Fatalf("expected path label to parse, got %q", labels["path"])
	}
}
