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

func TestParsePrometheusSnapshotSupportsComplexLabels(t *testing.T) {
	metrics := `
perfolizer_rps{sampler="Sampler with , comma"} 1.5
perfolizer_rps{sampler="Sampler with \"quoted\" value",path="/foo/bar"} 2.0
perfolizer_rps{sampler="Sampler with escaped \\ backslash"} 3.0
`

	snapshot, err := parsePrometheusSnapshot(strings.NewReader(metrics))
	if err != nil {
		t.Fatalf("parsePrometheusSnapshot returned error: %v", err)
	}

	if snapshot.Data["Sampler with , comma"].RPS != 1.5 {
		t.Fatalf("expected sampler with comma RPS 1.5, got %v", snapshot.Data["Sampler with , comma"].RPS)
	}
	if snapshot.Data["Sampler with \"quoted\" value"].RPS != 2.0 {
		t.Fatalf("expected sampler with quotes RPS 2.0, got %v", snapshot.Data["Sampler with \"quoted\" value"].RPS)
	}
	if snapshot.Data["Sampler with escaped \\ backslash"].RPS != 3.0 {
		t.Fatalf("expected sampler with backslash RPS 3.0, got %v", snapshot.Data["Sampler with escaped \\ backslash"].RPS)
	}
}

func TestParsePrometheusSnapshotSupportsLongLines(t *testing.T) {
	// Create a very long label value
	longLabel := strings.Repeat("a", 1024*1024) // 1MB-ish label
	metrics := `perfolizer_rps{sampler="` + longLabel + `"} 42.0` + "\n"

	snapshot, err := parsePrometheusSnapshot(strings.NewReader(metrics))
	if err != nil {
		t.Fatalf("parsePrometheusSnapshot returned error for long line: %v", err)
	}

	if snapshot.Data[longLabel].RPS != 42.0 {
		t.Fatalf("expected long label sampler RPS 42.0, got %v", snapshot.Data[longLabel].RPS)
	}
}
