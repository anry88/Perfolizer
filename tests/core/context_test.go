package core_test

import (
	"context"
	"testing"
	"time"

	"perfolizer/pkg/core"
)

func TestContextSubstitute(t *testing.T) {
	ctx := core.NewContext(context.Background(), 7)
	ctx.SetVar("host", "localhost")
	ctx.SetVar("port", 8080)
	ctx.SetVar("path", "api")

	tests := []struct {
		input    string
		expected string
	}{
		{"http://${host}:${port}/${path}", "http://localhost:8080/api"},
		{"no variables", "no variables"},
		{"${missing}", "${missing}"},
		{"prefix-${host}-suffix", "prefix-localhost-suffix"},
		{"", ""},
	}

	for _, tc := range tests {
		if got := ctx.Substitute(tc.input); got != tc.expected {
			t.Fatalf("Substitute(%q) = %q; want %q", tc.input, got, tc.expected)
		}
	}
}

func TestContextInheritsParentState(t *testing.T) {
	parent := core.NewContext(context.Background(), 1)
	parent.SetVar("token", "abc")
	parent.ParameterDefinitions["request_id"] = core.Parameter{ID: "p1", Name: "request_id", Type: core.ParamTypeStatic, Value: "42"}

	child := core.NewContext(parent, 2)

	if got := child.GetVar("token"); got != "abc" {
		t.Fatalf("expected inherited variable %q, got %#v", "abc", got)
	}

	param, ok := child.GetParameterDefinition("request_id")
	if !ok {
		t.Fatal("expected inherited parameter definition")
	}
	if param.Value != "42" {
		t.Fatalf("expected inherited parameter value %q, got %q", "42", param.Value)
	}

	child.SetVar("token", "child")
	if got := parent.GetVar("token"); got != "abc" {
		t.Fatalf("expected parent variable to stay %q, got %#v", "abc", got)
	}
}

func TestContextInheritsHTTPRuntimeFromParentContext(t *testing.T) {
	runtime := core.NewHTTPRuntime(core.HTTPRuntimeOptions{
		RequestTimeout: 1250 * time.Millisecond,
	})

	ctx := core.NewContext(core.WithHTTPRuntime(context.Background(), runtime), 9)

	if got := ctx.HTTPClient(); got != runtime.Client {
		t.Fatalf("expected inherited HTTP client %p, got %p", runtime.Client, got)
	}
	if got := ctx.EffectiveHTTPRequestTimeout(0); got != 1250*time.Millisecond {
		t.Fatalf("expected inherited HTTP timeout %v, got %v", 1250*time.Millisecond, got)
	}
	if got := ctx.EffectiveHTTPRequestTimeout(250 * time.Millisecond); got != 250*time.Millisecond {
		t.Fatalf("expected sampler override timeout %v, got %v", 250*time.Millisecond, got)
	}
}

func TestContextAccessorsAndSampleResultDuration(t *testing.T) {
	ctx := core.NewContext(context.Background(), 3)

	if got := ctx.GetVar("missing"); got != nil {
		t.Fatalf("expected nil for missing variable, got %#v", got)
	}

	if _, ok := ctx.GetParameterDefinition("missing"); ok {
		t.Fatal("expected missing parameter definition")
	}

	start := time.Now()
	end := start.Add(350 * time.Millisecond)
	result := &core.SampleResult{StartTime: start, EndTime: end}

	if got := result.Duration(); got != 350*time.Millisecond {
		t.Fatalf("expected duration %v, got %v", 350*time.Millisecond, got)
	}
}
