package core_test

import (
	"testing"

	"perfolizer/pkg/core"
)

func TestParameterIsExtractor(t *testing.T) {
	tests := []struct {
		name     string
		param    core.Parameter
		expected bool
	}{
		{name: "static", param: core.Parameter{Type: core.ParamTypeStatic}, expected: false},
		{name: "regexp", param: core.Parameter{Type: core.ParamTypeRegexp}, expected: true},
		{name: "json", param: core.Parameter{Type: core.ParamTypeJSON}, expected: true},
		{name: "unknown", param: core.Parameter{Type: "Other"}, expected: false},
	}

	for _, tc := range tests {
		if got := tc.param.IsExtractor(); got != tc.expected {
			t.Fatalf("%s: IsExtractor() = %v, want %v", tc.name, got, tc.expected)
		}
	}
}
