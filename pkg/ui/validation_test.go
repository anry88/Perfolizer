package ui

import (
	"strings"
	"testing"
)

func TestParseUsersInput(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expected    int
		errContains string
	}{
		{name: "valid boundary", input: "1", expected: 1},
		{name: "non numeric", input: "abc", errContains: "Users must be a whole number"},
		{name: "below minimum", input: "0", errContains: "Users must be greater than or equal to 1"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			value, err := parseUsersInput(tc.input)
			if tc.errContains == "" {
				if err != nil {
					t.Fatalf("expected no error, got %v", err)
				}
				if value != tc.expected {
					t.Fatalf("expected value %d, got %d", tc.expected, value)
				}
				return
			}
			if err == nil {
				t.Fatal("expected validation error")
			}
			if !strings.Contains(err.Error(), tc.errContains) {
				t.Fatalf("expected error %q to contain %q", err, tc.errContains)
			}
		})
	}
}

func TestParseIterationsInput(t *testing.T) {
	value, err := parseIterationsInput("-1")
	if err != nil {
		t.Fatalf("expected -1 to be valid, got %v", err)
	}
	if value != -1 {
		t.Fatalf("expected -1, got %d", value)
	}

	if _, err := parseIterationsInput("-2"); err == nil || !strings.Contains(err.Error(), "Iterations must be greater than or equal to -1") {
		t.Fatalf("expected lower-bound error, got %v", err)
	}
}

func TestParseRPSInput(t *testing.T) {
	value, err := parseRPSInput("Target RPS", "0")
	if err != nil {
		t.Fatalf("expected 0 to be valid, got %v", err)
	}
	if value != 0 {
		t.Fatalf("expected 0, got %v", value)
	}

	if _, err := parseRPSInput("Target RPS", "NaN"); err == nil || !strings.Contains(err.Error(), "Target RPS must be a finite number") {
		t.Fatalf("expected finite-number error, got %v", err)
	}
	if _, err := parseRPSInput("Target RPS", "-0.1"); err == nil || !strings.Contains(err.Error(), "Target RPS must be greater than or equal to 0") {
		t.Fatalf("expected non-negative error, got %v", err)
	}
}

func TestParseDurationMillisInput(t *testing.T) {
	value, err := parseDurationMillisInput("Duration", "0")
	if err != nil {
		t.Fatalf("expected 0 to be valid, got %v", err)
	}
	if value != 0 {
		t.Fatalf("expected 0, got %d", value)
	}

	if _, err := parseDurationMillisInput("Duration", "abc"); err == nil || !strings.Contains(err.Error(), "Duration must be a whole number") {
		t.Fatalf("expected numeric validation error, got %v", err)
	}
	if _, err := parseDurationMillisInput("Duration", "-1"); err == nil || !strings.Contains(err.Error(), "Duration must be greater than or equal to 0 ms") {
		t.Fatalf("expected non-negative validation error, got %v", err)
	}
}
