package elements_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"perfolizer/pkg/core"
	"perfolizer/pkg/elements"
)

type sampleCaptureRunner struct {
	results chan *core.SampleResult
}

func (r *sampleCaptureRunner) ReportResult(result *core.SampleResult) {
	r.results <- result
}

func TestHttpSamplerReportsTimeoutSampleResult(t *testing.T) {
	runtime := &core.HTTPRuntime{
		Client: &http.Client{
			Transport: &blockingRoundTripper{releaseAfter: 500 * time.Millisecond},
		},
		RequestTimeout: 50 * time.Millisecond,
	}

	ctx := core.NewContext(core.WithHTTPRuntime(context.Background(), runtime), 1)
	runner := &sampleCaptureRunner{results: make(chan *core.SampleResult, 1)}
	ctx.SetVar("Reporter", runner)

	sampler := elements.NewHttpSampler("Slow endpoint", http.MethodGet, "https://example.com/slow")
	if err := sampler.Execute(ctx); err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}

	result := waitForSampleResult(t, runner.results)
	if result.Success {
		t.Fatal("expected timeout sample to be marked as failed")
	}
	if result.Error == nil {
		t.Fatal("expected timeout error on sample result")
	}
	if !errors.Is(result.Error, context.DeadlineExceeded) && !isTimeoutError(result.Error) {
		t.Fatalf("expected deadline-exceeded style error, got %v", result.Error)
	}
	if result.ResponseCode != "" {
		t.Fatalf("expected empty response code on timeout, got %q", result.ResponseCode)
	}
	if result.Latency <= 0 || result.Latency >= 250*time.Millisecond {
		t.Fatalf("expected timeout latency below 250ms, got %v", result.Latency)
	}
}

func TestHttpSamplerReturnsPromptlyOnContextCancellation(t *testing.T) {
	requestStarted := make(chan struct{}, 1)
	runtime := &core.HTTPRuntime{
		Client: &http.Client{
			Transport: &blockingRoundTripper{started: requestStarted},
		},
		RequestTimeout: 2 * time.Second,
	}

	parent := core.WithHTTPRuntime(context.Background(), runtime)
	runCtx, cancel := context.WithCancel(parent)
	defer cancel()

	ctx := core.NewContext(runCtx, 3)
	runner := &sampleCaptureRunner{results: make(chan *core.SampleResult, 1)}
	ctx.SetVar("Reporter", runner)

	sampler := elements.NewHttpSampler("Blocked endpoint", http.MethodGet, "https://example.com/blocked")
	done := make(chan error, 1)
	go func() {
		done <- sampler.Execute(ctx)
	}()

	select {
	case <-requestStarted:
	case <-time.After(time.Second):
		t.Fatal("request did not start in time")
	}

	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Execute returned unexpected error: %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("sampler did not return promptly after cancellation")
	}

	result := waitForSampleResult(t, runner.results)
	if result.Success {
		t.Fatal("expected canceled sample to be marked as failed")
	}
	if result.Error == nil {
		t.Fatal("expected cancellation error on sample result")
	}
	if !errors.Is(result.Error, context.Canceled) {
		t.Fatalf("expected canceled context error, got %v", result.Error)
	}
}

func waitForSampleResult(t *testing.T, results <-chan *core.SampleResult) *core.SampleResult {
	t.Helper()

	select {
	case result := <-results:
		if result == nil {
			t.Fatal("expected non-nil sample result")
		}
		return result
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for sample result")
		return nil
	}
}

type timeoutError interface {
	error
	Timeout() bool
}

type blockingRoundTripper struct {
	started      chan<- struct{}
	releaseAfter time.Duration
}

func (rt *blockingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if rt != nil && rt.started != nil {
		select {
		case rt.started <- struct{}{}:
		default:
		}
	}

	if rt != nil && rt.releaseAfter > 0 {
		select {
		case <-req.Context().Done():
			return nil, req.Context().Err()
		case <-time.After(rt.releaseAfter):
			return newHTTPResponse(req, http.StatusOK, "ok"), nil
		}
	}

	<-req.Context().Done()
	return nil, req.Context().Err()
}

func isTimeoutError(err error) bool {
	var netErr timeoutError
	return errors.As(err, &netErr) && netErr.Timeout()
}

func newHTTPResponse(req *http.Request, statusCode int, body string) *http.Response {
	return &http.Response{
		StatusCode: statusCode,
		Status:     http.StatusText(statusCode),
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    req,
	}
}
